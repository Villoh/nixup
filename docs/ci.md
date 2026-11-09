---
title: CI
description: Run nixup from a scheduled GitHub Actions workflow to report pending updates and open an update pull request.
lang: en
---

# CI

**English** | [Español](es/ci.md)

nixup does not ship a workflow for your configuration. This page shows how to build one from the flags that exist for it: `--markdown`, `--json`, `--report` and `-y`.

## Flags for unattended runs

- `check --markdown` prints outdated and failed entries as Markdown. Add `--review` for commits and release notes.
- `check --json` prints every entry as JSON, for your own scripts.
- `lock --report <file>` and `pkg --report <file>` append what they changed, the upstream review and the build result to a Markdown file. Both commands can share one file.
- `-y` answers yes. A CI job has no terminal, so without `-y` every question is answered no and nothing is written.

`check` exits with 1 when something is outdated. In a CI step that counts as a failure, so accept 1 explicitly and let 2 fail the job:

```bash
nixup check --markdown --review >> "$GITHUB_STEP_SUMMARY" || [ $? -eq 1 ]
```

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
      - uses: actions/checkout@v4
      - uses: DeterminateSystems/nix-installer-action@main

      - name: pending updates
        run: |
          # exit code 1 only means "something is outdated"
          nix run "$NIXUP" -- check --source input,package --markdown --review \
            >> "$GITHUB_STEP_SUMMARY" || [ $? -eq 1 ]

      - name: update flake.lock and packages
        run: |
          touch "$RUNNER_TEMP/report.md"
          nix run "$NIXUP" -- lock -y --no-build --report "$RUNNER_TEMP/report.md"
          nix run "$NIXUP" -- pkg -y --report "$RUNNER_TEMP/report.md"

      - uses: peter-evans/create-pull-request@v7
        with:
          branch: nixup/updates
          commit-message: "chore: update flake inputs and packages"
          title: "chore: update flake inputs and packages"
          body-path: ${{ runner.temp }}/report.md
```

Notes on this workflow:

- `--source input,package` skips `profile` and `flatpak`, which describe the machine running nixup, not your configuration.
- The runner has no `nix-update`, so `pkg` runs it through `nix run nixpkgs#nix-update`.
- `pkg -y` builds each updated package on the runner. If one fails, its files are restored, `pkg` exits with 2 and the job stops before the pull request.
- To get the same nixup every week, pin a release tag in `NIXUP`, for example `github:Villoh/nixup/v0.2.0`.
- The commit and the pull request come from `create-pull-request`. nixup itself never commits.

## Full-system builds

The workflow passes `--no-build` to `lock`, so the new lock is written without building any host. A full NixOS system often needs more disk space and time than a GitHub-hosted runner provides, and `lock` builds every host in `nixosConfigurations` unless you pass `-H`.

You have three options:

- Keep `--no-build` in CI and run `nixup lock` (which builds) or `nh os build` on your machine before merging the pull request.
- Build one host with `lock -y -H <host> --report ...`. The runner then needs `nh` on `PATH` and enough space for that host.
- Use a self-hosted runner, where the build fits.

Whichever you pick, nixup never activates the result. Test the merged configuration with `nh os test` on the machine itself.
