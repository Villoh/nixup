---
title: Getting started
description: Install nixup, point it at your NixOS flake and read the first report.
lang: en
---

# Getting started

English | [Español](es/getting-started.md)

## Requirements

- A NixOS configuration in a flake, inside a git repository, with `flake.nix` and `flake.lock` committed.
- `git` and `nix` on `PATH`.
- `nh` if you want `nixup lock` to build your hosts. With `--no-build` it is not needed.
- `nix-update` for `nixup pkg`. Without it, nixup runs `nix run nixpkgs#nix-update --` instead.
- `flatpak` is optional. Without it the `flatpak` section is skipped.
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
inputs
  UPDATE home-manager               4839ed6 (20d) -> 2c0c1f0       nixup lock home-manager
  UPDATE lanzaboote                 v0.4.1 -> v1.2.0               pinned tag: edit flake.nix
         ⚠ MAJOR version jump: read the changelog, expect breaking changes
  UPDATE nixpkgs                    0bb2eed (8d) -> f4a03c0        nixup lock nixpkgs
  1 up to date (use --all to list)

packages
  UPDATE ripgrep                    14.0.0 -> 15.2.0               nixup pkg ripgrep
         ⚠ MAJOR version jump: read the changelog, expect breaking changes
```

Each line has a status, a name, the current and newest version, and the command that applies the update.

- `UPDATE`: a newer version exists.
- `ok`: up to date. These lines are only counted; `--all` lists them.
- `info`: nixup cannot check this entry, and the note says why (for example `no GitHub upstream: check by hand`).
- `ERROR`: the check failed, and the note has the reason.

For branch inputs the current column shows the locked commit and how many days old it is: `4839ed6 (20d)` was committed upstream 20 days ago.

The `⚠` line appears when a jump looks breaking: a new major version, or a new minor version while still at `0.x`.

## Read what changed

```bash
nixup check --review
```

Under every `UPDATE` line this adds the source repository, the number of commits with a compare link, the last commit subjects and, for tagged versions, the release notes in between.

## Apply updates

```bash
nixup lock                 # all inputs
nixup lock home-manager    # one input
nixup pkg ripgrep          # one of your own packages
```

`lock` shows the upstream commits for every input that moved, builds each host from a scratch copy and asks before it writes `flake.lock`. `pkg` asks before each package. Neither commits nor activates anything. When `lock` finishes it prints the next steps:

```text
flake.lock updated (not committed, nothing activated).
Next: git diff flake.lock && nix flake check && nh os test . -H <host>
```

Pinned tags (`lanzaboote` above) are not moved by `nix flake update`. Change the tag in `flake.nix` yourself, then run `nixup lock lanzaboote`.

Next: [Commands](commands.md) for every flag, or [How it works](how-it-works.md) for what happens underneath.
