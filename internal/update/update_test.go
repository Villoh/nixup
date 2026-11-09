package update_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/testutil"
	"github.com/Villoh/nixup/internal/update"
)

type fixture struct {
	e         *testutil.Env
	repo      string
	u         *update.Updater
	out, errb *bytes.Buffer
}

func newFixture(t *testing.T, answers string, files map[string]string) *fixture {
	t.Helper()
	e := testutil.New(t)
	term, out, errb := testutil.Term(answers)
	return &fixture{
		e: e, repo: testutil.Tree(t, files), out: out, errb: errb,
		u: &update.Updater{R: e.Run, GH: e.GH, Check: &check.Checker{R: e.Run, GH: e.GH}, T: term},
	}
}

func (f *fixture) called(prefix string) bool { return f.e.Run.Count(prefix) > 0 }

// ---- Pkg ------------------------------------------------------------------------------

const padpollJSON = `{"padpoll":{"version":"1.0.0","urls":["https://github.com/PabloStarOk/padpoll/releases/download/v1.0.0/x"],"homepage":"","update":["nix-update"]}}`

// pkgFixture scripts a padpoll 1.0.0 whose upstream is at 1.1.0.
func pkgFixture(t *testing.T, answers string, extra ...run.Rule) *fixture {
	f := newFixture(t, answers, map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	for _, r := range extra {
		f.e.Run.On(r)
	}
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "git diff --", Out: "-version = 1.0.0\n+version = 1.1.0\n"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: padpollJSON})
	f.e.Tags("PabloStarOk/padpoll", "v1.0.0", "v1.1.0")
	f.e.Compare("PabloStarOk/padpoll", "v1.0.0", "v1.1.0", 5, "build: bump")
	f.e.Releases("PabloStarOk/padpoll", testutil.Release{Tag: "v1.1.0", Body: "release notes"})
	return f
}

func TestPkgUpdatesAfterReview(t *testing.T) {
	f := pkgFixture(t, "y\n")
	code := f.u.Pkg(update.PkgOpts{Repo: f.repo})
	if code != exit.OK {
		t.Fatalf("code %d\n%s%s", code, f.out, f.errb)
	}
	for _, want := range []string{"nix-update --flake padpoll --version=1.1.0", "nix build --no-link .#padpoll"} {
		if !f.called(want) {
			t.Errorf("did not run %q: %v", want, f.e.Run.Calls())
		}
	}
	s := f.out.String()
	for _, want := range []string{"padpoll  1.0.0 -> 1.1.0", "source   https://github.com/PabloStarOk/padpoll", "release notes", "Update padpoll to 1.1.0? [y/N]", "+version = 1.1.0", "padpoll updated (not committed)"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if f.called("git checkout") {
		t.Error("nothing to restore on success")
	}
}

func TestPkgNeverActivatesAnything(t *testing.T) {
	f := pkgFixture(t, "y\n")
	f.u.Pkg(update.PkgOpts{Repo: f.repo})
	for _, c := range f.e.Run.Calls() {
		s := c.String()
		if strings.Contains(s, "switch") || strings.Contains(s, "nixos-rebuild") || strings.HasPrefix(s, "sudo") || strings.Contains(s, "nh os test") {
			t.Errorf("nixup must not activate: %s", s)
		}
	}
}

func TestPkgDeclinedOrUnattendedChangesNothing(t *testing.T) {
	for name, answers := range map[string]string{"answered no": "n\n", "no terminal": ""} {
		f := pkgFixture(t, answers)
		if code := f.u.Pkg(update.PkgOpts{Repo: f.repo}); code != exit.OK {
			t.Errorf("%s: code %d", name, code)
		}
		if f.called("nix-update") || f.called("nix run") || f.called("nix build") {
			t.Errorf("%s: touched the repo: %v", name, f.e.Run.Calls())
		}
		if !strings.Contains(f.out.String(), "Skipped.") {
			t.Errorf("%s: %s", name, f.out)
		}
	}
}

func TestPkgYesSkipsTheQuestion(t *testing.T) {
	f := pkgFixture(t, "")
	if code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true}); code != exit.OK || !f.called("nix-update") {
		t.Errorf("code %d, calls %v", code, f.e.Run.Calls())
	}
	if strings.Contains(f.out.String(), "[y/N]") {
		t.Error("-y must not prompt")
	}
}

