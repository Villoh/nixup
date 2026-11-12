---
title: Configuration
description: How nixup finds your flake, the environment variables it reads, the GitHub token and the tools it needs.
lang: en
---

# Configuration

**English** | [Español](es/configuration.md)

nixup has no configuration file. It takes a flag, a few environment variables, the `passthru.updateScript` and `passthru.nixup` attributes of your packages, and `# nixup: follows` comments in `flake.nix` (see [How it works](how-it-works.md)).

## Which flake

nixup works on one flake: a directory with both `flake.nix` and `flake.lock`. It picks the first of these that applies:

1. `-C <dir>`
2. The flake you are in: the current directory or the closest parent with both files, like git finds its repository.
3. `NIXUP_FLAKE`
4. `NH_OS_FLAKE`
5. `NH_FLAKE`

Inside a flake you work on that flake; anywhere else, on the one you configured. nixup prints nothing about which one it picked.

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

`-C <dir>` starts the upward search at `<dir>`, so `-C ~/nixos-config/hosts` finds `~/nixos-config`. It wins over the directory you are in and over every environment variable.

### Environment variables

The variables are read only outside a flake. A value from `NIXUP_FLAKE`, `NH_OS_FLAKE` or `NH_FLAKE` is used as is:

- A `#host` suffix is dropped: `~/nixos-config#desktop` means `~/nixos-config`.
- A `path:` prefix is dropped.
- A leading `~` is expanded to your home directory.
- Remote references such as `github:owner/repo` are rejected, because nixup has to write to the flake.
- The directory itself must contain `flake.nix` and `flake.lock`. nixup never falls back to a parent directory.

An invalid value is an error (exit code 2), even if a later variable would have worked:

```text
nixup: $NIXUP_FLAKE: "github:foo/bar" is not a local directory
nixup: $NH_FLAKE=/tmp: no flake.nix + flake.lock there
```

## Color

Output is colored only when stdout is a terminal. Any non-empty `NO_COLOR` turns color off, and so does `TERM=dumb`.

## Questions

`update` asks before it writes or installs anything. Questions are answered only when stdin is a terminal. `y` or `yes` is yes, and Enter, `n` or `no` is no, in any case; anything else is asked again. With no terminal every question is answered no, so unattended runs must pass `-y`.

## GitHub token

nixup reads tags and branch heads with `git ls-remote` over HTTPS, which needs no token. It uses the GitHub REST API for commit comparisons, release notes, and the release list of packages that follow GitHub releases. For those calls it takes the first token it finds:

1. `GH_TOKEN`
2. `GITHUB_TOKEN`
3. The output of `gh auth token`, if `gh` is installed.

Without a token the API calls are unauthenticated, and GitHub allows only 60 of those per hour. A `check --review` on a flake with many outdated entries can run out. When an API call fails, the review shows `review unavailable` with the reason and the rest of the run goes on.

## Package update scripts

When `passthru.updateScript` runs nix-update, `nixup check` and `nixup update` read its command line and use some of its arguments:

- `--url`: the upstream repository, for a source hosted elsewhere.
- `--use-github-releases`: compare against published GitHub releases instead of every tag.
- `--version-regex`: how versions are read from tags.
- `--version=unstable`: allow prereleases as the newest version.

`--version`, `--version-regex`, `-vr` and their values are not passed on to `nix-update`, because nixup passes the exact version you reviewed. Every other argument is passed on.

A package without an update script is listed as `info`, unless it sets `passthru.nixup.pinned` or `passthru.nixup.versionUrl`. A script that is not nix-update needs `passthru.nixup.versionUrl` (see [How it works](how-it-works.md#packages)).

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
| `git` | `ls-remote` for tags and heads, the scratch copy, the uncommitted-changes checks, the committed `flake.lock` | Most checks fail |
| `nix` | `nix eval` of packages and hosts, `nix flake update`, `nix build`, `nix profile list`, `nix profile upgrade` | The `profile` section is skipped; `packages` and `update` fail |
| `nh` | `nh os build` in the input step of `nixup update` with `--build`, `--test`, `--switch` or `--boot`; `nh os test`, `switch` or `boot` with `update --test`, `--switch` or `--boot` | The input step fails with those flags; without them `nh` is not used |
| `nix-update` | The package step of `nixup update` | nixup runs `nix run nixpkgs#nix-update --` instead |
| `flatpak` | The `flatpak` section and the Flatpak step of `nixup update` | Both are skipped |
| `gh` | Only to read a token with `gh auth token` | API calls go out without a token |

nixup does not change `PATH`. When installed from the flake it uses the `nix`, `nh` and `git` already on your system.
