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

- **Inputs pinned to a release tag.** If the input's `ref` looks like a version (`v1.6.2`, `1.2.0`, `dms/1.11.6`), nixup lists the repository's tags with `git ls-remote --tags` and takes the highest one with the same prefix. Prereleases are only considered when the current tag is a prerelease. `nix flake update` never moves a tag, so `nixup update` rewrites it in `flake.nix` (see [The input step](#the-input-step)). An outdated tag has `nixup update <name>` as its action; an up-to-date one says `pinned tag`.
- **Inputs that follow a nixpkgs package.** Some flakes are used only for what they add to a package that comes from nixpkgs, such as a Home Manager module. Such an input should stay at the version nixpkgs packages, not at upstream's newest. Mark it in `flake.nix` with a comment above the input; other comments and blank lines may sit in between:

  ```nix
  # nixup: follows nixpkgs#dux-shell
  dms = {
    url = "github:duxlabs-dev/dux-shell/v2.1.1";
  };
  ```

  nixup reads that attribute's version in the nixpkgs your flake locks, the same evaluation as the [nixpkgs hint](#packages). If it is newer than the pinned tag, the row is outdated with the upstream tag of that version as the latest, and `nixup update` moves the tag there. Otherwise the row is `ok` (`follows nixpkgs dux-shell 2.1.1`), or `info` when upstream already has a newer tag (`follows nixpkgs dux-shell 2.1.1; upstream has v2.1.2`) or the pin is ahead of nixpkgs (`ahead of nixpkgs dux-shell 2.1.1`). Without a `nixpkgs` input, without that attribute in nixpkgs, or without an upstream tag for its version, the row is an error. The marker applies only to inputs pinned to a tag.
- **Branch inputs.** Any other `ref`, or none, is a branch. nixup asks `git ls-remote` for the head of that branch (or `HEAD` when there is no `ref`) and compares it with the locked commit. The current column shows the locked commit and its age in days.

### packages

nixup evaluates `.#packages.<system>` for the machine it runs on, and reads each package's `version`, `src.urls`, `meta.homepage`, `passthru.updateScript`, `passthru.nixup.pinned` and `passthru.nixup.versionUrl`.

- The upstream repository is the first `github.com/<owner>/<repo>` found in the update script's `--url`, then in the source URLs, then in the homepage. `--url` is how nix-update finds the versions of a source hosted elsewhere, so a tarball from the project's own server can follow the tags of its GitHub repository:

  ```nix
  passthru.updateScript = nix-update-script {
    extraArgs = [ "--url" "https://github.com/owner/repo" ];
  };
  ```

  Without one, the package is reported as `info` with `no GitHub upstream: check by hand`.
- Without `passthru.updateScript` the package is reported as `info` with `no passthru.updateScript`, because `nixup update` could not update it.
- nixup reads the flags of an update script that runs nix-update. A script of its own (`writeShellScript`, a path) is reported as `info` with `updateScript is not nix-update: set passthru.nixup.versionUrl`, since its versions could come from anywhere.
- A package kept where it is on purpose can say so with `passthru.nixup.pinned`, a reason. nixup then looks nothing up: the row is `ok`, hidden without `--all`, and `NOTE` shows `pinned: <reason>`. It is for a source with no versions to follow, such as a single file from a repository commit:

  ```nix
  passthru.nixup.pinned = "a font file at a plugin commit; bump it when the font changes";
  ```

- A package whose versions are not the tags or releases of its repository can name a URL whose body is the latest version, in `passthru.nixup.versionUrl`. nixup reads the first line, compares it with `version`, and `update` runs nix-update pinned to it, as for any package. The review still compares the GitHub tags of both versions when the package has a GitHub upstream. Brave, for example, tags its Release, Beta and Nightly builds alike, and publishes each channel's version on its own:

  ```nix
  passthru.nixup.versionUrl = "https://versions.brave.com/latest/origin-release-linux-x64.version";
  ```

  `pinned` and `versionUrl` are the only nixup-specific attributes; for inputs there is the `# nixup: follows` comment above.

- If the update script passes `--use-github-releases`, nixup compares against published GitHub releases, ignoring drafts. Otherwise it compares against tags. The aim is to pick the same version `nix-update` would.
- If the update script passes `--version-regex`, nixup reads versions the same way: only tags the regex matches from their start count, and the version is its non-empty groups joined with `.`. That is how a repository with several tag series (`desktop-v0.6.1` next to `desktop-minimal-v0.6.1`) is followed. Without it, only tags that start with a digit or `v` plus a digit count.
- Prereleases count only when the update script passes `--version=unstable` or the current version is already a prerelease.
- If the flake has an input called `nixpkgs`, nixup also looks each package up in that locked nixpkgs, by its own name and, for `foo-bin`, as `foo`. When nixpkgs has the same project (same GitHub repository or same homepage) at the same or a newer version, the row gets a hint line, `→ nixpkgs has foo 1.2.0: pkgs.foo could replace this package`, and an up-to-date row is shown as `info` instead of being hidden. The hint never changes the exit code.

A flake without a `packages` output has nothing in this section.

### profile

If `nix` is installed, nixup reads `nix profile list --json`. For each entry whose original URL contains a 40-character nixpkgs revision, it compares that revision with the one locked for your flake's `nixpkgs` input. A different revision is reported as outdated with `pinned: nix profile remove, then install again`, because `nix profile upgrade` cannot move a pinned entry. Entries that follow a moving reference (such as `flake:nixpkgs`) instead of a fixed revision are `ok`, with `nixup update <name>` as the action; like every `ok` row they are hidden without `--all`. If `nix profile list` fails, the section has one `error` row.

### flatpak

If `flatpak` is installed, nixup runs `flatpak remote-ls --updates` and lists each application or runtime once, with the installed version (from `flatpak list`) and the new one. Runtimes have no version, so their branch is shown as the current one. `LATEST` is the new version; it says `new build` when the same version is rebuilt, and for every runtime update, since a runtime has only a branch. It says `unknown` when the app's remote does not publish versions, as with a remote added from a `.flatpakref` file (for example `hylki-origin`). The action is `nixup update <app id>`.

## Versions

Tags and versions are compared by their numbers, ignoring a leading `v` or a prefix like `dms/`. A prerelease sorts below its final release. Words like `alpha`, `beta`, `rc`, `pre`, `preview`, `dev`, `nightly` and `snapshot` mark a prerelease when they start a segment, so `1.2.3-rc.1` is a prerelease and `1.2.3-arch` is not.

A jump gets a warning when the first number changes (`1.x` to `2.x`), or when the second number changes while the first is `0` (`0.21` to `0.22`).

## The upstream review

With `check --review`, and always in `nixup update`, nixup builds a review for each pending update:

- `source`: the GitHub repository.
- `changes`: the number of commits between the two versions, from the GitHub compare API, how many files changed with the lines added and removed, and a compare link: `changes  45 commits · 38 files (+1,204 −311)  <compare url>`. In Markdown it is `[45 commits](<compare url>) · 38 files (+1,204 −311)`. The compare API lists at most 300 files, so from 300 on the line says `300+ files` without line counts. When GitHub gives no files, that part is left out. In JSON these are `files`, `additions` and `deletions`.
- Up to eight commit subjects, newest first.
- For packages, and for pinned inputs in `check --review`, the release notes published between the two versions. Drafts are skipped. Prereleases are skipped and counted. The terminal shows the first lines of up to three releases; the compare link has the rest.

Branch inputs, and every input in `nixup update`, get commits only, without release notes. If the GitHub API fails, the review says `review unavailable` with the reason, and the update can still go ahead.

## update, step by step

`nixup update` runs four steps in this order: inputs, packages, Flatpak and `nix profile`. Inputs go first, so packages are built against the new lock. Each step runs even when an earlier one failed or you answered no; packages are then built against the lock you already had. The exit code is the highest of the steps. With `--test`, `--switch` or `--boot`, the host to activate is picked before the first step and nh runs after the last one (see [Activation](#activation)).

With `--source`, only the listed steps run. With names, each name is looked up in `flake.lock`, in the flake's packages (evaluated only when the package step is on), in `flatpak list --columns=application` (apps and runtimes) and in `nix profile list --json`. Each step gets only the names of its source, and a step with none is skipped.

The input and package steps only edit files in your repository; nothing reaches the system until you apply it with nh, yourself or through `--test`, `--switch` or `--boot`. The Flatpak and `nix profile` steps install on the machine right away. Their question says so, and they print how to undo it.

### The input step

1. If `git diff --quiet -- flake.lock` reports changes, read the committed lock with `git show HEAD:flake.lock` and print one line naming the inputs already moved that this update does not cover. There is no question: the update runs on top of the working `flake.lock`, so those changes are kept and only what nixup moves is shown.
2. For each input in scope (the names you gave, or every input) pinned to a release tag, list its tags as `check` does. If a newer tag exists, the input's tag will move. If the tags cannot be listed, the tag stays and nixup says so. Without names, a likely breaking jump is left alone and reported; with the input's name it moves, with the warning.
3. If a tag will move and `git diff --quiet -- flake.nix` reports changes, ask, unless `--allow-dirty` or `-y`. A no ends the step with exit code 0.
4. Create a temporary directory and copy into it every file `git ls-files --cached --others --exclude-standard` lists: tracked files with their uncommitted edits, plus untracked files that are not ignored. Symlinks and files deleted from the working tree are skipped. This is the scratch copy.
5. For each tag that moves, replace the old tag with the new one in the copy's `flake.nix`. The old tag is read from `original.ref` in `flake.lock` and the new one from the tag list, both as written. The old tag must appear exactly once in `flake.nix` as a whole word, not inside a longer version; otherwise the step stops with exit code 2 before `nix flake update` runs.
6. Run `nix flake update --flake <copy>`, with the input names you gave.
7. For each tag that moved, check that the new lock pins the input (`original.ref`) to the new tag. If not, stop with exit code 2 and write nothing.
8. Compare the old and new lock. For each direct input whose commit moved, print the upstream commits. Count the other nodes that moved.
9. Only with `--build`, `--test`, `--switch` or `--boot`: take the hosts from `-H`, or list them with `nix eval <copy>#nixosConfigurations --apply builtins.attrNames`. With an activation flag, the only host is the one to activate.
10. Only with those flags, build each host with `nh os build <copy> -H <host> -o <copy>/result`. Nothing is activated. The first failed build stops the input step with exit code 2 and your `flake.lock` and `flake.nix` stay as they were.
11. Append the Markdown report if `--report` was given. This happens before the question, so the report is written even if you answer no.
12. Ask, unless `-y`. On yes, write the new `flake.lock` into your repository, and the copy's `flake.nix` if a tag moved.
13. Delete the scratch copy.

Without those flags steps 9 and 10 are skipped: no host is built, and the report has no build lines. Because the copy includes uncommitted and untracked files, a host build tests your working tree as it is now, not the last commit.

### The package step

For each package, in alphabetical order, or in the order you named them. `<pkg dir>` is the directory of the file that defines the package, read from its `meta.position` like `nix-update` does (`packages/prebuilt/kopuz` for a package in `packages/prebuilt/kopuz/package.nix`); for a package defined in a file at the top of the flake, such as `flake.nix`, it is that file alone, never the whole repository. If the file is not part of this flake, the package is refused before anything runs.

1. Run the same check as `nixup check` for that package, with the review.
2. If it is outdated, print the review.
3. If `git diff --quiet -- <pkg dir>` reports changes, ask whether to update on top of them, unless `--allow-dirty` or `-y`. A no skips this package with exit code 0. Then ask whether to update it, unless `-y`.
4. Save every regular file under `<pkg dir>`. Run `nix-update --flake <name> --version=<reviewed version>` plus the extra arguments from the update script. If `nix-update` is not on `PATH`, use `nix run nixpkgs#nix-update --`.
5. Run `nix build --no-link .#<name>`. This build always runs.
6. On failure in step 4 or 5, write the saved files back as they were and remove the files added since. On success, print `<name> updated to <version>.` and read `git diff -- <pkg dir>`: changes to routine attributes (`version`, `hash`, `sha256`, `rev`, and any attribute ending in `Hash`) are not shown; any other changed line in a `.nix` file is printed in full, and other files only as a count of added and removed lines, under `Besides version and hash, nix-update edited:`.

`nix-update` edits your repository directly here, not a copy. The files saved in step 4 are what puts the package back, uncommitted edits included. git is not used for it.

### The Flatpak step

Skipped when `flatpak` is not on `PATH`.

1. Run `flatpak remote-ls --updates --columns=ref,version` and `flatpak list --columns=ref,version`, as the `flatpak` check section does, and keep the named IDs if names were given.
2. Print the table of pending updates.
3. Note the deployed commit of each ref with `flatpak info --show-commit <ref>`.
4. Ask once, unless `-y`.
5. Run `flatpak update -y --noninteractive <ref...>` with exactly the refs shown.
6. Print `flatpak update --commit=<previous commit> <ref>` for each ref. This is printed after a failure too, since some refs may have moved.

### The nix profile step

1. Read `nix profile list --json`, and keep the named entries if names were given.
2. Leave out every entry whose original URL contains a 40-character revision, and print how to update it by hand: `nix profile remove`, then install it again.
3. Ask once, unless `-y`.
4. Run `nix profile upgrade <name...>` with the entries that follow a moving reference.
5. Print `nix profile rollback` as the way back.

### Activation

Only with `--test`, `--switch` or `--boot`.

1. Before the first step, take the host from `-H`, or list the hosts with `nix eval <repo>#nixosConfigurations --apply builtins.attrNames`. Anything but exactly one host stops `update` with exit code 2 before any work.
2. After the last step, if a step failed, print `Not activating: a step failed.` If neither `flake.lock` nor a package was written, print `Nothing was written: nothing to activate.` In both cases nh is not run.
3. Otherwise run `nh os <test|switch|boot> <repo> -H <host>` in your repository. nh builds the system, from the store when the input step already built it (it builds the activated host whenever it writes `flake.lock`), asks for sudo and activates. nixup only reports whether nh succeeded.

## Commands nixup runs

This is the complete list. Everything goes through one package (`internal/run`), so nothing else is started.

| Command | Where |
|---|---|
| `git ls-remote --tags --refs https://github.com/<repo>` | inputs, packages, reviews, update (inputs pinned to a tag) |
| `git ls-remote https://github.com/<repo> <ref>` | branch inputs |
| `git ls-files -z --cached --others --exclude-standard` | update (inputs), scratch copy |
| `git diff --quiet -- <path>` | update (inputs and packages) |
| `git show HEAD:flake.lock` | update (inputs), to name what an uncommitted `flake.lock` carries besides the update |
| `git diff -- <pkg dir>` | update (packages), after a success |
| `nix eval --json .#packages.<system> --apply ...` | packages, update |
| `nix eval --json --inputs-from . nixpkgs#legacyPackages.<system> --apply ...` | packages, if the flake has a `nixpkgs` input |
| `nix eval --json <copy>#nixosConfigurations --apply builtins.attrNames` | update (inputs) with `--build` and without `-H`, completion of `-H` |
| `nix eval --json <repo>#nixosConfigurations --apply builtins.attrNames` | update (`--test`, `--switch`, `--boot`), without `-H` |
| `nix eval --json .#packages.<system> --apply builtins.attrNames` | completion of names |
| `nix flake update --flake <copy> [input...]` | update (inputs) |
| `nh os build <copy> -H <host> -o <copy>/result` | update (inputs) with `--build`, `--test`, `--switch` or `--boot` |
| `nix-update --flake <name> --version=<v> [args...]` | update (packages) |
| `nix build --no-link .#<name>` | update (packages) |
| `nh os <test\|switch\|boot> <repo> -H <host>` | update (`--test`, `--switch`, `--boot`) |
| `nix profile list --json` | profile, update (names, nix profile) |
| `nix profile upgrade <name...>` | update (nix profile) |
| `nix eval --json --impure --expr <getFlake of the flake>` | search |
| `nix-store --query --requisites /run/current-system` | search (what is in the system) |
| `flatpak list --app --columns=...` | list, search |
| `flatpak search --columns=... <name>` | search |
| `flatpak list --app --columns=application` | search (what is installed) |
| `flatpak list --columns=application` | update (names) |
| `flatpak remote-ls --updates --columns=ref,version` | flatpak, update (Flatpak) |
| `flatpak list --columns=ref,version` | flatpak, update (Flatpak), installed versions |
| `flatpak info --show-commit <ref>` | update (Flatpak), before updating |
| `flatpak update -y --noninteractive <ref...>` | update (Flatpak) |
| `gh auth token` | GitHub token, if no variable is set |

nixup's own HTTP requests go only to `api.github.com`. `git`, `nix` and `nix-update` reach the network on their own.

## What is written and what never is

Written:

- By the input step of `update`: your `flake.lock`, after a yes or `-y`, and your `flake.nix` too when a pinned tag moved. A temporary scratch copy, deleted at the end.
- By the package step of `update`: the files `nix-update` changes, usually under `<pkg dir>`. If the update or build fails, the files under `<pkg dir>` are put back as they were.
- By `--report <file>`: Markdown appended to that file.
- Builds land in the Nix store, as with any build.

Activated, only with `--test`, `--switch` or `--boot`:

- The new configuration, by `nh os test`, `switch` or `boot` on one host, after every step succeeded and only if `flake.lock` or a package was written. nh asks for sudo, not nixup.

Installed, after a yes or `-y`:

- By the Flatpak step of `update`: the pending Flatpak updates, through `flatpak update`. Go back with the `flatpak update --commit=<previous commit> <ref>` lines it prints.
- By the `nix profile` step of `update`: new versions of the profile entries that follow a moving reference, through `nix profile upgrade`. Undo it with `nix profile rollback`.

Never:

- `git add`, `git commit` or `git push`.
- `nixos-rebuild`, `nh os switch`, `nh os boot`, `nh os test`, or anything else that activates a generation, unless you pass `--test`, `--switch` or `--boot`. Then nh does it.
- `sudo` from nixup itself. With those flags, nh asks for it.
- A daemon, timer or background process.
