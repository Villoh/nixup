---
title: Safety
description: What nixup guarantees when it updates your flake, and where those guarantees end.
lang: en
---

# Safety

**English** | [Español](es/safety.md)

nixup is built so that a failed or unwanted update leaves your repository as it was, and so that nothing reaches the running system without you. The input and package steps of `update` only edit files in your repository. The Flatpak and `nix profile` steps install on the machine right away, after a yes. This page lists what holds and what does not.

## What nixup guarantees

- **`check` writes nothing.** It reads `flake.lock`, evaluates your packages, and asks git, GitHub, `nix profile` and Flatpak.
- **The input step of `update` never edits your repository before the end.** The update and the builds run in a temporary copy. Your `flake.lock` is replaced only after every build passes and you answer yes, or you passed `-y`.
- **A failed host build leaves `flake.lock` untouched.** nixup prints `build of <host> FAILED: flake.lock left untouched.` and exits with 2.
- **Answering no to the new `flake.lock` changes nothing.** nixup prints `Discarded. Nothing changed.`
- **`update` refuses to touch a dirty `flake.lock`.** If `flake.lock` has uncommitted changes, the input step stops before doing anything and the exit code is 1. The package step still runs.
- **`update` moves a package to the version you reviewed.** It passes `--version=<that version>` to `nix-update`, so a release published while you were reading is not picked up.
- **A failed package update or build is rolled back.** nixup runs `git checkout -- <pkg dir>` and prints `nix-update failed; <pkg dir> restored.` or `build FAILED; <pkg dir> restored.`
- **`update` refuses a dirty package directory.** If `<pkg dir>` has uncommitted changes, that package is skipped with exit code 1, because the rollback would throw those changes away.
- **The Flatpak and `nix profile` steps ask before they install.** The question says it: `Update 2 Flatpak refs now? This installs them right away. [y/N]`. A no installs nothing.
- **The Flatpak step updates only what it showed you.** It passes exactly the refs in its table to `flatpak update`. Before that it notes each ref's deployed commit, and afterwards prints `flatpak update --commit=<previous commit> <ref>` for each one, also after a failure.
- **The `nix profile` step says how to undo it.** After an upgrade it prints `To undo it: nix profile rollback`.
- **Pinned `nix profile` entries are never touched.** An entry pinned to a revision is left out of `nix profile upgrade`. nixup only says how to update it by hand.
- **No terminal means no.** When stdin is not a terminal, every question is answered no. Unattended runs must pass `-y` on purpose.
- **No generation is activated, nothing is committed.** No `git commit`, no `switch`, no `boot`, no `test`, no `sudo`. After an update you still run `git diff`, `nix flake check` and `nh os test` yourself.

## Where the guarantees end

- **A build that passes is not a system that boots.** `nh os build` proves the configuration evaluates and builds. It does not prove the new generation works. Test it with `nh os test` before you switch.
- **`--no-build` removes the build check.** With `update --no-build -y` the new lock is written without building any host, and a broken package is only noticed later.
- **`-y` skips your review.** The review is still printed and, with `--report`, saved. Nobody reads it before the write.
- **The package step edits the real checkout.** Unlike the input step, `nix-update` runs in your repository. The rollback only covers tracked files in the directory of the file that defines the package (from `meta.position`). Files `nix-update` might change elsewhere are neither checked nor restored. New untracked files are not removed.
- **A failed rollback is reported, not retried.** If `git checkout` fails, nixup prints `nixup: restore failed:` with the reason and you have to clean up by hand.
- **The scratch copy is your working tree, not your last commit.** Uncommitted and untracked (but not ignored) files go into the host builds. Symlinks are not copied.
- **The review depends on GitHub.** If the API is unreachable or rate-limited, you see `review unavailable` and can still accept the update.
- **The breaking-change warning is a guess from version numbers.** A patch release can break things. Date-based versions (`20261106.1`, `2026.10.14`) are recognised and get no warning, since their numbers carry no compatibility meaning.
- **`--report` is written before the `flake.lock` question.** The report describes the proposed change even if you then answer no.
- **The Flatpak and `nix profile` steps change the machine at once.** There is no scratch copy, no build check and no diff to review: what you accept is installed when the step ends. With `-y` that happens without a question.
- **Going back depends on Flatpak and Nix.** `flatpak update --commit` needs the previous commit to still be available from the remote, and a ref whose commit could not be read gets no line. `nix profile rollback` returns to the previous profile generation, which is the one before the upgrade only if nothing changed the profile since.
- **Flatpak updates have no upstream review.** The Flatpak step shows versions only, not commits or release notes.
