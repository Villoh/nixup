# nixup

Update checker and safe updater for a flake-based NixOS system. Go, standard library only.
It never activates anything: no `switch`, no `boot`, no `sudo`, no daemon or timer. You run
it, you read what changes upstream, you decide.

```bash
nixup                  # = nixup check: what is outdated (read-only)
nixup check --review   # + upstream commits and release notes for each outdated entry
nixup lock             # update flake.lock in a scratch copy, show changes, build, ask, apply
nixup lock nixpkgs home-manager        # only those inputs
nixup pkg              # update own packages/ one by one, after reviewing each
nixup pkg mypkg -y     # no questions (CI)
```

Flags work before, between and after names.

## Which flake

You do not have to be inside it. In order of precedence:

1. `-C <dir>`
2. `$NIXUP_FLAKE`
3. `$NH_OS_FLAKE`, then `$NH_FLAKE`: the variables [nh](https://github.com/nix-community/nh)
   reads, so a configured `nh` needs no extra setup (a `#host` suffix is ignored)
4. otherwise, search upwards from the current directory

A configured path must be a local directory with `flake.nix` and `flake.lock`; it is never
replaced by a parent or by the current directory. When the flake comes from an environment
variable, nixup says so on stderr (`nixup: using /path ($NH_FLAKE)`), so `--json` stays clean.
There is no config file: on NixOS `programs.nh.flake` sets `NH_FLAKE` declaratively. `check` exits 0 (all fine), 1 (something outdated), 2 (error).

## What it covers

| Section | How | Moves with |
|---|---|---|
| `inputs` | tag-pinned (`.../v1.6.2`) vs newest tag; branch inputs vs remote HEAD | `nixup lock`, or edit `flake.nix` for pinned tags |
| `packages` | every package in the flake's `packages.<system>` output with a GitHub upstream | `nixup pkg` (runs `nix-update`) |
| `profile` | `nix profile` entries pinned to a different nixpkgs than the system | `nix profile upgrade` |
| `flatpak` | `flatpak remote-ls --updates` | `flatpak update` |

## Review before updating

For every pending update nixup shows what Arch's PKGBUILD review would: the source repo,
the commit count with a compare link, the latest commit subjects and the release notes in
between (prereleases are skipped and counted). Jumps that are probably breaking (`1.x → 2.x`,
`0.21 → 1.0`, `0.x` minor) get a warning. Nothing is written until you answer yes.

## Safety

- `lock` works on a copy of the working tree. If a build fails or you answer no, your repo
  is untouched. It refuses to start if `flake.lock` has uncommitted changes.
- `pkg` runs `nix-update --version=<the version you just reviewed>`, builds the package and
  prints the diff. If `nix-update` or the build fails, `packages/<name>` is restored.
  It refuses to start if that directory has uncommitted changes.
- Without a terminal every question is answered "no": unattended runs must pass `-y`.
- `nix-update --flake` ignores `passthru.updateScript`'s `extraArgs`, so nixup reads them
  (for example `--version=unstable`, which allows prereleases) and passes them on.

## CI (weekly job)

`--report FILE` appends Markdown (changes, notes, build result) for a PR body, and
`check --markdown` prints the same for the pending list. A workflow would run
`nixup lock -y --no-build --report r.md` and `nixup pkg -y --report r.md`, then open a PR with
`r.md`; the full-system build stays local. Not wired yet.

## Install

```bash
nix run github:Villoh/nixup -- check       # try it without installing
```

```nix
# flake input; then add inputs.nixup.packages.${system}.default to systemPackages
nixup = {
  url = "github:Villoh/nixup";
  inputs.nixpkgs.follows = "nixpkgs";
};
```

## Layout

```text
cmd/nixup/          main: wires the real terminal, process runner and GitHub client
internal/
  cli/              flag parsing, subcommands, exit codes
  check/            the read-only checks: inputs, packages, profile, flatpak
  update/           the two commands that write: Lock (flake.lock) and Pkg (packages/)
  report/           terminal text and Markdown rendering
  flake/            flake.lock, repo discovery, hosts, own packages, scratch copy
  gh/               tags and heads through git; commits and release notes through the API
  ver/              version ordering, prerelease detection, major-jump warnings
  run/              the only place that starts processes (Runner), plus a scripted Fake
  ui/               terminal output, color, yes/no questions
  par/              bounded concurrency
  exit/             exit codes shared by every command
  testutil/         fake GitHub API, temp trees and terminals (tests only)
```

Dependencies point one way: `cli → update → check → gh/flake → run/ver`. Nothing outside
`run` starts a process and nothing outside `ui` talks to the terminal, so every command
can be tested against a scripted runner.

## Develop

```bash
nix develop        # go, gopls, nix-update
go test -race ./...
nix build          # also runs the tests inside the Nix sandbox
```

The tests never touch the network, your repos or your system: `run.Fake` answers every
`git`, `nix`, `nh`, `nix-update` and `flatpak` call from a script, and `testutil` serves a
fake GitHub API. Two tests use a real temporary `git` repository. CI runs gofmt, vet, the
tests with `-race`, and `nix build`.

Needs `git`, `nix`, `nh` (for `lock` builds) and `nix-update` (or it runs it through
`nix run nixpkgs#nix-update`). `flatpak` is optional. GitHub API calls use `GH_TOKEN`,
`GITHUB_TOKEN` or `gh auth token`; without one the rate limit is low.

## Releases

Do not edit the version or tag by hand. Run the workflow with the new version:

```bash
gh workflow run release -f version=0.2.0                 # or the Actions tab
gh workflow run release -f version=0.2.0 -f dry_run=true # validate only, publish nothing
```

It checks the version is `X.Y.Z` and higher than the current one, bumps `flake.nix`, runs
vet and the tests, commits `chore(release): vX.Y.Z` to `main` through the API (so GitHub signs
it) and publishes the `vX.Y.Z` release with notes built from the Conventional Commits since the last tag (`cliff.toml`).

## Not included

- Anything defined outside the flake's `packages` output (a `fetchurl` inside a module, a pinned
  `rev`): nixup does not see it. Move it to `packages/` to have it checked.
- Non-GitHub upstreams: reported as `info`.
- Bumping hashes by itself: that is `nix-update`'s job.
