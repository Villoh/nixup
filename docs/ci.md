---
title: CI
description: Run nixup from a scheduled GitHub Actions workflow to keep an issue with the pending updates, or to open an update pull request.
lang: en
---

# CI

**English** | [Español](es/ci.md)

nixup does not ship a workflow for your configuration. This page shows how to build one from the flags that exist for it: `--markdown`, `--json`, `--report` and `-y`.

## Flags for unattended runs

- `check --markdown` prints outdated and failed entries as Markdown. Add `--review` for commits and release notes.
- `check --json` prints every entry as JSON, for your own scripts.
- `update --report <file>` appends what changed, the upstream review and the build result to a Markdown file. Several runs can share one file.
- `-y` answers yes. A CI job has no terminal, so without `-y` every question is answered no and nothing is written.

`check` exits with 1 when something is outdated. In a CI step that counts as a failure, so accept 1 explicitly and let 2 fail the job:

```bash
nixup check --markdown --review >> "$GITHUB_STEP_SUMMARY" || [ $? -eq 1 ]
```

## A weekly issue

If you update on your own machine, where the hosts can be built, a reminder is enough. This workflow keeps the pending updates in an issue and changes nothing in the repository:

```yaml
name: updates

on:
  schedule:
    - cron: "0 6 * * 1" # Mondays at 06:00 UTC
  workflow_dispatch:

permissions:
  contents: read
  issues: write

concurrency: updates

jobs:
  check:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ github.token }} # gh, and nixup's GitHub API calls
      TITLE: Pending updates
      LABEL: updates
      NIXUP: github:Villoh/nixup/v0.8.0 # or the rev your flake.lock pins
    steps:
      - uses: actions/checkout@v7
      - uses: DeterminateSystems/nix-installer-action@v23

      - name: update the issue
        run: |
          set -euo pipefail
          nixup=$NIXUP
          sources=input,package

          code=0
          nix run "$nixup" -- check --source "$sources" --json > rows.json ||
            code=$?
          if [ "$code" -ge 2 ]; then # a check failed: red run
            cat rows.json
            exit 1
          fi

          read -r num state < <(
            gh issue list --label "$LABEL" --state all --limit 1 \
              --json number,state --jq '.[0] | "\(.number) \(.state)"'
          ) || true
          open=""
          if [ "${state:-}" = OPEN ]; then open=$num; fi

          if [ "$code" -eq 0 ]; then # everything is up to date
            if [ -n "$open" ]; then
              gh issue close "$open" --comment "Everything is up to date."
            fi
            exit 0
          fi

          # What "something new" means: a name that was not pending before.
          # Versions are left out: a branch or a nightly moves every day, and
          # an entry already pending is already known.
          pending=$(jq -r '[.[] | select(.status == "outdated") | .name]
            | sort | join(" ")' rows.json)
          nix run "$nixup" -- check --source "$sources" --markdown \
            --review > review.md || [ $? -eq 1 ]
          {
            echo "Pending updates as of $(date -u +%F)."
            echo "Apply them locally with \`nixup update\`."
            echo
            # The ACTION column is left out: it is "nixup update <name>"
            # but for the rows that need something else, kept as Notes.
            jq -r '
              def kind: {inputs: "input", packages: "package"}[.] // .;
              def status: if .status != "outdated" then .status
                elif (.warning // "") | test("MAJOR") then "**major**"
                elif (.warning // "") != "" then "**0.x-minor**"
                else "update" end;
              def notes: [(.note // "" | select(startswith("nixup update")
                | not)), (.hint // empty)] | map(select(. != ""))
                | join("; ");
              "| Source | Name | Current | Latest | Status | Notes |",
              "|---|---|---|---|---|---|",
              (.[] | select(.status != "ok")
                | "| \(.section | kind) | \(.name) | `\(.current)` " +
                  "| `\(.latest // "")` | \(status) | \(notes) |")
            ' rows.json
          } > body.md
          # Issue bodies stop at 65536 characters: drop the details if
          # they do not fit.
          size=$(($(wc -c < body.md) + $(wc -c < review.md)))
          if [ "$size" -lt 60000 ]; then
            {
              printf '\n<details><summary>What changed upstream</summary>\n\n'
              cat review.md
              printf '\n</details>\n'
            } >> body.md
          fi

          printf '\n<!-- nixup: %s -->\n' "$pending" >> body.md

          if [ -n "$open" ]; then
            before=$(gh issue view "$open" --json body --jq .body |
              sed -n 's/^<!-- nixup: \(.*\) -->$/\1/p')
            new=$(comm -23 <(tr ' ' '\n' <<< "$pending" | sort) \
              <(tr ' ' '\n' <<< "$before" | sort))
            if [ -z "$new" ]; then # nothing new: refresh it quietly
              gh issue edit "$open" --body-file body.md
              exit 0
            fi
          fi

          gh label create "$LABEL" --color 4A6FC4 --force \
            --description "Pending flake updates (nixup)"
          url=$(gh issue create --title "$TITLE ($(date -u +%F))" \
            --label "$LABEL" --body-file body.md)
          if [ -n "$open" ]; then
            gh issue close "$open" --comment "Superseded by #${url##*/}."
          fi
