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
nixup pkg strata -y    # no questions (CI)
```

Flags work before, between and after names. `-C <dir>` selects the flake (default: search
upwards from the current directory). `check` exits 0 (all fine), 1 (something outdated), 2 (error).

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

## Install and develop

```nix
# flake input; then add inputs.nixup.packages.${system}.default to systemPackages
nixup.url = "path:/home/mikel/development/personal/projects/nixup";
```

```bash
nix build .      # package
nix develop      # go, gopls, nix-update
go test ./...
```

Needs `git`, `nix`, `nh` (for `lock` builds) and `nix-update` (or it runs it through
`nix run nixpkgs#nix-update`). `flatpak` is optional. GitHub API calls use `GH_TOKEN`,
`GITHUB_TOKEN` or `gh auth token`; without one the rate limit is low.

## Not included

- `dms-bitwarden` (pinned on purpose, lives in a module, not in `packages/`).
- Non-GitHub upstreams: reported as `info`.
- Bumping hashes by itself: that is `nix-update`'s job.
