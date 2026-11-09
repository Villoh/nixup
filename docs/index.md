---
title: nixup documentation
description: Update checker and safe updater for flake-based NixOS. Overview and map of the documentation.
lang: en
---

# nixup documentation

English | [Español](es/index.md)

nixup is a command-line tool for people who manage NixOS with a flake. It answers two questions: what is out of date, and what changed upstream since the version you run. Then it updates `flake.lock` and your own packages, but only after a build in a safe place and a yes from you.

It does not activate anything. There is no `switch`, `boot` or `sudo` in the code, no daemon and no timer. After nixup writes a file, you review the diff, test the system with your usual tools and commit.

## What it checks

| Section | What it compares | How you update it |
|---|---|---|
| `inputs` | Branch inputs against the remote branch head; inputs pinned to a release tag against the newest tag | `nixup lock`, or edit `flake.nix` for a pinned tag |
| `packages` | Each package in your flake's own `packages.<system>` output against its newest GitHub tag or release | `nixup pkg` (runs `nix-update`) |
| `profile` | `nix profile` entries pinned to a different nixpkgs revision than your flake's `nixpkgs` input | `nix profile upgrade` |
| `flatpak` | Applications and runtimes with a pending update | `flatpak update` |

## The commands

- `nixup check` lists what is outdated. It is the default command and it is read-only.
- `nixup lock` updates `flake.lock` in a scratch copy, shows what moved upstream, builds every host and writes the new lock after you confirm.
- `nixup pkg` updates the packages your flake defines, one at a time, after showing what changed upstream.

## Pages

- [Getting started](getting-started.md): requirements, install options and a first run.
- [Commands](commands.md): every command, flag, output format and exit code.
- [Configuration](configuration.md): how nixup finds your flake, environment variables, GitHub token, required tools.
- [How it works](how-it-works.md): what each section checks, how the upstream review is built, what is written and what never is.
- [Safety](safety.md): the guarantees and their limits.
- [CI](ci.md): running nixup from a scheduled GitHub Actions workflow.
- [Limitations](limitations.md): what nixup does not see or do.
- [Development](development.md): code layout, tests and the testing approach.
- [Releases](releases.md): how maintainers cut a release.

The project lives at [github.com/Villoh/nixup](https://github.com/Villoh/nixup). The short version of all this is in the [README](../README.md).