func TestPkgNoBuild(t *testing.T) {
	f := pkgFixture(t, "")
	f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, NoBuild: true})
	if !f.called("nix-update") || f.called("nix build") {
		t.Errorf("calls: %v", f.e.Run.Calls())
	}
}

func TestPkgNixUpdateFailureRestores(t *testing.T) {
	f := pkgFixture(t, "", run.Rule{Prefix: "nix-update", Fail: errors.New("exit 1")})
	report := filepath.Join(t.TempDir(), "r.md")
	code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, Report: report})
	if code != exit.Error {
		t.Errorf("code %d", code)
	}
	if !f.called("git checkout -- packages/padpoll") {
		t.Errorf("packages/padpoll must be restored: %v", f.e.Run.Calls())
	}
	if f.called("nix build") {
		t.Error("no build after a failed update")
	}
	if b, _ := os.ReadFile(report); !strings.Contains(string(b), "nix-update failed") {
		t.Errorf("report: %q", b)
	}
}

func TestPkgBuildFailureRestores(t *testing.T) {
	f := pkgFixture(t, "", run.Rule{Prefix: "nix build", Fail: errors.New("exit 1")})
	report := filepath.Join(t.TempDir(), "r.md")
	code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, Report: report})
	if code != exit.Error || !f.called("git checkout -- packages/padpoll") {
		t.Errorf("code %d, calls %v", code, f.e.Run.Calls())
	}
	if !strings.Contains(f.out.String(), "build FAILED") || strings.Contains(f.out.String(), "updated (not committed)") {
		t.Errorf("%s", f.out)
	}
	b, _ := os.ReadFile(report)
	if !strings.Contains(string(b), "Build failed") || !strings.Contains(string(b), "### padpoll `1.0.0` → `1.1.0`") {
		t.Errorf("report: %q", b)
	}
}

func TestPkgRefusesDirtyPackageDir(t *testing.T) {
	f := pkgFixture(t, "", run.Rule{Prefix: "git diff --quiet -- packages/padpoll", Fail: errors.New("exit 1")})
	if code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true}); code != exit.Pending {
		t.Errorf("code %d", code)
	}
	if f.called("nix-update") || f.called("git checkout") {
		t.Errorf("uncommitted work must never be touched or restored: %v", f.e.Run.Calls())
	}
	if !strings.Contains(f.out.String(), "uncommitted changes") {
		t.Errorf("%s", f.out)
	}
}

func TestPkgPassesPackageArgsAndNeverTheirVersion(t *testing.T) {
	f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"flectar":{"version":"0.1.0-alpha.6","urls":[],"homepage":"https://github.com/flectar/mail","update":["nix-update","--version=unstable","--use-github-releases"]}}`})
	f.e.Tags("flectar/mail", "v0.1.0-alpha.6", "v0.1.0-alpha.7")
	// --use-github-releases makes nixup read releases (as nix-update will), not tags.
	f.e.Releases("flectar/mail", testutil.Release{Tag: "v0.1.0-alpha.6"}, testutil.Release{Tag: "v0.1.0-alpha.7"})
	if code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, NoBuild: true}); code != exit.OK {
		t.Fatalf("code %d\n%s", code, f.out)
	}
	for _, c := range f.e.Run.Calls() {
		if c.Name == "nix-update" {
			if got := c.String(); got != "nix-update --flake flectar --version=0.1.0-alpha.7 --use-github-releases" {
				t.Errorf("command = %s", got)
			}
			return
		}
	}
	t.Error("nix-update did not run")
}

func TestPkgFallsBackToNixRunWithoutNixUpdate(t *testing.T) {
	f := pkgFixture(t, "")
	f.e.Run.Missing["nix-update"] = true
	f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, NoBuild: true})
	if !f.called("nix run nixpkgs#nix-update -- --flake padpoll --version=1.1.0") {
		t.Errorf("calls: %v", f.e.Run.Calls())
	}
}

func TestPkgOnlyTheRequestedPackage(t *testing.T) {
	f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{
	 "a":{"version":"1.0.0","urls":["https://github.com/o/a/x"],"homepage":"","update":["nix-update"]},
	 "b":{"version":"1.0.0","urls":["https://github.com/o/b/x"],"homepage":"","update":["nix-update"]}}`})
	f.e.Tags("o/a", "v1.0.0", "v1.0.1")
	f.e.Tags("o/b", "v1.0.0", "v1.0.1")
	f.u.Pkg(update.PkgOpts{Repo: f.repo, Names: []string{"b"}, Yes: true, NoBuild: true})
	if f.called("nix-update --flake a") || !f.called("nix-update --flake b") {
		t.Errorf("calls: %v", f.e.Run.Calls())
	}
}

