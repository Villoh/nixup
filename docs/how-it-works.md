---
title: How it works
description: What each nixup section checks, how upstream changes are reviewed, the scratch copy, and what is written to disk.
lang: en
---

# How it works

**English** | [Español](es/how-it-works.md)

## The check sections

`nixup check` runs its four sections at the same time and up to six lookups per section in parallel. None of them writes to your repository.

### inputs

nixup reads `flake.lock` and looks at the direct inputs of your flake whose original type is `github`. Inputs that only `follows` another one are skipped, and so are inputs of any other type.

- **Inputs pinned to a release tag.** If the input's `ref` looks like a version (`v1.6.2`, `1.2.0`, `dms/1.11.6`), nixup lists the repository's tags with `git ls-remote --tags` and takes the highest one with the same prefix. Prereleases are only considered when the current tag is a prerelease. `nix flake update` never moves a tag, so the note says `pinned tag: edit flake.nix`.
- **Branch inputs.** Any other `ref`, or none, is a branch. nixup asks `git ls-remote` for the head of that branch (or `HEAD` when there is no `ref`) and compares it with the locked commit. The current column shows the locked commit and its age in days.

### packages

nixup evaluates `.#packages.<system>` for the machine it runs on, and reads each package's `version`, `src.urls`, `meta.homepage` and `passthru.updateScript`.

- The upstream repository is the first `github.com/<owner>/<repo>` found in the source URLs, then in the homepage. Without one, the package is reported as `info` with `no GitHub upstream: check by hand`.
- Without `passthru.updateScript` the package is reported as `info` with `no passthru.updateScript`, because `nixup pkg` could not update it.
- If the update script passes `--use-github-releases`, nixup compares against published GitHub releases, ignoring drafts. Otherwise it compares against tags. The aim is to pick the same version `nix-update` would.
- If the update script passes `--version-regex`, nixup reads versions the same way: only tags the regex matches from their start count, and the version is its non-empty groups joined with `.`. That is how a repository with several tag series (`desktop-v0.6.1` next to `desktop-minimal-v0.6.1`) is followed. Without it, only tags that start with a digit or `v` plus a digit count.
- Prereleases count only when the update script passes `--version=unstable` or the current version is already a prerelease.
- If the flake has an input called `nixpkgs`, nixup also looks each package up in that locked nixpkgs, by its own name and, for `foo-bin`, as `foo`. When nixpkgs has the same project (same GitHub repository or same homepage) at the same or a newer version, the row gets a hint line, `→ nixpkgs has foo 1.2.0: pkgs.foo could replace this package`, and an up-to-date row is shown as `info` instead of being hidden. The hint never changes the exit code.

A flake without a `packages` output has nothing in this section.

### profile

If `nix` is installed, nixup reads `nix profile list --json`. For each entry whose original URL contains a 40-character nixpkgs revision, it compares that revision with the one locked for your flake's `nixpkgs` input. A different revision is reported as `pinned to an old nixpkgs`. Entries that follow a flake reference instead of a fixed revision are `ok`, with `nix profile upgrade <name>` as the note.

### flatpak

If `flatpak` is installed, nixup runs `flatpak remote-ls --updates` and lists each application or runtime once, with the installed version (from `flatpak list`) and the new one. Runtimes have no version, so their branch is shown; an update that keeps the same version shows as `new build`. It only reports; `flatpak update` is up to you.

## Versions

Tags and versions are compared by their numbers, ignoring a leading `v` or a prefix like `dms/`. A prerelease sorts below its final release. Words like `alpha`, `beta`, `rc`, `pre`, `preview`, `dev`, `nightly` and `snapshot` mark a prerelease when they start a segment, so `1.2.3-rc.1` is a prerelease and `1.2.3-arch` is not.

A jump gets a warning when the first number changes (`1.x` to `2.x`), or when the second number changes while the first is `0` (`0.21` to `0.22`).

## The upstream review

With `check --review`, and always in `lock` and `pkg`, nixup builds a review for each pending update:

- `source`: the GitHub repository.
- `changes`: the number of commits between the two versions, from the GitHub compare API, and a compare link.
- Up to eight commit subjects, newest first.
- For packages, and for pinned inputs in `check --review`, the release notes published between the two versions. Drafts are skipped. Prereleases are skipped and counted. The terminal shows the first lines of up to three releases; the compare link has the rest.

Branch inputs, and every input in `lock`, get commits only, without release notes. If the GitHub API fails, the review says `review unavailable` with the reason, and the update can still go ahead.

## lock, step by step

