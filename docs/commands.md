---
title: Commands
description: Every nixup command, flag, output format and exit code.
lang: en
---

# Commands

**English** | [Español](es/commands.md)

## Synopsis

```text
nixup [check] [flags]
nixup update [name...] [flags]
nixup list [flags]
nixup search <name> [flags]
nixup version
nixup help
nixup completion bash|zsh|fish
```

Without a command nixup runs `check`, so `nixup`, `nixup --review` and `nixup check --review` do the same thing.

Flags can go before, between or after names: `nixup update nixpkgs -y home-manager` updates two inputs without asking. Flags take one dash or two (`-y`, `--review`, `--source=input`).

Every command that works on a flake accepts `-C <dir>`. See [Configuration](configuration.md) for how the flake is found when `-C` is not given.

`nixup <command> -h` prints that command's flags on stderr and exits with 0.

## Output streams

- Results, reviews and questions go to stdout.
- Errors go to stderr.
- The output of `nh os build`, `nix build`, `flatpak update`, `nix profile upgrade` and `nh os test`, `switch` or `boot` is streamed to stderr. `nix flake update` and `nix-update` run quietly, since nixup summarises what they changed; their output is shown only when they fail. nix's `warning: Git tree … is dirty` is turned off (`warn-dirty = false` added to `NIX_CONFIG`), since nixup works on uncommitted trees on purpose.

So `nixup check --json > out.json` gives clean JSON.

## check

Lists what is outdated. Read-only.

```bash
nixup check
nixup check --review
nixup check --source input,package --all
nixup check --json
nixup check --markdown --review
```

| Flag | Description |
|---|---|
| `-C <dir>` | Flake directory. |
| `--source <list>` | Comma-separated sources to check: `input`, `package`, `flatpak`, `profile` (plurals work too). Default: all. An unknown name is an error. |
| `--all` | Also list up-to-date entries in the text output. Without it they are only counted. |
| `--review` | Add the upstream review (commits, changed files and lines, compare link, release notes) to every outdated entry. |
| `--json` | Print every row as JSON, up-to-date ones included. Takes precedence over `--markdown`. |
| `--markdown` | Print only outdated and failed entries as Markdown, for a pull request body or a CI summary. Prints `Everything is up to date.` when there is nothing. |

`check` takes no names; extra words after it are ignored.