func TestPkgNothingToDo(t *testing.T) {
	f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: padpollJSON})
	f.e.Tags("PabloStarOk/padpoll", "v1.0.0")
	if code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true}); code != exit.OK {
		t.Errorf("code %d", code)
	}
	if f.called("nix-update") || f.out.Len() != 0 {
		t.Errorf("everything is current, so be quiet: %q %v", f.out, f.e.Run.Calls())
	}
	// Asking for a package by name reports its state instead of staying silent.
	f.u.Pkg(update.PkgOpts{Repo: f.repo, Names: []string{"padpoll"}, Yes: true})
	if !strings.Contains(f.out.String(), "OK padpoll 1.0.0") {
		t.Errorf("%q", f.out)
	}
}

func TestPkgErrors(t *testing.T) {
	f := pkgFixture(t, "")
	if code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Names: []string{"nope"}}); code != exit.Error {
		t.Errorf("unknown package: code %d", code)
	}
	if !strings.Contains(f.errb.String(), "no such package: nope") {
		t.Errorf("%q", f.errb)
	}

	g := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	g.e.Run.On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"), Err: "error: boom\n"})
	if code := g.u.Pkg(update.PkgOpts{Repo: g.repo}); code != exit.Error {
		t.Errorf("eval failure: code %d", code)
	}

	h := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	h.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: padpollJSON})
	h.e.Run.On(run.Rule{Prefix: "git ls-remote", Fail: errors.New("x"), Err: "fatal: no network\n"})
	if code := h.u.Pkg(update.PkgOpts{Repo: h.repo, Yes: true}); code != exit.Error || h.called("nix-update") {
		t.Errorf("a package whose upstream cannot be read is an error and is not touched: code %d", code)
	}
}

