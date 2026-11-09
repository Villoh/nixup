---
title: Safety
description: What nixup guarantees when it updates your flake, and where those guarantees end.
lang: en
---

# Safety

**English** | [Español](es/safety.md)

nixup is built so that a failed or unwanted update leaves your repository as it was, and so that nothing reaches the running system without you. This page lists what holds and what does not.

## What nixup guarantees

- **`check` writes nothing.** It reads `flake.lock`, evaluates your packages, and asks git, GitHub, `nix profile` and Flatpak.
- **`lock` never edits your repository before the end.** The update and the builds run in a temporary copy. Your `flake.lock` is replaced only after every build passes and you answer yes, or you passed `-y`.
- **A failed `lock` build leaves `flake.lock` untouched.** nixup prints `build of <host> FAILED: flake.lock left untouched.` and exits with 2.
- **Answering no to `lock` changes nothing.** nixup prints `Discarded. Nothing changed.`
- **`lock` refuses to run on a dirty `flake.lock`.** If `flake.lock` has uncommitted changes, it stops with exit code 1 before doing anything.
- **`pkg` updates to the version you reviewed.** It passes `--version=<that version>` to `nix-update`, so a release published while you were reading is not picked up.
- **A failed `pkg` update or build is rolled back.** nixup runs `git checkout -- <pkg dir>` and prints `nix-update failed; <pkg dir> restored.` or `build FAILED; <pkg dir> restored.`
- **`pkg` refuses a dirty package directory.** If `<pkg dir>` has uncommitted changes, that package is skipped with exit code 1, because the rollback would throw those changes away.
- **No terminal means no.** When stdin is not a terminal, every question is answered no. Unattended runs must pass `-y` on purpose.
- **Nothing is committed or activated.** No `git commit`, no `switch`, no `boot`, no `test`, no `sudo`. After an update you still run `git diff`, `nix flake check` and `nh os test` yourself.

## Where the guarantees end

- **A build that passes is not a system that boots.** `nh os build` proves the configuration evaluates and builds. It does not prove the new generation works. Test it with `nh os test` before you switch.
- **`--no-build` removes the build check.** With `lock --no-build -y` the new lock is written without building anything. With `pkg --no-build` a broken package is only noticed later.
- **`-y` skips your review.** The review is still printed and, with `--report`, saved. Nobody reads it before the write.
- **`pkg` edits the real checkout.** Unlike `lock`, `nix-update` runs in your repository. The rollback only covers tracked files in the directory of the file that defines the package (from `meta.position`). Files `nix-update` might change elsewhere are neither checked nor restored. New untracked files are not removed.
- **A failed rollback is reported, not retried.** If `git checkout` fails, nixup prints `nixup: restore failed:` with the reason and you have to clean up by hand.
- **The scratch copy is your working tree, not your last commit.** Uncommitted and untracked (but not ignored) files go into the `lock` build. Symlinks are not copied.
- **The review depends on GitHub.** If the API is unreachable or rate-limited, you see `review unavailable` and can still accept the update.
- **The breaking-change warning is a guess from version numbers.** A patch release can break things. Date-based versions (`20261106.1`, `2026.10.14`) are recognised and get no warning, since their numbers carry no compatibility meaning.
- **`--report` is written before the `lock` question.** The report describes the proposed change even if you then answer no.