1. If `git diff --quiet -- flake.lock` reports changes, stop with exit code 1.
2. Create a temporary directory and copy into it every file `git ls-files --cached --others --exclude-standard` lists: tracked files with their uncommitted edits, plus untracked files that are not ignored. Symlinks and files deleted from the working tree are skipped. This is the scratch copy.
3. Run `nix flake update --flake <copy>`, with the input names you gave.
4. Compare the old and new lock. For each direct input whose commit moved, print the upstream commits. Count the other nodes that moved.
5. Unless `--no-build`, take the hosts from `-H`, or list them with `nix eval <copy>#nixosConfigurations --apply builtins.attrNames`.
6. Unless `--no-build`, build each host with `nh os build <copy> -H <host> -o <copy>/result`. Nothing is activated. The first failed build stops the run with exit code 2 and your `flake.lock` stays as it was.
7. Append the Markdown report if `--report` was given. This happens before the question, so the report is written even if you answer no.
8. Ask, unless `-y`. On yes, write the new `flake.lock` into your repository.
9. Delete the scratch copy.

Because the copy includes uncommitted and untracked files, the build tests your working tree as it is now, not the last commit.

## pkg, step by step

For each package, in alphabetical order, or in the order you named them. `<pkg dir>` is the directory of the file that defines the package, read from its `meta.position` like `nix-update` does (`packages/prebuilt/kopuz` for a package in `packages/prebuilt/kopuz/package.nix`); for a package defined in a file at the top of the flake, such as `flake.nix`, it is that file alone, never the whole repository. If the file is not part of this flake, the package is refused before anything runs.

1. Run the same check as `nixup check` for that package, with the review.
2. If it is outdated, print the review and ask, unless `-y`.
3. If `git diff --quiet -- <pkg dir>` reports changes, refuse this package with exit code 1 and go on.
4. Run `nix-update --flake <name> --version=<reviewed version>` plus the extra arguments from the update script. If `nix-update` is not on `PATH`, use `nix run nixpkgs#nix-update --`.
5. Run `nix build --no-link .#<name>`, unless `--no-build`.
6. On failure in step 4 or 5, run `git checkout -- <pkg dir>`. On success, print `git diff -- <pkg dir>`.

`nix-update` edits your repository directly here, not a copy. The `git checkout` in step 6 is what puts the files back.

## Commands nixup runs

This is the complete list. Everything goes through one package (`internal/run`), so nothing else is started.

| Command | Where |
|---|---|
| `git ls-remote --tags --refs https://github.com/<repo>` | inputs, packages, reviews |
| `git ls-remote https://github.com/<repo> <ref>` | branch inputs |
| `git ls-files -z --cached --others --exclude-standard` | lock, scratch copy |
| `git diff --quiet -- <path>` | lock, pkg |
| `git checkout -- <pkg dir>` | pkg, after a failure |
| `git diff -- <pkg dir>` | pkg, after a success |
| `nix eval --json .#packages.<system> --apply ...` | packages, pkg |
| `nix eval --json --inputs-from . nixpkgs#legacyPackages.<system> --apply ...` | packages, if the flake has a `nixpkgs` input |
| `nix eval --json <copy>#nixosConfigurations --apply builtins.attrNames` | lock |
| `nix flake update --flake <copy> [input...]` | lock |
| `nh os build <copy> -H <host> -o <copy>/result` | lock |
| `nix-update --flake <name> --version=<v> [args...]` | pkg |
| `nix build --no-link .#<name>` | pkg |
| `nix profile list --json` | profile |
| `nix eval --json --impure --expr <getFlake of the flake>` | search |
| `nix-store --query --requisites /run/current-system` | search (what is in the system) |
| `flatpak list --app --columns=...` | list, search |
| `flatpak search --columns=... <name>` | search |
| `flatpak remote-ls --updates --columns=application,version,branch` | flatpak |
| `flatpak list --columns=application,version,branch` | flatpak (installed versions) |
| `gh auth token` | GitHub token, if no variable is set |

nixup's own HTTP requests go only to `api.github.com`. `git`, `nix` and `nix-update` reach the network on their own.

## What is written and what never is

Written:

- By `lock`: your `flake.lock`, after a yes or `-y`. A temporary scratch copy, deleted at the end.
- By `pkg`: the files `nix-update` changes, usually under `<pkg dir>`. They are restored if the update or build fails.
- By `--report <file>`: Markdown appended to that file.
- Builds land in the Nix store, as with any build.

Never:

- `git add`, `git commit` or `git push`.
- `nixos-rebuild`, `nh os switch`, `nh os boot`, `nh os test`, or anything else that activates a generation.
- `sudo`.
- A daemon, timer or background process.
