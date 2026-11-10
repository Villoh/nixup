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
```

Without a command nixup runs `check`, so `nixup`, `nixup --review` and `nixup check --review` do the same thing.

Flags can go before, between or after names: `nixup update nixpkgs -y home-manager` updates two inputs without asking. Flags take one dash or two (`-y`, `--review`, `--source=input`).

Every command that works on a flake accepts `-C <dir>`. See [Configuration](configuration.md) for how the flake is found when `-C` is not given.

`nixup <command> -h` prints that command's flags on stderr and exits with 0.

## Output streams

- Results, reviews and questions go to stdout.
- Errors go to stderr.
- The output of `nix flake update`, `nh os build`, `nix-update`, `nix build`, `flatpak update` and `nix profile upgrade` is streamed to stderr.

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
| `--review` | Add the upstream review (commits, compare link, release notes) to every outdated entry. |
| `--json` | Print every row as JSON, up-to-date ones included. Takes precedence over `--markdown`. |
| `--markdown` | Print only outdated and failed entries as Markdown, for a pull request body or a CI summary. Prints `Everything is up to date.` when there is nothing. |

`check` takes no names; extra words after it are ignored.

The text output is one table: `SOURCE`, `NAME`, `CURRENT`, `LATEST`, `STATUS` and `ACTION`, followed by a summary line and, with `--review`, one block of upstream changes per update. `STATUS` is `update`, `major` or `0.x-minor` (likely breaking), `ok`, `info` or `error`; see [Getting started](getting-started.md#reading-the-report). An `ok` row repeats its version under `LATEST`, and the summary counts the `ok` rows even when they are hidden. In JSON the statuses are `outdated`, `ok`, `info` and `error`, and a likely breaking jump has a `warning`:

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

`latest`, `note`, `warning`, `hint` and `review` are left out when empty. `hint` says that the flake's nixpkgs already has the package (see [How it works](how-it-works.md#packages)). With `--review`, `review` holds `repo`, `from`, `to`, `compare_url`, `commits`, `subjects`, `releases` (each with `tag` and `body`), `skipped_prereleases` and, if the GitHub API failed, `error`.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Everything checked is up to date. |
| 1 | At least one entry is outdated. |
| 2 | At least one check failed, or the command line or flake location was wrong. A failure wins over outdated entries. |

## update

Reviews and applies updates in four steps, in this order: the flake's inputs (`flake.lock`), the packages your flake defines in its own `packages.<system>` output, Flatpak, and `nix profile`.

The input and package steps only edit files in your repository; you apply them to the system with nh. The Flatpak and `nix profile` steps install on the machine right away, after one question each, and print how to undo it.

```bash
nixup update
nixup update nixpkgs home-manager
nixup update ripgrep --no-build
nixup update co.hyprlab.Hylki
nixup update --source input -H desktop -H laptop
nixup update --source package -y --report report.md
nixup update --source flatpak
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
| `-H <host>` | Host to build in the input step. Repeatable. Default: every entry in `nixosConfigurations`. |
| `--no-build` | Update and show the changes, but do not build: no `nh os build` for inputs, no `nix build` for packages. The Flatpak and `nix profile` steps build nothing either way. |
| `-y` | Write the new lock, update the packages, install the Flatpak updates and upgrade the `nix profile` entries without asking. Needed for unattended runs. |
| `--report <file>` | Append a Markdown report (moved inputs, reviews, build results, one block per updated package, the Flatpak table with the commands to go back, the upgraded `nix profile` entries) to this file. Created if missing. |

### Inputs

The input step, in order:

1. Refuse to start if `flake.lock` has uncommitted changes. The other steps still run.
2. Copy the working tree to a temporary directory and run `nix flake update --flake <copy> [input...]` there.
3. If nothing changed, print `flake.lock is already up to date.` and stop.
4. For every direct input that moved, print the old and new commit and the upstream commits. Inputs that are not direct are counted on one line.
5. Unless `--no-build`, run `nh os build <copy> -H <host>` for each host. The first failure stops the step and the repository is left as it was.
6. Append the report, if `--report` was given.
7. Ask `Write this flake.lock to the repo? [y/N]`, unless `-y`. Then copy the new `flake.lock` into the repository.

### Packages

The package step goes through the packages one at a time. For each outdated package nixup:

1. Prints the current and new version, the major-jump warning if any, and the upstream review.
2. Asks `Update <name> to <version>? [y/N]`, unless `-y`. A no prints `Skipped.` and moves on.
3. Refuses this package if `<pkg dir>` has uncommitted changes, and moves on.
4. Runs `nix-update --flake <name> --version=<the reviewed version>`, plus the extra arguments from the package's `passthru.updateScript`.
5. Unless `--no-build`, runs `nix build --no-link .#<name>`.
6. If step 4 or 5 fails, restores `<pkg dir>` with `git checkout`. Otherwise prints `git diff -- <pkg dir>`.

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

Exit codes. The worst code of the four steps, and of all the packages, wins:

| Code | Meaning |
|---|---|
| 0 | Everything was updated, you answered no, or there was nothing to update. |
| 1 | Something was refused because of uncommitted changes: `flake.lock` or a package directory. Commit or stash, then run again. |
| 2 | Something failed: a name or `--source` was wrong, a check failed, or `nix flake update`, listing hosts, a build, `nix-update`, writing the file, `flatpak update` or `nix profile upgrade` failed. |

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

## Errors on the command line

An unknown command prints `nixup: unknown command "<name>"` and the usage on stderr, and exits with 2. An unknown flag prints the flag list of the command and exits with 2. If you typed no command, nixup also says that `check` was assumed:

```text
$ nixup --bogus
flag provided but not defined: -bogus
Usage of check:
  ...
nixup: no command given, 'check' was assumed (see 'nixup help' for the others)
```
