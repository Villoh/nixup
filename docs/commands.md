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
nixup lock [input...] [flags]
nixup pkg [package...] [flags]
nixup list [flags]
nixup search <name> [flags]
nixup version
nixup help
```

Without a command nixup runs `check`, so `nixup`, `nixup --review` and `nixup check --review` do the same thing.

Flags can go before, between or after names: `nixup lock nixpkgs -y home-manager` updates two inputs without asking. Flags take one dash or two (`-y`, `--review`, `--source=input`).

Every command that works on a flake accepts `-C <dir>`. See [Configuration](configuration.md) for how the flake is found when `-C` is not given.

`nixup <command> -h` prints that command's flags on stderr and exits with 0.

## Output streams

- Results, reviews and questions go to stdout.
- Errors go to stderr.
- The output of `nix flake update`, `nh os build`, `nix-update` and `nix build` is streamed to stderr.

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
| `--source <list>` | Comma-separated sources to check: `input`, `package`, `profile`, `flatpak` (plurals work too). Default: all. An unknown name is an error. |
| `--all` | Also list up-to-date entries in the text output. Without it they are only counted. |
| `--review` | Add the upstream review (commits, compare link, release notes) to every outdated entry. |
| `--json` | Print every row as JSON, up-to-date ones included. Takes precedence over `--markdown`. |
| `--markdown` | Print only outdated and failed entries as Markdown, for a pull request body or a CI summary. Prints `Everything is up to date.` when there is nothing. |

`check` takes no names; extra words after it are ignored.

The text output is one table: `SOURCE`, `NAME`, `CURRENT`, `LATEST`, `STATUS` and `ACTION`, followed by a summary line and, with `--review`, one block of upstream changes per update. `STATUS` is `update`, `major` or `0.x-minor` (likely breaking), `ok`, `info` or `error`; see [Getting started](getting-started.md#reading-the-report). In JSON the statuses are `outdated`, `ok`, `info` and `error`, and a likely breaking jump has a `warning`:

```json
[
  {
    "section": "packages",
    "name": "ripgrep",
    "current": "14.0.0",
    "latest": "15.2.0",
    "status": "outdated",
    "note": "nixup pkg ripgrep",
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

## lock

Updates `flake.lock` in a scratch copy, shows what moved, builds the hosts there and writes the new lock to your repository after you confirm.

```bash
nixup lock
nixup lock nixpkgs home-manager
nixup lock -H desktop -H laptop
nixup lock --no-build
nixup lock -y --no-build --report report.md
```

Names are flake inputs; without names every input is updated.

| Flag | Description |
|---|---|
| `-C <dir>` | Flake directory. |
| `-H <host>` | Host to build. Repeatable. Default: every entry in `nixosConfigurations`. |
| `--no-build` | Update and show the changes, but do not build. |
| `-y` | Write the new lock without asking. Needed for unattended runs. |
| `--report <file>` | Append a Markdown report (moved inputs, reviews, build results) to this file. Created if missing. |

The steps, in order:

1. Refuse to start if `flake.lock` has uncommitted changes.
2. Copy the working tree to a temporary directory and run `nix flake update --flake <copy> [input...]` there.
3. If nothing changed, print `flake.lock is already up to date.` and stop.
4. For every direct input that moved, print the old and new commit and the upstream commits. Inputs that are not direct are counted on one line.
5. Unless `--no-build`, run `nh os build <copy> -H <host>` for each host. The first failure stops the run and the repository is left as it was.
6. Append the report, if `--report` was given.
7. Ask `Write this flake.lock to the repo? [y/N]`, unless `-y`. Then copy the new `flake.lock` into the repository.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | The lock was written, you answered no, or there was nothing to update. |
| 1 | `flake.lock` has uncommitted changes. Commit or stash it, then run again. |
| 2 | Something failed: `nix flake update`, listing hosts, a build, or writing the file. |

## pkg

Updates the packages your flake defines in its own `packages.<system>` output, one at a time, after showing what changed upstream.

```bash
nixup pkg
nixup pkg ripgrep
nixup pkg ripgrep --no-build
nixup pkg -y --report report.md
```

Names are package attribute names; without names every package is considered. An unknown name is an error.

| Flag | Description |
|---|---|
| `-C <dir>` | Flake directory. |
| `--no-build` | Do not run `nix build` after `nix-update`. |
| `-y` | Update without asking. Needed for unattended runs. |
| `--report <file>` | Append a Markdown report per updated package to this file. Created if missing. |

For each outdated package nixup:

1. Prints the current and new version, the major-jump warning if any, and the upstream review.
2. Asks `Update <name> to <version>? [y/N]`, unless `-y`. A no prints `Skipped.` and moves on.
3. Refuses this package if `packages/<name>` has uncommitted changes, and moves on.
4. Runs `nix-update --flake <name> --version=<the reviewed version>`, plus the extra arguments from the package's `passthru.updateScript`.
5. Unless `--no-build`, runs `nix build --no-link .#<name>`.
6. If step 4 or 5 fails, restores `packages/<name>` with `git checkout`. Otherwise prints `git diff -- packages/<name>`.

Packages that are up to date are listed only when you name them. Entries marked `info` or `ERROR` are always listed. If you name no packages and everything is current, `pkg` prints nothing.

Exit codes. With several packages, the highest code wins:

| Code | Meaning |
|---|---|
| 0 | Every package was updated, skipped or already current. |
| 1 | A package directory had uncommitted changes. |
| 2 | A check failed, a name does not exist, or `nix-update` or the build failed. |

## list

Lists everything nixup watches, with the version you have now. It reads `flake.lock`, evaluates your own packages and asks `nix profile` and `flatpak` what is installed. Nothing goes to the network, so it is quick; `check --all` gives the same list with upstream versions.

```bash
nixup list
nixup list --json
```

| Flag | Description |
|---|---|
| `-C <dir>` | Flake directory. |
| `--source <list>` | Comma-separated sources: `input`, `package`, `profile`, `flatpak`. Default: all. |
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
