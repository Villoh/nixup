---
title: Releases
description: How maintainers cut a nixup release with the manual release workflow and git-cliff notes.
lang: en
---

# Releases

**English** | [Español](es/releases.md)

Releases are made by a manual GitHub Actions workflow. Do not edit the version or create a tag by hand.

## Cut a release

From the Actions tab, or with the GitHub CLI:

```bash
gh workflow run release -f version=0.2.0                  # bump, commit, tag and publish
gh workflow run release -f version=0.2.0 -f dry_run=true  # check everything, publish nothing
```

The workflow only runs from `main`.

## What the workflow does

`.github/workflows/release.yml`, in order:

1. Checks that the version is `X.Y.Z`, higher than the current one, and that the tag `vX.Y.Z` does not exist yet.
2. Replaces the version in `flake.nix`. The version lives in one line there, marked with the comment `# the one place the version lives`. The workflow finds the line by that comment, so keep it.
3. Runs `go vet ./...` and `go test -race -count=1 ./...`.
4. Generates the release notes with [git-cliff](https://git-cliff.org/) from the commits since the last tag, and prints them in the log.
5. Unless `dry_run` is set, commits the new `flake.nix` to `main` as `chore(release): vX.Y.Z` through the GitHub API, so GitHub signs it and it shows as Verified. The commit is rejected if `main` moved after the workflow started.
6. Publishes the GitHub release `vX.Y.Z` on that commit, with the generated notes, marked as latest.

No binaries are attached. Users install from the flake, and `nix run github:Villoh/nixup/vX.Y.Z` runs a specific release.

## Release notes

`cliff.toml` groups commits by their Conventional Commit type:

| Type | Section |
|---|---|
| `feat` | Features |
| `fix` | Bug fixes |
| `perf` | Performance |
| `refactor` | Refactoring |
| `docs` | Documentation |
| `test`, `ci`, `build` | Tests, CI and build |
| `chore` | Maintenance |
| anything else | Other |

`chore(release)` commits are left out. Each entry shows the scope in bold, the subject and a link to the commit, and the notes end with a full-changelog compare link.

Preview the notes for the next release locally:

```bash
nix run nixpkgs#git-cliff -- --unreleased --tag v0.2.0 --strip header
```
