**English** | [Español](README.es.md)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/header-dark.svg">
  <img alt="nixup" src="brand/header-light.svg">
</picture>

nixup checks a flake-based NixOS configuration for updates and applies them only after you have read what changed upstream. It compares your flake inputs, the packages your flake defines, `nix profile` entries and Flatpak apps with their upstream. For each pending update it shows the commits and release notes in between, then builds in a scratch copy before it writes anything. It never activates a system: no `switch`, no `boot`, no `sudo`, no daemon. Written in Go, standard library only.

## Example

```text
$ nixup check --only inputs,packages

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

Add `--review` to see, under every `UPDATE` line, the source repository, the commit count with a compare link, the latest commit subjects and, for tagged versions, the release notes in between.

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
nixup lock               # update flake.lock in a scratch copy, build, ask, write
nixup lock nixpkgs       # only that input
nixup pkg                # update your own packages one by one, after reviewing each
```

Run it inside the flake, pass `-C <dir>`, or let it read `NIXUP_FLAKE`, `NH_OS_FLAKE` or `NH_FLAKE`.

## How it stays safe

- `check` only reads. It writes nothing to your repository.
- `lock` runs `nix flake update` and the host builds in a copy of your working tree. Your `flake.lock` changes only after the build passes and you answer yes.
- `pkg` runs `nix-update` pinned to the version you just reviewed, then builds the package. If either step fails, `packages/<name>` goes back to its committed state.
- Both refuse to touch files that have uncommitted changes.
- Without a terminal every question is answered no, so unattended runs must pass `-y`.
- nixup never commits and never activates a generation. Testing and switching stay with you.

## Documentation

| Page | What it covers |
|---|---|
| [Getting started](docs/getting-started.md) | Requirements, install options, first run |
| [Commands](docs/commands.md) | Every flag and exit code |
| [Configuration](docs/configuration.md) | Which flake, environment variables, GitHub token |
| [How it works](docs/how-it-works.md) | What each section checks and what gets written |
| [Safety](docs/safety.md) | The guarantees and where they end |
| [CI](docs/ci.md) | Scheduled checks and update pull requests |
| [Limitations](docs/limitations.md) | What nixup does not see |
| [Development](docs/development.md) | Code layout and tests, for contributors |
| [Releases](docs/releases.md) | How a release is cut, for maintainers |

MIT licensed, see [LICENSE](LICENSE).
