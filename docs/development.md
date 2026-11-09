---
title: Development
description: Code layout, building, running the tests, and how nixup is tested without touching the network or your system.
lang: en
---

# Development

**English** | [Español](es/development.md)

## Layout

```text
cmd/nixup/          main: wires the real terminal, process runner and GitHub client
internal/
  cli/              flag parsing, subcommands, exit codes
  check/            the read-only checks: inputs, packages, profile, flatpak
  update/           the command that writes: Update runs Lock (flake.lock), then Pkg (packages/)
  report/           terminal text and Markdown rendering
  flake/            flake.lock, finding the flake, hosts, own packages, scratch copy
  gh/               tags and heads through git; commits and release notes through the API
  ver/              version ordering, prerelease detection, major-jump warnings
  run/              the only place that starts processes (Runner), plus a scripted Fake
  ui/               terminal output, color, yes/no questions
  par/              bounded concurrency
  exit/             exit codes shared by every command
  testutil/         fake GitHub API, temporary trees and terminals (tests only)
```

Dependencies point one way: `cli → update → report → check → flake → gh → run, ver`. Two rules keep the code testable:

- Only `internal/run` starts processes. Everything else receives a `run.Runner`.
- Only `internal/ui` talks to the terminal. `main` passes stderr to the runner so builds stream there.

The code uses the Go standard library only. `go.mod` has no dependencies.

## Build and run

```bash
nix develop                          # go, gopls, nix-update
go build -o nixup ./cmd/nixup
./nixup help
```

A plain `go build` reports its version as `nixup dev`. The flake sets the real version and commit with `-ldflags -X` on `internal/cli.Version` and `internal/cli.Commit`:

```bash
nix build
./result/bin/nixup version
```

## Tests

```bash
go test -race ./...
nix build            # also runs the tests, inside the Nix sandbox
```

CI (`.github/workflows/ci.yml`) runs on every push to `main` and on pull requests: `gofmt`, `go vet`, `go test -race -count=1`, `nix build -L` and `nix flake check`. Run `gofmt -l .` before you push; CI fails if it prints anything.

## How the tests work

The tests never touch the network, your repositories or your system.

**`run.Fake`** replaces the process runner. Each `run.Rule` answers the first command whose command line starts with its `Prefix`, with stdout, stderr and an optional error. A rule's `Do` function runs before the answer, to simulate side effects such as `nix-update` editing a file. Commands without a rule succeed with no output. `Missing` makes `Has` report a tool as absent, and `Calls` and `Count` let a test assert what ran, or that something never ran:

```go
f := (&run.Fake{}).
	On(run.Rule{Prefix: "nix flake update", Fail: errors.New("exit 1"), Err: "error: boom\n"})
// ... run the code under test with f as its Runner ...
if f.Count("nh os build") != 0 {
	t.Error("must not build after a failed update")
}
```

**`testutil.New`** builds a fake world on top of that: a `run.Fake` and an `httptest` server that plays the GitHub API. It clears `GH_TOKEN` and `GITHUB_TOKEN` and marks `gh` as missing, so no real token is read. Helpers script the rest:

- `Tags` and `Head` answer `git ls-remote` for a repository.
- `Compare` and `Releases` answer the compare and releases API endpoints. `API` answers any other path.
- `Tree` writes a temporary directory from a map of files.
- `Term` returns a terminal that captures stdout and stderr, and answers questions from a string when one is given.
- `LockJSON` builds a minimal `flake.lock`.

A few tests run real processes on purpose: the `run` package tests start `sh`, and one `flake` test creates a temporary git repository to check the scratch copy, the uncommitted-changes check and the restore against real git. It is skipped when `git` is not installed.

## Commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/). The release notes are generated from them, so the type decides the section a change appears in. See [Releases](releases.md).