```

Notes on this workflow:

- At most one issue is open. A new issue, which notifies you, replaces the open one only when an input or package that was not pending before shows up; otherwise the open one is refreshed quietly. When nothing is pending, it is closed.
- "New" compares names, not versions: a branch input or a nightly moves every day, and an entry already pending is already known. The names are kept in an HTML comment at the end of the issue.
- The table comes from `check --json`. It has no command column: every row is applied with `nixup update <name>`.
- `--review` adds the upstream commits and release notes in a collapsed section, left out if the body would pass GitHub's 65536-character limit.
- `GH_TOKEN` serves both `gh` and nixup's GitHub API calls. Without a token nixup is limited to 60 API calls an hour and the reviews fail.
- A check that fails (exit code 2) turns the run red, and GitHub tells you by mail.
- If your flake has nixup as an input, take the version from `flake.lock` instead, so the issue matches what `nixup check` says on your machine: `nixup="github:Villoh/nixup/$(jq -r .nodes.nixup.locked.rev flake.lock)"`.

## A weekly update pull request

This workflow lists pending updates in the job summary, updates `flake.lock` and your packages, and opens a pull request with the report as its body. Put it in your configuration repository, for example as `.github/workflows/updates.yml`:

```yaml
name: updates

on:
  schedule:
    - cron: "0 6 * * 1" # Mondays at 06:00 UTC
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  updates:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ github.token }} # for the GitHub API calls in the reviews
      NIXUP: github:Villoh/nixup
    steps:
      - uses: actions/checkout@v7
      - uses: DeterminateSystems/nix-installer-action@v23

      - name: pending updates
        run: |
          # exit code 1 only means "something is outdated"
          nix run "$NIXUP" -- check --source input,package --markdown --review \
            >> "$GITHUB_STEP_SUMMARY" || [ $? -eq 1 ]

      - name: update flake.lock and packages
        run: |
          touch "$RUNNER_TEMP/report.md"
          nix run "$NIXUP" -- update --source input -y --report "$RUNNER_TEMP/report.md"
          nix run "$NIXUP" -- update --source package -y --report "$RUNNER_TEMP/report.md"

      - uses: peter-evans/create-pull-request@v7
        with:
          branch: nixup/updates
          commit-message: "chore: update flake inputs and packages"
          title: "chore: update flake inputs and packages"
          body-path: ${{ runner.temp }}/report.md
```

Notes on this workflow:

- `--source input,package` skips `profile` and `flatpak`, which describe the machine running nixup, not your configuration.
- The two update commands pass `--source input` and `--source package` for the same reason: without `--source`, `update -y` would also run the Flatpak and `nix profile` steps, which install on the runner instead of editing the repository.
- The input step needs no flag to skip the host build: without `--build` it builds no host.
- The runner has no `nix-update`, so `update` runs it through `nix run nixpkgs#nix-update`.
- `update --source package -y` builds each updated package on the runner. If one fails, its files are restored, nixup exits with 2 and the job stops before the pull request.
- To get the same nixup every week, pin a release tag in `NIXUP`, for example `github:Villoh/nixup/v0.8.0`.
- The commit and the pull request come from `create-pull-request`. nixup itself never commits.

## Full-system builds

The input step builds no host unless you pass `--build`, so the workflow writes the new lock without building any host. A full NixOS system often needs more disk space and time than a GitHub-hosted runner provides, and `update --build` builds every host in `nixosConfigurations` unless you pass `-H`.

You have three options:

- Leave the input step as it is in CI and run `nh os build` (or `nh os test`) on your machine on the pull request branch before merging it.
- Build one host on the runner with `update --source input -y --build -H <host> --report ...`. The runner then needs `nh` on `PATH` and enough space for that host.
- Use a self-hosted runner, where the build fits.

Whichever you pick, do not pass `--test`, `--switch` or `--boot` in CI: they would activate the configuration on the runner. Without them nixup never activates the result. Test the merged configuration with `nh os test` on the machine itself.
