---
title: Configuration
description: How nixup finds your flake, the environment variables it reads, the GitHub token and the tools it needs.
lang: en
---

# Configuration

**English** | [Español](es/configuration.md)

nixup has no configuration file. It takes a flag, a few environment variables, and the `passthru.updateScript` of your packages.

## Which flake

nixup works on one flake: a directory with both `flake.nix` and `flake.lock`. It picks the first of these that is set:

1. `-C <dir>`
2. `NIXUP_FLAKE`
3. `NH_OS_FLAKE`
4. `NH_FLAKE`
5. Otherwise, a search upwards from the current directory.

`NH_OS_FLAKE` and `NH_FLAKE` are the variables [nh](https://github.com/nix-community/nh) reads, so if `nh` already knows your flake, nixup needs no extra setup. On NixOS, `programs.nh.flake` sets `NH_FLAKE` for you:

```nix
{
  programs.nh = {
    enable = true;
    flake = "/home/alice/nixos-config";
  };
}
```

`NH_HOME_FLAKE` and `NH_DARWIN_FLAKE` are not read.

### -C

`-C <dir>` starts the upward search at `<dir>`, so `-C ~/nixos-config/hosts` finds `~/nixos-config`. It also wins over every environment variable, and nixup prints nothing extra about it.

### Environment variables

A value from `NIXUP_FLAKE`, `NH_OS_FLAKE` or `NH_FLAKE` is used as is:

- A `#host` suffix is dropped: `~/nixos-config#desktop` means `~/nixos-config`.
- A `path:` prefix is dropped.
- A leading `~` is expanded to your home directory.
- Remote references such as `github:owner/repo` are rejected, because nixup has to write to the flake.
- The directory itself must contain `flake.nix` and `flake.lock`. nixup never falls back to a parent directory or to the current one.

An invalid value is an error (exit code 2), even if a later variable would have worked:

```text
nixup: $NIXUP_FLAKE: "github:foo/bar" is not a local directory
nixup: $NH_FLAKE=/tmp: no flake.nix + flake.lock there
```

When the flake comes from a variable, nixup says so on stderr (so `--json` output stays clean) only where it matters: before `lock` and `pkg` write to it, and when you are inside a different flake, which the variable overrides. `check`, `list` and `search` run from anywhere else stay quiet.

```text
nixup: using ~/nixos-config ($NH_FLAKE)
nixup: using $NH_FLAKE, not the flake here (-C . to use this one)
```

## Color

Output is colored only when stdout is a terminal. Any non-empty `NO_COLOR` turns color off, and so does `TERM=dumb`.

## Questions

`lock` and `pkg` ask before they write. Questions are answered only when stdin is a terminal, and only `y` (any case) counts as yes. With no terminal every question is answered no, so unattended runs must pass `-y`.

## GitHub token

nixup reads tags and branch heads with `git ls-remote` over HTTPS, which needs no token. It uses the GitHub REST API for commit comparisons, release notes, and the release list of packages that follow GitHub releases. For those calls it takes the first token it finds:

1. `GH_TOKEN`
2. `GITHUB_TOKEN`
3. The output of `gh auth token`, if `gh` is installed.

Without a token the API calls are unauthenticated, and GitHub allows only 60 of those per hour. A `check --review` on a flake with many outdated entries can run out. When an API call fails, the review shows `review unavailable` with the reason and the rest of the run goes on.

## Package update scripts

`nixup check` and `nixup pkg` only look at packages that define `passthru.updateScript`. nixup reads that command line and uses some of its arguments:

- `--use-github-releases`: compare against published GitHub releases instead of every tag.
- `--version=unstable`: allow prereleases as the newest version. Any other `--version=...` argument is ignored, because nixup always passes the version you reviewed.
- Every other argument is passed on to `nix-update`.

For example:

```nix
{
  passthru.updateScript = nix-update-script {
    extraArgs = [ "--use-github-releases" ];
  };
}
```

## Required tools

| Tool | Used for | When missing |
|---|---|---|
| `git` | `ls-remote` for tags and heads, the scratch copy, the uncommitted-changes checks, restoring a package | Most checks fail |
| `nix` | `nix eval` of packages and hosts, `nix flake update`, `nix build`, `nix profile list` | The `profile` section is skipped; `packages`, `lock` and `pkg` fail |
| `nh` | `nh os build` in `nixup lock` | `lock` fails unless `--no-build` |
| `nix-update` | `nixup pkg` | nixup runs `nix run nixpkgs#nix-update --` instead |
| `flatpak` | The `flatpak` section | The section is skipped |
| `gh` | Only to read a token with `gh auth token` | API calls go out without a token |

nixup does not change `PATH`. When installed from the flake it uses the `nix`, `nh` and `git` already on your system.
