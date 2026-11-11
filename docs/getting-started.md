---
title: Getting started
description: Install nixup, point it at your NixOS flake and read the first report.
lang: en
---

# Getting started

**English** | [Español](es/getting-started.md)

## Requirements

- A NixOS configuration in a flake, inside a git repository, with `flake.nix` and `flake.lock` committed.
- `git` and `nix` on `PATH`.
- `nh` if you want `nixup update` to build your hosts. With `--no-build` it is not needed.
- `nix-update` for updating your own packages with `nixup update`. Without it, nixup runs `nix run nixpkgs#nix-update --` instead.
- `flatpak` is optional. Without it the `flatpak` section and the Flatpak step of `nixup update` are skipped.
- Network access to github.com and api.github.com.

The flake ships builds for `x86_64-linux` and `aarch64-linux`. See [Configuration](configuration.md) for the full list of tools and what each one is used for.

## Install

Run it once without installing anything:

```bash
nix run github:Villoh/nixup -- check
```

To install it on your system, add it as a flake input:

```nix
{
  inputs.nixup = {
    url = "github:Villoh/nixup";
    inputs.nixpkgs.follows = "nixpkgs";
  };
}
```

Then add the package in a NixOS module. This assumes you pass `inputs` to your modules through `specialArgs`:

```nix
{ inputs, pkgs, ... }:
{
  environment.systemPackages = [
    inputs.nixup.packages.${pkgs.stdenv.hostPlatform.system}.default
  ];
}
```

The flake also has an overlay that adds `pkgs.nixup`:

```nix
{ inputs, pkgs, ... }:
{
  nixpkgs.overlays = [ inputs.nixup.overlays.default ];
  environment.systemPackages = [ pkgs.nixup ];
}
```

The package also installs completions for bash, zsh and fish: in a new shell, `nixup update <TAB>` offers your inputs, packages and Flatpak apps. See [completion](commands.md#completion).

Check the install:

```bash
nixup version
```

## First run

Go to your flake and run `nixup` with no arguments. It is the same as `nixup check`:

```bash
cd ~/nixos-config
nixup
```

You do not have to be inside the flake. `nixup -C ~/nixos-config` works from anywhere, and if `nh` already knows your flake through `NH_FLAKE` or `NH_OS_FLAKE`, nixup uses the same one. See [Configuration](configuration.md).

## Reading the report

```text
SOURCE   NAME          CURRENT        LATEST   STATUS
input    home-manager  4839ed6 · 20d  2c0c1f0  update
input    lanzaboote    v0.4.1         v1.2.0   major
input    nixpkgs       0bb2eed · 8d   f4a03c0  update
package  ripgrep       14.0.0         15.2.0   major

4 to update (2 likely breaking) · 1 up to date (--all to show them)
```

One row per entry: where it comes from (`SOURCE`: `input`, `package`, `flatpak`, `profile`), its name, the current and newest version, and a status. Every update is applied with `nixup update <name>` (or `nixup update` for all), so the table does not repeat it; a `NOTE` column appears only for rows that need something else. The last line counts it all.

`STATUS` is one of:

- `update`: a newer version exists.
- `major` or `0.x-minor`: a newer version exists and the jump looks breaking, a new major version or a new minor version while still at `0.x`. Read the changelog first.
- `ok`: up to date. These rows are only counted; `--all` lists them.
- `info`: nixup cannot check this entry, and `NOTE` says why (for example `no GitHub upstream: check by hand`).
- `error`: the check failed, and `NOTE` has the reason.

For branch inputs `CURRENT` shows the locked commit and how many days old it is: `4839ed6 · 20d` was committed upstream 20 days ago.

## Read what changed

```bash
nixup check --review
```

After the table, for every update, this adds the source repository, the number of commits with a compare link, the last commit subjects and, for tagged versions, the release notes in between.

## Apply updates

```bash
nixup update                  # inputs, your own packages, Flatpak, nix profile
nixup update --source input   # only the inputs
nixup update home-manager     # one input
nixup update ripgrep          # one of your own packages
nixup update co.hyprlab.Hylki # one Flatpak app
```

For the inputs, `update` shows the upstream commits for every input that moved, builds each host from a scratch copy and asks before it writes `flake.lock`. Then it asks before each of your own packages. These two steps only edit files in your repository, and nixup neither commits them nor activates a generation.

Last come Flatpak and `nix profile`. Each shows what it would update and asks once, and the question says `This installs them right away.` On yes they install, then print how to go back: one `flatpak update --commit=<previous commit> <ref>` line per Flatpak ref, or `nix profile rollback`. Use `--source input,package` to leave the machine alone.

When `flake.lock` or a package was written, `update` ends with one line on how to apply it. The path is `.` from inside the repository, and the host is filled in when the flake has only one or you passed `-H`:

```text
Next: review with git diff, then nh os test . -H desktop (nothing is committed or activated)
```

Flatpak and `nix profile` updates are already installed, so they print no such line.

Pinned tags (`lanzaboote` above) are not moved by `nix flake update`, so nixup moves them itself. In the scratch copy it replaces the tag in `flake.nix` with the newest release, and after your yes it writes `flake.nix` together with `flake.lock`. A plain `nixup update` leaves a likely breaking jump, such as `v0.4.1` to `v1.2.0`, where it is and says so. `nixup update lanzaboote` moves it.

Next: [Commands](commands.md) for every flag, or [How it works](how-it-works.md) for what happens underneath.