The text output is one table: `SOURCE`, `NAME`, `CURRENT`, `LATEST`, `STATUS` and, when some row has one, `NOTE`, followed by a summary line and, with `--review`, one block of upstream changes per update. `STATUS` is `update`, `major` or `0.x-minor` (likely breaking), `ok`, `info` or `error`; see [Getting started](getting-started.md#reading-the-report). An `ok` row repeats its version under `LATEST`, and the summary counts the `ok` rows even when they are hidden. In JSON the statuses are `outdated`, `ok`, `info` and `error`, and a likely breaking jump has a `warning`:

```json
[
  {
    "section": "packages",
    "name": "ripgrep",
    "current": "14.0.0",
    "latest": "15.2.0",
    "status": "outdated",
    "note": "nixup update ripgrep",
    "warning": "MAJOR version jump: read the changelog, expect breaking changes"
  }
]
```

`latest`, `note`, `warning`, `hint` and `review` are left out when empty. `hint` says that the flake's nixpkgs already has the package (see [How it works](how-it-works.md#packages)). With `--review`, `review` holds `repo`, `from`, `to`, `compare_url`, `commits`, `files`, `additions`, `deletions`, `subjects`, `releases` (each with `tag` and `body`), `skipped_prereleases` and, if the GitHub API failed, `error`.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Everything checked is up to date. |
| 1 | At least one entry is outdated. |
| 2 | At least one check failed, or the command line or flake location was wrong. A failure wins over outdated entries. |

## update

Reviews and applies updates in four steps, in this order: the flake's inputs (`flake.lock`), the packages your flake defines in its own `packages.<system>` output, Flatpak, and `nix profile`.

The input and package steps only edit files in your repository; you apply them to the system with nh, or pass `--test`, `--switch` or `--boot` and nixup runs nh for you at the end (see [Activation](#activation)). The Flatpak and `nix profile` steps install on the machine right away, after one question each, and print how to undo it.

```bash
nixup update
nixup update nixpkgs home-manager
nixup update ripgrep
nixup update --build
nixup update co.hyprlab.Hylki
nixup update --source input -H desktop -H laptop
nixup update --source package -y --report report.md
nixup update --source flatpak
nixup update --test
```

Without names everything is updated: every input, every package, every pending Flatpak update and every `nix profile` entry that follows a moving reference. Each name is looked up as a direct input of `flake.lock`, as a package of the flake (only when the package step is on), as an installed Flatpak app ID and as a `nix profile` entry. Each name goes to the step of its source; a step with no names of its kind is skipped, so `nixup update ripgrep` never touches `flake.lock` and `nixup update co.hyprlab.Hylki` runs only the Flatpak step. Inputs always go first, so packages are built against the new lock.

A name that does not lead to exactly one step is an error, and nothing runs:

- In more than one enabled source: `<name> is both an input and a package: pick one with --source input or --source package`. With three or more sources: `<name> is an input, a package and a Flatpak app: pick one with --source input, --source package or --source flatpak`.
- Only in sources that `--source` leaves out: `<name> is an input, which --source leaves out`.
- Nowhere: `no input, package, Flatpak app or nix profile entry called <name>`. The message lists only the enabled sources, so with `--source input` it is `no input called <name>`, and packages are not even evaluated.

| Flag | Description |
|---|---|
| `-C <dir>` | Flake directory. |
| `--source <list>` | Comma-separated steps to run: `input`, `package`, `flatpak`, `profile` (plurals work too). Default: all. Anything else is an error. |
| `-H <host>` | Host to build in the input step with `--build`, and the host to activate with `--test`, `--switch` or `--boot`. Repeatable. Default: every entry in `nixosConfigurations`. |
| `--build` | In the input step, build the hosts in the scratch copy with `nh os build` before asking to write `flake.lock`. Without it the hosts are not built. `--test`, `--switch` and `--boot` imply it, for the activated host only. Packages are always built; the Flatpak and `nix profile` steps build nothing. |
| `-y` | Write the new lock, update the packages, install the Flatpak updates and upgrade the `nix profile` entries without asking. Needed for unattended runs. Implies `--allow-dirty`. |
| `--allow-dirty` | Update over uncommitted changes in `flake.nix` or a package without asking. They are kept. |
| `--report <file>` | Append a Markdown report (moved inputs, reviews, build results, one block per updated package, the Flatpak table with the commands to go back, the upgraded `nix profile` entries) to this file. Created if missing. |
| `--test` | After the four steps, run `nh os test` on the host if `flake.lock` or a package was written. |
| `--switch` | The same with `nh os switch`. |
| `--boot` | The same with `nh os boot`. |

Questions take `y` or `yes`, `n` or `no`; Enter is no, and anything else is asked again. What was typed while nixup was busy, such as an Enter pressed during a build, is dropped before each question.

### Inputs

The input step, in order:

1. If `flake.lock` has uncommitted changes, go on without asking: nothing is lost, the update starts from your working `flake.lock` and the changes shown are only what nixup moved. When the result will also carry inputs this update does not cover, say so: `flake.lock already has uncommitted changes (holded-tt): they stay, and this update goes on top.` The names are the inputs your working `flake.lock` moved since the last commit (`git show HEAD:flake.lock`) and that you did not name; without names every input is covered and nothing is printed. When nixup cannot tell what changed, the line has no names.
2. Look for inputs pinned to a release tag (`v1.6.2`, `1.2.0`, `dms/1.11.6`) with a newer tag upstream, among the named inputs or, without names, among all of them. Without names, a likely breaking jump is left alone: `dms v1.6.2 -> v2.1.2 is a likely breaking jump: left alone. Run nixup update dms to move it.` Named, it moves.
3. If a tag will move and `flake.nix` has uncommitted changes, ask `flake.nix has uncommitted changes. Update on top of them? They are kept. [y/N]`. A no prints `Skipped.` and ends the step. `--allow-dirty` and `-y` skip the question.
4. Copy the working tree to a temporary directory. For each tag that moves, replace it in the copy's `flake.nix` and print `Moving the dms tag in flake.nix: v1.6.2 -> v2.1.2`, followed by `⚠ MAJOR version jump: read the changelog, expect breaking changes` for a likely breaking jump. Then run `nix flake update --flake <copy> [input...]` there.
5. If nothing changed, print `flake.lock is already up to date.` and stop.
6. If a moved input is not pinned to its new tag in the new lock, stop with exit code 2: `after the edit, flake.lock pins dms to "v1.6.2", not v2.1.2: nothing written`.
7. For every direct input that moved, print the old and new commit and the upstream commits. Inputs that are not direct are counted on one line.
8. With `--build`, run `nh os build <copy> -H <host>` for each host: the ones from `-H`, or every entry in `nixosConfigurations`. With `--test`, `--switch` or `--boot`, only the host to activate. nh prints its diff (`CHANGED`, paths, size), so you see the impact on the system before the question. The first failure stops the step and the repository is left as it was. Without any of these flags no host is built.
9. Append the report, if `--report` was given. A moved tag adds a line such as ``Tag of `dms` moved in flake.nix: `v1.6.2` → `v2.1.2`.``
10. Ask `Write this flake.lock to the repo? [y/N]`, unless `-y`. Then copy the new `flake.lock` into the repository and print `flake.lock updated.` When a tag moved, the question is `Write this flake.lock and flake.nix to the repo? [y/N]`, the copy's `flake.nix` is written too, and the last line is `flake.lock and flake.nix updated.`

The tag is replaced as text. The old tag is taken from `flake.lock` (`original.ref`) and the new one from the tag list, both as written, so the tag format does not matter. The old tag must appear exactly once in `flake.nix` as a whole word, not inside a longer version such as `v1.2.0-rc1` or `v11.2.0`, in any form of the URL: `github:o/r/v1.2.0`, `?ref=v1.2.0` or `ref = "v1.2.0";`. Otherwise the step stops with exit code 2 before `nix flake update` runs, with one of these errors:

- `dms: the tag v1.0.0 appears 2 times in flake.nix; change it in flake.nix by hand`
- `dms: the tag v1.2.0 is not written in flake.nix as such; change it in flake.nix by hand`, for example when the URL is built with `${version}`.

If the tags of an input cannot be listed, nixup prints `<name>: cannot list its tags (<reason>): its tag stays.` and goes on without moving it.

### Packages

The package step goes through the packages one at a time. For each outdated package nixup:

1. Prints the current and new version, the major-jump warning if any, and the upstream review.
2. If `<pkg dir>` has uncommitted changes, asks `<pkg dir> has uncommitted changes. Update on top of them? They are kept. [y/N]`. A no prints `Skipped.` and moves on. With `--allow-dirty` or `-y` there is no question, only `<pkg dir> has uncommitted changes: updating on top of them; they are kept.`
3. Asks `Update <name> to <version>? [y/N]`, unless `-y`. A no prints `Skipped.` and moves on.
4. Saves every file under `<pkg dir>`, then runs `nix-update --flake <name> --version=<the reviewed version>`, plus the extra arguments from the package's `passthru.updateScript`. It prints `Updating <file> with nix-update…`, naming the file that defines the package; nix-update's own output is shown only when it fails.
5. Prints `Checking that <name> builds…` and runs `nix build --no-link .#<name>`. This builds the package alone, not the system: it checks that the edited recipe still builds (for a prebuilt `-bin`, that the release tarball exists, its hash matches and the patching works).
6. If step 4 or 5 fails, writes the saved files back as they were, uncommitted edits included, removes the files added since and prints `nix-update failed; <pkg dir> restored.` or `build FAILED; <pkg dir> restored.` Otherwise prints `<name> updated to <version>.` and reads `git diff -- <pkg dir>`. A routine update changes only routine attributes (`version`, `hash`, `sha256`, `rev` and any attribute ending in `Hash`) and prints nothing more. Anything else nix-update edited in your files is listed under `Besides version and hash, nix-update edited:`: each other changed line of a `.nix` file in full, and other files (lock files it regenerates) as a count, `packages/x/deps.json: +2 −1 lines`. These are edits to your repository, not upstream changes. The `--report` gets the files and attributes in any case.

Packages that are up to date are listed only when you name them. Entries marked `info` or `ERROR` are always listed. If you name no packages and none is outdated, the step prints `Every package is up to date.`

### Flatpak

The Flatpak step runs only when `flatpak` is installed. Without it the step is skipped without a message. In order:

1. List the pending updates with `flatpak remote-ls --updates` and the installed versions with `flatpak list`, the same data as `check`. With names, keep only those IDs; a named one with no update prints `<id> is up to date.` With no names and nothing pending, print `Every Flatpak is up to date.` and stop.
2. Print a table: `SOURCE`, `NAME`, `CURRENT`, `LATEST`. `LATEST` is the new version, `unknown` when the remote publishes none, or `new build` for runtimes and rebuilds of the same version.
3. Read the deployed commit of each ref with `flatpak info --show-commit <ref>`.
4. Ask once, `Update 3 Flatpak refs now? This installs them right away. [y/N]` (`Update 1 Flatpak ref now? This installs it right away.` for one), unless `-y`. A no prints `Skipped.`
5. Run `flatpak update -y --noninteractive` with exactly those refs.
6. Print `Flatpak updated.`, or `flatpak update failed; some refs may have been updated.` and exit code 2. In both cases, print one line per ref to go back: `flatpak update --commit=<previous commit> <ref>`.

Names are Flatpak IDs of apps or runtimes, as in `nixup update co.hyprlab.Hylki` or `nixup update org.gnome.Platform`, the names `check` shows. An app installed from a bundle whose remote is disabled, such as a `.flatpak` bundle built by one of your packages, never has an update in Flatpak. It is updated through its package.

### nix profile

The `nix profile` step, in order:

1. Read `nix profile list --json`. With names, keep only those entries.
2. Leave out every entry pinned to a revision (`github:NixOS/nixpkgs/<40 hex characters>`), because `nix profile upgrade` cannot move it. For each one print `<name> is pinned to <short rev>: nix profile upgrade cannot move it. To update it, nix profile remove <name> and install it again.`
3. If no entry left follows a moving reference (such as `flake:nixpkgs`, or `github:owner/repo` without a revision), stop. Without names it first prints `No nix profile entry follows a moving reference: nothing to upgrade.`
4. Ask `Upgrade 2 nix profile entries (a, b) now? This installs them right away. [y/N]`, unless `-y`. A no prints `Skipped.`
5. Run `nix profile upgrade` with those entry names.
6. Print `nix profile upgraded. To undo it: nix profile rollback`. A failure prints `nix profile upgrade failed.` and exits with 2.

Names are `nix profile` entry names, as `nix profile list` shows them.

### Activation

Without `--test`, `--switch` or `--boot`, `update` activates nothing. With one of them, it hands the result to nh at the end. Only one is allowed: `--test --switch` stops with `--test and --switch: pick one` and exit code 2. In order:

1. Before any step, pick the host: the one given with `-H`, or the only entry in `nixosConfigurations`. With several hosts and no `-H`, or with more than one `-H`, stop before doing any work: `--test activates one host: name it with -H (this flake has desktop, laptop)`, exit code 2.
2. Run the four steps as usual. The input step builds that host in the scratch copy before asking to write `flake.lock`, as with `--build`.
3. If a step failed, print `Not activating: a step failed.` and stop. The exit code stays 2.
4. If neither `flake.lock` nor a package was written, because you answered no or everything was up to date, print `Nothing was written: nothing to activate.` and exit with 0. Flatpak and `nix profile` updates do not count: they are already installed.
5. Print `Activating desktop (nh os test)…` and run `nh os test <repo> -H desktop`. nh builds the system, asks for your password through sudo and activates it. If the input step already built that host, the build comes from the Nix store. If nh fails, nixup prints `nh os test failed.` and exits with 2.

nixup has no activation code of its own: `test`, `switch` and `boot` mean what they mean in nh. With `-y` and `--switch` nixup asks nothing: an unattended run writes the updates and switches the system, and only sudo may still ask for a password.

Exit codes. The worst code of the four steps, and of all the packages, wins:

| Code | Meaning |
|---|---|
| 0 | Everything was updated, you answered no, or there was nothing to update. |
| 2 | Something failed: a name or `--source` was wrong, more than one of `--test`, `--switch` and `--boot` was given, there was not exactly one host to activate, a check failed, or `nix flake update`, listing hosts, a build, `nix-update`, writing the file, `flatpak update`, `nix profile upgrade` or `nh os test`, `switch` or `boot` failed. |

## list

Lists everything nixup watches, with the version you have now. It reads `flake.lock`, evaluates your own packages and asks `nix profile` and `flatpak` what is installed. Nothing goes to the network, so it is quick; `check --all` gives the same list with upstream versions.

```bash
nixup list
nixup list --json
```

| Flag | Description |
|---|---|
| `-C <dir>` | Flake directory. |
| `--source <list>` | Comma-separated sources: `input`, `package`, `flatpak`, `profile`. Default: all. |
| `--json` | A JSON array of sources, each with `source`, `items` (`name`, `version`, `note`, the origin) and, if it failed, `error`. |

The output is one table, `SOURCE`, `NAME`, `VERSION` and `ORIGIN`, and a count per source:

```text
SOURCE   NAME                  VERSION        ORIGIN
input    home-manager          4839ed6 · 20d  github:nix-community/home-manager
input    nixpkgs               0bb2eed · 8d   github:NixOS/nixpkgs/nixos-unstable
package  ripgrep               14.0.0         github:BurntSushi/ripgrep
flatpak  md.obsidian.Obsidian  1.15.0         flathub

2 inputs · 1 package · 1 flatpak
```

Inputs show the tag they are pinned to, or the locked commit and its age. Origins use flake syntax (`github:owner/repo/ref`); for a `nix profile` entry it is its flake without the pinned revision, for a Flatpak app its remote. Exit code 0, or 2 if a source failed.

## search

Looks a name up in every place a package can come from, and marks in `INSTALLED` with a green `●` what is in the running system.

```bash
nixup search ripgrep
nixup search obsidian --limit 20
nixup search hex --json
```

| `SOURCE` | What is searched |
|---|---|
| `nixpkgs` | The nixpkgs revision your `flake.lock` pins, so versions are the ones you would install today. The revision is in the summary line. Needs an input called `nixpkgs`. |
| `package` | Your flake's own `packages.<system>`. |
| `input:<name>` | The `packages.<system>` output of each input. |
| `flatpak` | Every Flatpak remote you have configured, usually Flathub. |

```text
SOURCE   NAME                      VERSION  INSTALLED  DESCRIPTION
nixpkgs  ripgrep                   15.2.0   ●          Utility that combines the usability of The Silver Searcher with the raw…
nixpkgs  ripgrep-all               0.10.10             Ripgrep, but also search in PDFs, E-Books, Office documents, zip, tar.g…
flatpak  de.leopoldluley.Clapgrep  26.09               Search through all your files

3 results (nixpkgs 0bb2eed)
```

The query matches package names (attribute names, Flatpak IDs and names), not descriptions, without regard to case. Exact matches come first, then names that start with it, then the rest. It can contain letters, digits and `. _ + -`.

| Flag | Description |
|---|---|
| `-C <dir>` | Flake directory. |
| `--limit <n>` | Results shown per source (default 10). How many were left out is printed. |
| `--source <list>` | Comma-separated sources: `nixpkgs`, `package`, `input`, `flatpak`. `input` covers every input. Sources not asked for are not searched at all. |
| `--json` | Same shape as `list --json` (`note` is the description), plus `rev` for nixpkgs, `more` (results left out) and `in_system`. |

`●` means a store path with the same name and version is in `/run/current-system`. On a machine without it (CI, a container) nothing is marked.

| Code | Meaning |
|---|---|
| 0 | Something was found. |
| 1 | Nothing was found. |
| 2 | The query is not a package name, or the flake could not be evaluated. |

## version and help

```bash
nixup version      # also: nixup --version, nixup -v
nixup help         # also: nixup --help, nixup -h
```

`version` prints `nixup <version> (<commit>)` for builds from the flake, and `nixup dev` for a plain `go build`. `help` prints the command list on stdout. Both exit with 0.

## completion

```bash
nixup completion zsh    # or bash, fish
```

Prints a completion script for the shell. The Nix package installs all three, so with nixup in `environment.systemPackages` they work in a new shell. Elsewhere, load one by hand:

```bash
source <(nixup completion bash)    # in ~/.bashrc
source <(nixup completion zsh)     # in ~/.zshrc, after compinit
nixup completion fish | source     # in ~/.config/fish/config.fish
```

TAB completes commands, flags with their description, the values of `--source` (also as a comma list), `-H` with the hosts of the flake and `-C` with directories. After `nixup update` it offers the names `update` takes: the flake's inputs and packages, the installed Flatpak apps and runtimes and the `nix profile` entries, each with where it comes from, limited by `--source` and without the names already typed. The flake is the one `-C` names on the line, else the usual one.

The scripts only call `nixup __complete <words>`, which prints the candidates, so the three shells offer the same. Reading the names takes `nix eval .#packages.<system> --apply builtins.attrNames`, which nix answers from its evaluation cache, plus `flatpak list` and `nix profile list`.

## Errors on the command line

An unknown command prints `nixup: unknown command "<name>"` and the usage on stderr, and exits with 2. An unknown flag prints the flag list of the command and exits with 2. If you typed no command, nixup also says that `check` was assumed:

```text
$ nixup --bogus
flag provided but not defined: -bogus
Usage of check:
  ...
nixup: no command given, 'check' was assumed (see 'nixup help' for the others)
```
