---
title: Limitations
description: What nixup does not see, check or do.
lang: en
---

# Limitations

**English** | [Español](es/limitations.md)

## Only GitHub upstreams

- Inputs: only direct inputs of type `github` are checked. `gitlab:`, `git+https:`, `path:`, tarball and other inputs do not appear in the report at all.
- Packages: a package whose source URLs and homepage do not point to github.com is reported as `info` with `no GitHub upstream: check by hand`.
- Reviews (commits, compare links, release notes) come from the GitHub API only.

## Only what the flake exposes

- The `packages` section reads the flake's own `packages.<system>` output, for the system nixup runs on. A `fetchurl` inside a module, a pinned `rev` in an overlay, or a package defined only for another system is invisible. Move it into the `packages` output to have it checked.
- A package needs `passthru.updateScript` to be checked. Without it nixup reports `info` with `no passthru.updateScript`.
- `nixup update` guards and restores the directory of the file that defines each package, from its `meta.position`. A package without `meta.position`, or defined outside this flake, is refused.
- Transitive inputs are not checked. `update` updates them with everything else and only reports how many moved.

## update builds NixOS hosts only

The input step of `update` builds each entry in `nixosConfigurations` with `nh os build`. Home Manager, nix-darwin and other outputs are not built. On a flake without `nixosConfigurations`, run `update --source input --no-build` (and `update --source package` for the packages, which are still built); otherwise listing the hosts fails with exit code 2.

## What update cannot move or review

- Pinned `nix profile` entries: an entry pinned to a revision (`github:NixOS/nixpkgs/<rev>`) cannot be moved by `nix profile upgrade`, so `update` never upgrades it. Run `nix profile remove <name>` and install it again. `check` compares those revisions only with your flake's input named `nixpkgs`.
- Flatpak has no changelog review: the Flatpak step shows versions, not commits or release notes. `LATEST` says `unknown` when the remote publishes no versions, as with a remote added from a `.flatpakref` file.
- An app installed from a bundle whose remote is disabled, such as a `.flatpak` bundle built by your own package, never has an update in Flatpak. Update it through its package.
- Pinned tag inputs: `nix flake update` does not move a tag, so `update` does not either. Edit the tag in `flake.nix`.

## Version heuristics

- A `ref` that starts with a digit, `v` plus a digit, or a `word/` prefix followed by those (`v1.2`, `1.2`, `dms/1.2`) is treated as a release tag. Everything else is a branch.
- Date-based versions such as `20240101.1` get no major-jump warning: a first number of 1000 or more is read as a year or a date, so a real major version that high would be missed too.
- The nixpkgs hint only finds a package under its own name or, for `foo-bin`, as `foo`. A package called differently in nixpkgs is missed. It also needs proof that both are the same project (repository or homepage), so a namesake is never suggested, but neither is the real thing if its metadata differs.
- Release notes are read from the 50 most recent GitHub releases, and nixup shows at most three per update.

## search

- Only names are searched, not descriptions: searching descriptions means evaluating every package in nixpkgs, which takes about half a minute (`nix search` does that).
- Only top-level attributes: `python3Packages.requests` and other package sets are not searched.
- `●` compares names and versions with the running system's closure. A library pulled in by something else counts as in the system, and a Home Manager setup outside NixOS is not looked at.
- `search` evaluates the flake with `--impure`, because `builtins.getFlake` on a local path needs it. It reads; it builds and writes nothing.

## Other limits

- nixup does not compute hashes. `nix-update` does.
- There is no configuration file, daemon, timer or notification. Schedule it yourself, for example [in CI](ci.md).
- Without a GitHub token the API allows 60 requests per hour, which a large `--review` can exceed.
- The flake provides builds for `x86_64-linux` and `aarch64-linux` only.