func TestPkgWorstCodeWins(t *testing.T) {
	f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix-update --flake bad", Fail: errors.New("exit 1")})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{
	 "bad":{"version":"1.0.0","urls":["https://github.com/o/bad/x"],"homepage":"","update":["nix-update"]},
	 "good":{"version":"1.0.0","urls":["https://github.com/o/good/x"],"homepage":"","update":["nix-update"]}}`})
	f.e.Tags("o/bad", "v1.0.0", "v1.0.1")
	f.e.Tags("o/good", "v1.0.0", "v1.0.1")
	code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, NoBuild: true})
	if code != exit.Error || !f.called("nix-update --flake good") {
		t.Errorf("one failure must not stop the others, and must show in the exit code: code %d", code)
	}
}

// ---- Lock -----------------------------------------------------------------------------

const (
	oldRev, newRev = "aaa1111", "bbb2222"
)

func lockJSON(rev string) string {
	return testutil.LockJSON(map[string]string{"nixpkgs": "NixOS/nixpkgs@#" + rev})
}

// lockFixture scripts a repo whose nixpkgs moves from oldRev to newRev on `nix flake update`.
func lockFixture(t *testing.T, answers string, extra ...run.Rule) *fixture {
	f := newFixture(t, answers, map[string]string{"flake.nix": "{}", "flake.lock": lockJSON(oldRev)})
	for _, r := range extra {
		f.e.Run.On(r)
	}
	f.e.Run.On(run.Rule{Prefix: "git ls-files -z", Out: "flake.nix\x00flake.lock\x00"})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix flake update", Do: func(c run.Call) {
		os.WriteFile(filepath.Join(c.Args[3], "flake.lock"), []byte(lockJSON(newRev)), 0o644)
	}})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json", Out: `["desktop","zenbook"]`})
	f.e.Compare("NixOS/nixpkgs", oldRev, newRev, 7, "fix: something")
	return f
}

func (f *fixture) lock() string {
	b, _ := os.ReadFile(filepath.Join(f.repo, "flake.lock"))
	return string(b)
}

func TestLockAppliesAfterBuildAndConfirmation(t *testing.T) {
	f := lockFixture(t, "y\n")
	if code := f.u.Lock(update.LockOpts{Repo: f.repo}); code != exit.OK {
		t.Fatalf("code %d\n%s%s", code, f.out, f.errb)
	}
	if f.lock() != lockJSON(newRev) {
		t.Error("the new flake.lock must be written")
	}
	s := f.out.String()
	for _, want := range []string{"nixpkgs  aaa1111 -> bbb2222", "7 commits", "· fix: something", "Write this flake.lock to the repo? [y/N]", "flake.lock updated (not committed, nothing activated)"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	// Every host is built, in the scratch copy, never in the repo.
	for _, h := range []string{"desktop", "zenbook"} {
		found := false
		for _, c := range f.e.Run.Calls() {
			if c.Name == "nh" && strings.Contains(c.String(), "-H "+h) {
				found = true
				if strings.Contains(c.String(), f.repo) {
					t.Errorf("build must run on the scratch copy, not the repo: %s", c)
				}
			}
		}
		if !found {
			t.Errorf("host %s was not built", h)
		}
	}
	for _, c := range f.e.Run.Calls() {
		if strings.Contains(c.String(), "nh os switch") || strings.Contains(c.String(), "nh os test") || strings.Contains(c.String(), "nh os boot") {
			t.Errorf("nixup must not activate: %s", c)
		}
	}
}

func TestLockBuildFailureLeavesRepoUntouched(t *testing.T) {
	f := lockFixture(t, "", run.Rule{Prefix: "nh os build", Fail: errors.New("exit 1")})
	report := filepath.Join(t.TempDir(), "r.md")
	if code := f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true, Report: report}); code != exit.Error {
		t.Errorf("code %d", code)
	}
	if f.lock() != lockJSON(oldRev) {
		t.Error("a failed build must not change flake.lock, even with -y")
	}
	if !strings.Contains(f.out.String(), "FAILED") {
		t.Errorf("%s", f.out)
	}
	b, _ := os.ReadFile(report)
	if !strings.Contains(string(b), "Build of `desktop` failed") {
		t.Errorf("report: %q", b)
	}
	if f.e.Run.Count("nh os build") != 1 {
		t.Error("stop at the first failing host")
	}
}

func TestLockDeclinedOrUnattendedChangesNothing(t *testing.T) {
	for name, answers := range map[string]string{"answered no": "n\n", "no terminal": ""} {
		f := lockFixture(t, answers)
		if code := f.u.Lock(update.LockOpts{Repo: f.repo}); code != exit.OK {
			t.Errorf("%s: code %d", name, code)
		}
		if f.lock() != lockJSON(oldRev) || !strings.Contains(f.out.String(), "Discarded. Nothing changed.") {
			t.Errorf("%s: lock=%s out=%s", name, f.lock(), f.out)
		}
	}
}

func TestLockYesWritesWithoutAsking(t *testing.T) {
	f := lockFixture(t, "")
	if code := f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true, Hosts: []string{"desktop"}}); code != exit.OK {
		t.Fatalf("code %d", code)
	}
	if f.lock() != lockJSON(newRev) || strings.Contains(f.out.String(), "[y/N]") {
		t.Error("-y writes the lock without prompting")
	}
	if f.e.Run.Count("nh os build") != 1 || f.e.Run.Count("nix eval --json") != 0 {
		t.Errorf("explicit -H builds only those hosts and does not list them: %v", f.e.Run.Calls())
	}
}

func TestLockNoBuild(t *testing.T) {
	f := lockFixture(t, "")
	f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true, NoBuild: true})
	if f.called("nh ") || f.called("nix eval") {
		t.Errorf("--no-build must not build or list hosts: %v", f.e.Run.Calls())
	}
	if f.lock() != lockJSON(newRev) {
		t.Error("lock not written")
	}
}

func TestLockUpdatesOnlyTheNamedInputsInAScratchCopy(t *testing.T) {
	f := lockFixture(t, "")
	var scratch string
	f.e.Run.Rules = append([]run.Rule{{Prefix: "nix flake update", Do: func(c run.Call) {
		scratch = c.Args[3]
		os.WriteFile(filepath.Join(scratch, "flake.lock"), []byte(lockJSON(newRev)), 0o644)
	}}}, f.e.Run.Rules...)
	f.u.Lock(update.LockOpts{Repo: f.repo, Inputs: []string{"nixpkgs", "home-manager"}, Yes: true, NoBuild: true})

	var upd run.Call
	for _, c := range f.e.Run.Calls() {
		if strings.HasPrefix(c.String(), "nix flake update") {
			upd = c
		}
	}
	args := strings.Join(upd.Args, " ")
	if !strings.HasSuffix(args, "nixpkgs home-manager") || scratch == "" || scratch == f.repo {
		t.Errorf("args=%q scratch=%q repo=%q", args, scratch, f.repo)
	}
	if _, err := os.Stat(scratch); err == nil {
		t.Error("the scratch copy must be removed afterwards")
	}
}

func TestLockNothingChanged(t *testing.T) {
	f := lockFixture(t, "")
	f.e.Run.Rules = append([]run.Rule{{Prefix: "nix flake update"}}, f.e.Run.Rules...) // leaves the copy as is
	if code := f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true}); code != exit.OK {
		t.Errorf("code %d", code)
	}
	if !strings.Contains(f.out.String(), "already up to date") || f.called("nh ") {
		t.Errorf("%s %v", f.out, f.e.Run.Calls())
	}
}

func TestLockRefusesDirtyLock(t *testing.T) {
	f := lockFixture(t, "", run.Rule{Prefix: "git diff --quiet -- flake.lock", Fail: errors.New("exit 1")})
	if code := f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true}); code != exit.Pending {
		t.Errorf("code %d", code)
	}
	if f.called("nix flake update") || f.lock() != lockJSON(oldRev) {
		t.Error("a dirty lock must be left alone, without even running the update")
	}
}

func TestLockUpdateFailure(t *testing.T) {
	f := lockFixture(t, "", run.Rule{Prefix: "nix flake update", Fail: errors.New("exit 1")})
	if code := f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true}); code != exit.Error || f.lock() != lockJSON(oldRev) {
		t.Errorf("code %d", code)
	}
}

func TestLockWritesReport(t *testing.T) {
	f := lockFixture(t, "")
	report := filepath.Join(t.TempDir(), "r.md")
	f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true, Report: report})
	b, _ := os.ReadFile(report)
	for _, want := range []string{"### flake.lock", "#### nixpkgs `aaa1111` → `bbb2222`", "7 commits", "Build of `desktop`: ok.", "Build of `zenbook`: ok."} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in:\n%s", want, b)
		}
	}
}

func TestLockCopiesUncommittedWorkIntoTheBuild(t *testing.T) {
	f := lockFixture(t, "")
	var sawFile string
	f.e.Run.Rules = append([]run.Rule{{Prefix: "nh os build", Do: func(c run.Call) {
		b, _ := os.ReadFile(filepath.Join(c.Args[2], "flake.nix"))
		sawFile = string(b)
	}}}, f.e.Run.Rules...)
	os.WriteFile(filepath.Join(f.repo, "flake.nix"), []byte("{ uncommitted = true; }"), 0o644)
	f.u.Lock(update.LockOpts{Repo: f.repo, Yes: true, Hosts: []string{"desktop"}})
	if sawFile != "{ uncommitted = true; }" {
		t.Errorf("the build must see the working tree, got %q", sawFile)
	}
}
