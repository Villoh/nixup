<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/header-dark.svg">
  <img alt="nixup" src="brand/header-light.svg" width="420">
</picture>

Safe updates for flake-based NixOS. Review first, then decide.

[![CI](https://img.shields.io/github/actions/workflow/status/Villoh/nixup/ci.yml?branch=main&label=ci)](https://github.com/Villoh/nixup/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Villoh/nixup?color=4A6FC4)](https://github.com/Villoh/nixup/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/Villoh/nixup?color=4A6FC4)](go.mod)
[![License: MIT](https://img.shields.io/github/license/Villoh/nixup?color=4A6FC4)](LICENSE)

**English** | [Español](README.es.md)

</div>

---

nixup checks a flake-based NixOS configuration for updates and applies them only after you have read what changed upstream. It compares your flake inputs, the packages your flake defines, `nix profile` entries and Flatpak apps with their upstream. For each pending update it shows the commits and release notes in between, then updates `flake.lock` in a scratch copy and asks before it writes anything. With `--build` it builds your hosts in that copy before asking. Your own packages are always built after their update. It can also install pending Flatpak updates and upgrade `nix profile` entries, after asking. It never activates a NixOS generation unless you pass `--test`, `--switch` or `--boot`, and then nh does it: nixup has no activation code, no `sudo`, no daemon. Written in Go, standard library only.

## Example

```text
$ nixup check --source input,package
SOURCE   NAME          CURRENT        LATEST   STATUS
input    home-manager  4839ed6 · 20d  2c0c1f0  update
input    lanzaboote    v0.4.1         v1.2.0   major
input    nixpkgs       0bb2eed · 8d   f4a03c0  update
package  ripgrep       14.0.0         15.2.0   major

4 to update (2 likely breaking) · 1 up to date (--all to show them)
```

Add `--review` to see, after the table and for every update, the source repository, the commit count and size of the change with a compare link, the latest commit subjects and, for tagged versions, the release notes in between.

## Install

Try it without installing:

```bash
nix run github:Villoh/nixup -- check
```

Or add it to your flake and put `inputs.nixup.packages.${system}.default` in `environment.systemPackages`:

```nix
{
  inputs.nixup = {
    url = "github:Villoh/nixup";
    inputs.nixpkgs.follows = "nixpkgs";
  };
}
```

## Quick start

```bash
nixup                    # same as `nixup check`: what is outdated, read-only
nixup check --review     # plus upstream commits and release notes
nixup update             # inputs: update flake.lock in a scratch copy, show what moved, ask, write;
                         # then your own packages one by one, after reviewing each;
                         # then Flatpak and nix profile, one question each
nixup update nixpkgs     # only that input
nixup update ripgrep     # only that package
nixup update co.hyprlab.Hylki   # only that Flatpak app
nixup update --source input,package   # only what lives in the repository
nixup update --build     # build every host in the scratch copy before asking
nixup update --test      # build the host first, then nh os test if flake.lock or a package was written
nixup list               # everything it watches, with current versions, offline
nixup search ripgrep     # in your nixpkgs, your packages, your inputs and Flatpak
```

It works on the flake you are in; anywhere else it reads `NIXUP_FLAKE`, `NH_OS_FLAKE` or `NH_FLAKE` (the last two are nh's). `-C <dir>` overrides both.

## How it stays safe

- `check` only reads. It writes nothing to your repository.
- `update` runs `nix flake update` in a copy of your working tree. Your `flake.lock` changes only after you answer yes. With `--build`, `--test`, `--switch` or `--boot` the hosts are built in that copy first, and a failed build leaves `flake.lock` untouched. Without them the hosts are not built, so a `flake.lock` that does not build can be written; `git checkout flake.lock` takes it back. Use `update --test` on your own machine.
- For your own packages, `update` runs `nix-update` pinned to the version you just reviewed, then builds the package. If either step fails, the package's files go back to how they were, uncommitted edits included.
- `update` keeps uncommitted changes: it says when `flake.lock` carries some besides the update, and asks before it updates over your edits to `flake.nix` or a package.
- The input and package steps only edit files in the repository; you apply them with nh, or with `--test`, `--switch` or `--boot` nixup runs nh at the end. The Flatpak and `nix profile` steps install right away, so their question says so, and they print how to undo it (`flatpak update --commit=...`, `nix profile rollback`). A `nix profile` entry pinned to a revision is never upgraded.
- Without a terminal every question is answered no, so unattended runs must pass `-y`.
- nixup never commits, and never activates a NixOS generation unless you pass `--test`, `--switch` or `--boot`, and then nh does it. Without those flags, testing and switching stay with you.

## Documentation

| Page | What it covers |
|---|---|
| [Getting started](docs/getting-started.md) | Requirements, install options, first run |
| [Commands](docs/commands.md) | Every flag and exit code |
| [Configuration](docs/configuration.md) | Which flake, environment variables, GitHub token |
| [How it works](docs/how-it-works.md) | What each section checks and what gets written |
| [Safety](docs/safety.md) | The guarantees and where they end |
| [CI](docs/ci.md) | A weekly issue with pending updates, or update pull requests |
| [Limitations](docs/limitations.md) | What nixup does not see |
| [Development](docs/development.md) | Code layout and tests, for contributors |
| [Releases](docs/releases.md) | How a release is cut, for maintainers |

MIT licensed, see [LICENSE](LICENSE).
