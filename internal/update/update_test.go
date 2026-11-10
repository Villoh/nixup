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
	for _, name := range []string{"padpoll", "flectar", "a", "b", "bad", "good"} {
		files["packages/"+name+"/package.nix"] = "{}" // where meta.position points
	}
	e := testutil.New(t)
	term, out, errb := testutil.Term(answers)
	return &fixture{
		e: e, repo: testutil.Tree(t, files), out: out, errb: errb,
		u: &update.Updater{R: e.Run, GH: e.GH, Check: &check.Checker{R: e.Run, GH: e.GH}, T: term},
	}
}

func (f *fixture) called(prefix string) bool { return f.e.Run.Count(prefix) > 0 }

// ---- Pkg ------------------------------------------------------------------------------

const padpollJSON = `{"padpoll":{"position":"/nix/store/x-source/packages/padpoll/package.nix:1","version":"1.0.0","urls":["https://github.com/PabloStarOk/padpoll/releases/download/v1.0.0/x"],"homepage":"","update":["nix-update"]}}`

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
	for _, want := range []string{"padpoll  1.0.0 -> 1.1.0", "source   https://github.com/PabloStarOk/padpoll", "release notes", "Update padpoll to 1.1.0? [y/N]", "+version = 1.1.0", "padpoll updated."} {
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

// edits makes nix-update write into the package before the scripted answer.
func edits(f **fixture) func(run.Call) {
	return func(run.Call) {
		os.WriteFile(filepath.Join((*f).repo, "packages/padpoll/package.nix"), []byte("half-updated"), 0o644)
		os.WriteFile(filepath.Join((*f).repo, "packages/padpoll/new.json"), []byte("added"), 0o644)
	}
}

func (f *fixture) restored(t *testing.T) {
	t.Helper()
	if b, _ := os.ReadFile(filepath.Join(f.repo, "packages/padpoll/package.nix")); string(b) != "{}" {
		t.Errorf("packages/padpoll must be put back: %q", b)
	}
	if _, err := os.Stat(filepath.Join(f.repo, "packages/padpoll/new.json")); err == nil {
		t.Error("files nix-update added must go")
	}
}

func TestPkgNixUpdateFailureRestores(t *testing.T) {
	var f *fixture
	f = pkgFixture(t, "", run.Rule{Prefix: "nix-update", Fail: errors.New("exit 1"), Do: edits(&f)})
	report := filepath.Join(t.TempDir(), "r.md")
	code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, Report: report})
	if code != exit.Error {
		t.Errorf("code %d", code)
	}
	f.restored(t)
	if f.called("nix build") {
		t.Error("no build after a failed update")
	}
	if b, _ := os.ReadFile(report); !strings.Contains(string(b), "nix-update failed") {
		t.Errorf("report: %q", b)
	}
}

func TestPkgBuildFailureRestores(t *testing.T) {
	var f *fixture
	f = pkgFixture(t, "", run.Rule{Prefix: "nix-update", Do: edits(&f)}, run.Rule{Prefix: "nix build", Fail: errors.New("exit 1")})
	report := filepath.Join(t.TempDir(), "r.md")
	code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, Report: report})
	if code != exit.Error {
		t.Errorf("code %d, calls %v", code, f.e.Run.Calls())
	}
	f.restored(t)
	if !strings.Contains(f.out.String(), "build FAILED") || strings.Contains(f.out.String(), "updated (not committed)") {
		t.Errorf("%s", f.out)
	}
	b, _ := os.ReadFile(report)
	if !strings.Contains(string(b), "Build failed") || !strings.Contains(string(b), "### padpoll `1.0.0` → `1.1.0`") {
		t.Errorf("report: %q", b)
	}
}

func TestPkgDirtyPackageDir(t *testing.T) {
	dirty := run.Rule{Prefix: "git diff --quiet -- packages/padpoll", Fail: errors.New("exit 1")}
	f := pkgFixture(t, "n\n", dirty)
	if code := f.u.Pkg(update.PkgOpts{Repo: f.repo}); code != exit.OK || f.called("nix-update") {
		t.Errorf("a no leaves it alone: code %d %v", code, f.e.Run.Calls())
	}
	if s := f.out.String(); !strings.Contains(s, "packages/padpoll has uncommitted changes. Update on top of them? They are kept. [y/N]") ||
		strings.Contains(s, "Update padpoll to 1.1.0?") {
		t.Errorf("asked first, before the update question:\n%s", s)
	}

	g := pkgFixture(t, "y\ny\n", dirty)
	if g.u.Pkg(update.PkgOpts{Repo: g.repo}); !g.called("nix-update") {
		t.Errorf("two yeses update it: %s", g.out)
	}
	for _, o := range []update.PkgOpts{{AllowDirty: true}, {Yes: true}} {
		var h *fixture
		h = pkgFixture(t, "y\n", dirty, run.Rule{Prefix: "nix build", Fail: errors.New("exit 1")}, run.Rule{Prefix: "nix-update", Do: func(run.Call) {
			os.WriteFile(filepath.Join(h.repo, "packages/padpoll/package.nix"), []byte("by nix-update"), 0o644)
		}})
		os.WriteFile(filepath.Join(h.repo, "packages/padpoll/package.nix"), []byte("my edit"), 0o644)
		o.Repo = h.repo
		h.u.Pkg(o)
		if s := h.out.String(); strings.Contains(s, "Update on top") || !strings.Contains(s, "packages/padpoll has uncommitted changes: updating on top of them; they are kept.") {
			t.Errorf("%+v does not ask about it, but says so:\n%s", o, s)
		}
		if b, _ := os.ReadFile(filepath.Join(h.repo, "packages/padpoll/package.nix")); string(b) != "my edit" {
			t.Errorf("a failed build puts back the uncommitted edit, not the committed file: %q", b)
		}
	}
}

func TestPkgPassesPackageArgsAndNeverTheirVersion(t *testing.T) {
	f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"flectar":{"position":"/nix/store/x-source/packages/flectar/package.nix:1","version":"0.1.0-alpha.6","urls":[],"homepage":"https://github.com/flectar/mail","update":["nix-update","--version=unstable","--use-github-releases"]}}`})
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
	 "a":{"position":"/nix/store/x-source/packages/a/package.nix:1","version":"1.0.0","urls":["https://github.com/o/a/x"],"homepage":"","update":["nix-update"]},
	 "b":{"position":"/nix/store/x-source/packages/b/package.nix:1","version":"1.0.0","urls":["https://github.com/o/b/x"],"homepage":"","update":["nix-update"]}}`})
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
	if f.called("nix-update") || f.out.String() != "Every package is up to date.\n" {
		t.Errorf("everything is current: one line, nothing run: %q %v", f.out, f.e.Run.Calls())
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
	 "bad":{"position":"/nix/store/x-source/packages/bad/package.nix:1","version":"1.0.0","urls":["https://github.com/o/bad/x"],"homepage":"","update":["nix-update"]},
	 "good":{"position":"/nix/store/x-source/packages/good/package.nix:1","version":"1.0.0","urls":["https://github.com/o/good/x"],"homepage":"","update":["nix-update"]}}`})
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
	for _, want := range []string{"nixpkgs  aaa1111 -> bbb2222", "7 commits", "· fix: something", "Write this flake.lock to the repo? [y/N]", "flake.lock updated."} {
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

func TestLockDirtyLock(t *testing.T) {
	dirty := run.Rule{Prefix: "git diff --quiet -- flake.lock", Fail: errors.New("exit 1")}
	head := run.Rule{Prefix: "git show HEAD:flake.lock", Out: lockJSON(strings.Repeat("0", 40))}
	f := lockFixture(t, "n\n", dirty, head)
	if code := f.u.Lock(update.LockOpts{Repo: f.repo}); code != exit.OK || f.called("nix flake update") || f.lock() != lockJSON(oldRev) {
		t.Errorf("a no leaves it alone, without even running the update: code %d", code)
	}
	if !strings.Contains(f.out.String(), "flake.lock has uncommitted changes (nixpkgs). Update on top of them? They are kept. [y/N]") {
		t.Errorf("names the inputs already moved:\n%s", f.out)
	}
	for _, o := range []update.LockOpts{{AllowDirty: true, NoBuild: true}, {Yes: true, NoBuild: true}} {
		g := lockFixture(t, "y\n", dirty)
		o.Repo = g.repo
		g.u.Lock(o)
		if s := g.out.String(); strings.Contains(s, "Update on top") || !strings.Contains(s, "flake.lock has uncommitted changes: updating on top of them; they are kept.") ||
			!g.called("nix flake update") {
			t.Errorf("%+v goes on and says so (no HEAD: no names):\n%s", o, s)
		}
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

func TestPkgReportDoesNotClaimABuildThatDidNotHappen(t *testing.T) {
	for _, c := range []struct {
		noBuild bool
		want    string
		notWant string
	}{{false, "Build: ok.", "skipped"}, {true, "Build: skipped (--no-build).", "Build: ok."}} {
		f := pkgFixture(t, "")
		report := filepath.Join(t.TempDir(), "r.md")
		f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true, NoBuild: c.noBuild, Report: report})
		b, _ := os.ReadFile(report)
		if !strings.Contains(string(b), c.want) || strings.Contains(string(b), c.notWant) {
			t.Errorf("NoBuild=%v: report %q must contain %q and not %q", c.noBuild, b, c.want, c.notWant)
		}
	}
}

func TestPkgWorksWhereThePackageIsDefined(t *testing.T) {
	// A package defined outside packages/<name> (here packages/prebuilt/kopuz for
	// kopuz-bin): the dirty check, the restore and the diff all use that directory.
	f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}", "packages/prebuilt/kopuz/package.nix": "{}"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"kopuz-bin":{"position":"/nix/store/abc-source/packages/prebuilt/kopuz/package.nix:83","version":"1.0.0","urls":["https://github.com/o/kopuz/x"],"homepage":"","update":["nix-update"]}}`})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix build", Fail: errors.New("exit 1")})
	f.e.Tags("o/kopuz", "v1.0.0", "v1.1.0")
	f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true})
	if !f.called("git diff --quiet -- packages/prebuilt/kopuz") || f.called("git diff --quiet -- packages/kopuz-bin") {
		t.Errorf("the dirty check uses the real directory, not the guessed packages/<name>: %v", f.e.Run.Calls())
	}
}

func TestPkgRefusesAPackageItCannotLocate(t *testing.T) {
	for name, pos := range map[string]string{
		"no meta.position":         "",
		"defined in another flake": "/nix/store/abc-source/pkgs/elsewhere/package.nix:3",
		"outside any flake source": "/etc/nixos/thing.nix:1",
	} {
		f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
		f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"x":{"position":"` + pos + `","version":"1.0.0","urls":["https://github.com/o/x/y"],"homepage":"","update":["nix-update"]}}`})
		f.e.Tags("o/x", "v1.0.0", "v1.1.0")
		if code := f.u.Pkg(update.PkgOpts{Repo: f.repo, Yes: true}); code != exit.Error || !strings.Contains(f.out.String(), "cannot tell which file") {
			t.Errorf("%s: code %d out %s", name, code, f.out)
		}
		if f.called("nix-update") || f.called("git checkout") || f.called("git diff") {
			t.Errorf("%s: nothing may run: %v", name, f.e.Run.Calls())
		}
	}
}

// updateFixture: a flake with inputs nixpkgs and both, and packages padpoll and both.
func updateFixture(t *testing.T) *fixture {
	f := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": testutil.LockJSON(map[string]string{
		"nixpkgs": "NixOS/nixpkgs@#" + strings.Repeat("a", 40), "both": "o/both@#" + strings.Repeat("b", 40)})})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "git ls-files -z", Out: "flake.nix\x00flake.lock\x00"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{
	 "padpoll":{"position":"/nix/store/x-source/packages/padpoll/package.nix:1","version":"1.0.0","urls":["https://github.com/o/padpoll/x"],"homepage":"","update":["nix-update"]},
	 "both":{"position":"/nix/store/x-source/packages/b/package.nix:1","version":"1.0.0","urls":["https://github.com/o/b/x"],"homepage":"","update":["nix-update"]}}`})
	f.e.Tags("o/padpoll", "v1.0.0", "v1.1.0")
	f.e.Tags("o/b", "v1.0.0")
	return f
}

func flakeUpdateArgs(f *fixture) []string {
	for _, c := range f.e.Run.Calls() {
		if strings.HasPrefix(c.String(), "nix flake update") {
			return c.Args[4:]
		}
	}
	return nil
}

func TestUpdateEverythingInputsFirst(t *testing.T) {
	f := updateFixture(t)
	f.u.Update(update.Opts{Repo: f.repo, Yes: true, NoBuild: true})
	var order []string
	for _, c := range f.e.Run.Calls() {
		if strings.HasPrefix(c.String(), "nix flake update") || strings.HasPrefix(c.String(), "nix-update") {
			order = append(order, c.Name)
		}
	}
	if strings.Join(order, ",") != "nix,nix-update" || len(flakeUpdateArgs(f)) != 0 {
		t.Errorf("no names: every input (one nix flake update), then the packages: %v / %v", order, f.e.Run.Calls())
	}
}

func TestUpdateSplitsNames(t *testing.T) {
	f := updateFixture(t)
	f.u.Update(update.Opts{Repo: f.repo, Names: []string{"nixpkgs", "padpoll"}, Yes: true, NoBuild: true})
	if got := flakeUpdateArgs(f); strings.Join(got, " ") != "nixpkgs" {
		t.Errorf("inputs go to nix flake update: %v", got)
	}
	if !f.called("nix-update --flake padpoll") {
		t.Errorf("packages go to nix-update: %v", f.e.Run.Calls())
	}

	g := updateFixture(t)
	g.u.Update(update.Opts{Repo: g.repo, Names: []string{"padpoll"}, Yes: true, NoBuild: true})
	if g.called("nix flake update") || !g.called("nix-update --flake padpoll") {
		t.Errorf("only packages named: flake.lock is not touched: %v", g.e.Run.Calls())
	}
}

func TestUpdateNameErrors(t *testing.T) {
	f := updateFixture(t)
	if code := f.u.Update(update.Opts{Repo: f.repo, Names: []string{"both"}}); code != exit.Error ||
		!strings.Contains(f.errb.String(), "both an input and a package: pick one with --source") {
		t.Errorf("ambiguous: code %d err %q", code, f.errb)
	}
	g := updateFixture(t)
	g.u.Update(update.Opts{Repo: g.repo, Names: []string{"both"}, Sources: []string{"package"}, Yes: true, NoBuild: true})
	if g.called("nix flake update") || !g.called("nix eval --json .#packages.") || g.errb.Len() != 0 {
		t.Errorf("--source settles it: %q %v", g.errb, g.e.Run.Calls())
	}
	h := updateFixture(t)
	if code := h.u.Update(update.Opts{Repo: h.repo, Names: []string{"padpoll"}, Sources: []string{"input"}}); code != exit.Error ||
		!strings.Contains(h.errb.String(), "no input called padpoll") {
		t.Errorf("a package name with --source input: code %d err %q", code, h.errb)
	}
	if h.called("nix flake update") || h.called("nix-update") {
		t.Errorf("a bad name stops everything before it starts: %v", h.e.Run.Calls())
	}
}

func TestUpdateSourcesAndWorstCode(t *testing.T) {
	f := updateFixture(t)
	f.u.Update(update.Opts{Repo: f.repo, Sources: []string{"input"}, Yes: true, NoBuild: true})
	if f.called("nix-update") || !f.called("nix flake update") {
		t.Errorf("--source input: no packages: %v", f.e.Run.Calls())
	}
	g := updateFixture(t)
	g.e.Run.On(run.Rule{Prefix: "nix-update", Fail: errors.New("exit 1")})
	g.e.Run.Rules = append([]run.Rule{{Prefix: "git diff --quiet -- flake.lock", Fail: errors.New("dirty")}}, g.e.Run.Rules...)
	if code := g.u.Update(update.Opts{Repo: g.repo, Yes: true, NoBuild: true}); code != exit.Error {
		t.Errorf("a failed package (2): the worst wins, got %d", code)
	}
	if !g.called("nix-update") || !g.called("nix flake update") {
		t.Errorf("-y goes on over a dirty flake.lock, then the packages: %v", g.e.Run.Calls())
	}
	h := updateFixture(t)
	h.e.Run.Rules = append([]run.Rule{{Prefix: "nix flake update", Fail: errors.New("exit 1")}}, h.e.Run.Rules...)
	if code := h.u.Update(update.Opts{Repo: h.repo, Yes: true, NoBuild: true}); code != exit.Error || !h.called("nix-update --flake padpoll") {
		t.Errorf("a failed lock (2) is not hidden by packages that went fine: code %d", code)
	}
}

func TestUpdateNameErrorsWithSource(t *testing.T) {
	f := updateFixture(t)
	if code := f.u.Update(update.Opts{Repo: f.repo, Names: []string{"nixpkgs"}, Sources: []string{"package"}}); code != exit.Error ||
		!strings.Contains(f.errb.String(), "nixpkgs is an input, which --source leaves out") {
		t.Errorf("an input with --source package says so: code %d err %q", code, f.errb)
	}
	g := updateFixture(t)
	if code := g.u.Update(update.Opts{Repo: g.repo, Names: []string{"padpoll"}, Sources: []string{"input"}}); code != exit.Error ||
		!strings.Contains(g.errb.String(), "no input called padpoll") || g.called("nix eval") {
		t.Errorf("with --source input packages are not evaluated: code %d err %q %v", code, g.errb, g.e.Run.Calls())
	}
}

func TestPkgAsksOnlyAboutPackagesItCanUpdate(t *testing.T) {
	f := newFixture(t, "y\n", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"x":{"position":"/elsewhere/x.nix:1","version":"1.0.0","urls":["https://github.com/o/x/y"],"homepage":"","update":["nix-update"]}}`})
	f.e.Tags("o/x", "v1.0.0", "v1.1.0")
	f.u.Pkg(update.PkgOpts{Repo: f.repo})
	if strings.Contains(f.out.String(), "[y/N]") {
		t.Errorf("a package that would be refused is refused before the question:\n%s", f.out)
	}
}

// ---- Flatpak and nix profile ------------------------------------------------------------

func flatpakFixture(t *testing.T, answers string, extra ...run.Rule) *fixture {
	f := newFixture(t, answers, map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	for _, r := range extra {
		f.e.Run.On(r)
	}
	f.e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates", Out: "app/co.hyprlab.Hylki/x86_64/stable\t\nruntime/org.gnome.Platform/x86_64/50\t\n"})
	f.e.Run.On(run.Rule{Prefix: "flatpak list --columns=ref,version", Out: "co.hyprlab.Hylki/x86_64/stable\t1.41.1\norg.gnome.Platform/x86_64/50\t\n"})
	f.e.Run.On(run.Rule{Prefix: "flatpak list --columns=application", Out: "co.hyprlab.Hylki\ncom.x.Current\n"})
	f.e.Run.On(run.Rule{Prefix: "flatpak info --show-commit app/co.hyprlab.Hylki", Out: strings.Repeat("4", 64) + "\n"})
	f.e.Run.On(run.Rule{Prefix: "flatpak info --show-commit runtime/org.gnome.Platform", Out: strings.Repeat("b", 64) + "\n"})
	return f
}

func TestFlatpakStep(t *testing.T) {
	f := flatpakFixture(t, "y\n")
	report := filepath.Join(t.TempDir(), "r.md")
	if code := f.u.Flatpak(update.FlatpakOpts{Report: report}); code != exit.OK {
		t.Fatalf("code %d\n%s", code, f.out)
	}
	s := f.out.String()
	for _, want := range []string{
		"SOURCE   NAME                CURRENT  LATEST",
		"flatpak  co.hyprlab.Hylki    1.41.1   unknown",
		"Update 2 Flatpak refs now? This installs them right away. [y/N]",
		"flatpak update --commit=" + strings.Repeat("4", 64) + " app/co.hyprlab.Hylki/x86_64/stable",
		"flatpak update --commit=" + strings.Repeat("b", 64) + " runtime/org.gnome.Platform/x86_64/50",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if !f.called("flatpak update -y --noninteractive app/co.hyprlab.Hylki/x86_64/stable runtime/org.gnome.Platform/x86_64/50") {
		t.Errorf("exactly the refs shown: %v", f.e.Run.Calls())
	}
	var infoAt, updateAt int
	for i, c := range f.e.Run.Calls() {
		switch {
		case strings.HasPrefix(c.String(), "flatpak info --show-commit"):
			infoAt = i
		case strings.HasPrefix(c.String(), "flatpak update"):
			updateAt = i
		}
	}
	if infoAt > updateAt {
		t.Error("the deployed commits are read before updating")
	}
	if b, _ := os.ReadFile(report); !strings.Contains(string(b), "| co.hyprlab.Hylki | 1.41.1 | unknown |") || !strings.Contains(string(b), "--commit=") {
		t.Errorf("report: %s", b)
	}
}

func TestFlatpakStepNoAndNames(t *testing.T) {
	f := flatpakFixture(t, "n\n")
	if code := f.u.Flatpak(update.FlatpakOpts{}); code != exit.OK || !strings.Contains(f.out.String(), "Skipped.") || f.called("flatpak update") {
		t.Errorf("a no changes nothing: code %d %s", code, f.out)
	}
	g := flatpakFixture(t, "")
	g.u.Flatpak(update.FlatpakOpts{IDs: []string{"co.hyprlab.Hylki", "com.x.Current"}, Yes: true})
	if !g.called("flatpak update -y --noninteractive app/co.hyprlab.Hylki/x86_64/stable") || g.called("flatpak update -y --noninteractive app/co.hyprlab.Hylki/x86_64/stable runtime") {
		t.Errorf("only the named app: %v", g.e.Run.Calls())
	}
	i := flatpakFixture(t, "n\n")
	if i.u.Flatpak(update.FlatpakOpts{IDs: []string{"co.hyprlab.Hylki"}}); !strings.Contains(i.out.String(), "Update 1 Flatpak ref now? This installs it right away.") {
		t.Errorf("singular: %s", i.out)
	}
	if false {
		t.Errorf("only the named app: %v", g.e.Run.Calls())
	}
	if !strings.Contains(g.out.String(), "com.x.Current is up to date.") {
		t.Errorf("a named app without an update says so: %s", g.out)
	}
	h := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	if h.u.Flatpak(update.FlatpakOpts{}); !strings.Contains(h.out.String(), "Every Flatpak is up to date.") {
		t.Errorf("nothing pending: %s", h.out)
	}
}

func TestFlatpakStepFailure(t *testing.T) {
	f := flatpakFixture(t, "", run.Rule{Prefix: "flatpak update", Fail: errors.New("exit 1")})
	report := filepath.Join(t.TempDir(), "r.md")
	if code := f.u.Flatpak(update.FlatpakOpts{Yes: true, Report: report}); code != exit.Error {
		t.Errorf("code %d", code)
	}
	if s := f.out.String(); !strings.Contains(s, "flatpak update failed") || !strings.Contains(s, "--commit=") {
		t.Errorf("a failure still says how to go back, since some refs may have moved:\n%s", s)
	}
	if b, _ := os.ReadFile(report); !strings.Contains(string(b), "flatpak update failed") {
		t.Errorf("report: %s", b)
	}
}

func profileFixture(t *testing.T, answers string, extra ...run.Rule) *fixture {
	f := newFixture(t, answers, map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	for _, r := range extra {
		f.e.Run.On(r)
	}
	f.e.Run.On(run.Rule{Prefix: "nix profile list --json", Out: `{"elements":{
		"qt":{"originalUrl":"github:NixOS/nixpkgs/` + strings.Repeat("a", 40) + `"},
		"hello":{"originalUrl":"flake:nixpkgs"},
		"tool":{"originalUrl":"github:o/tool"}}}`})
	return f
}

func TestProfileStep(t *testing.T) {
	f := profileFixture(t, "y\n")
	if code := f.u.Profile(update.ProfileOpts{}); code != exit.OK {
		t.Fatalf("code %d\n%s", code, f.out)
	}
	if !f.called("nix profile upgrade hello tool") || f.called("nix profile upgrade hello tool qt") {
		t.Errorf("only the entries that follow a moving reference: %v", f.e.Run.Calls())
	}
	s := f.out.String()
	for _, want := range []string{
		"qt is pinned to aaaaaaa: nix profile upgrade cannot move it. To update it, nix profile remove qt and install it again.",
		"Upgrade 2 nix profile entries (hello, tool) now? This installs them right away. [y/N]",
		"To undo it: nix profile rollback",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestProfileStepNamesNoAndFailure(t *testing.T) {
	f := profileFixture(t, "", run.Rule{Prefix: "nix profile upgrade", Fail: errors.New("exit 1")})
	if code := f.u.Profile(update.ProfileOpts{Names: []string{"tool"}, Yes: true}); code != exit.Error || !f.called("nix profile upgrade tool") {
		t.Errorf("named entry, failed: code %d %v", code, f.e.Run.Calls())
	}
	g := profileFixture(t, "n\n")
	if g.u.Profile(update.ProfileOpts{}); g.called("nix profile upgrade") || !strings.Contains(g.out.String(), "Skipped.") {
		t.Errorf("a no changes nothing: %s", g.out)
	}
	h := profileFixture(t, "")
	if h.u.Profile(update.ProfileOpts{Names: []string{"qt"}}); h.called("nix profile upgrade") {
		t.Errorf("a pinned entry is never upgraded: %v", h.e.Run.Calls())
	}
}

func TestUpdateRunsTheFourStepsInOrder(t *testing.T) {
	f := updateFixture(t)
	f.e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates", Out: "app/io.x.App/x86_64/stable\t2.0\n"})
	f.e.Run.On(run.Rule{Prefix: "nix profile list --json", Out: `{"elements":{"hello":{"originalUrl":"flake:nixpkgs"}}}`})
	f.u.Update(update.Opts{Repo: f.repo, Yes: true, NoBuild: true})
	var order []string
	for _, c := range f.e.Run.Calls() {
		for _, step := range []string{"nix flake update", "nix-update", "flatpak update", "nix profile upgrade"} {
			if strings.HasPrefix(c.String(), step) {
				order = append(order, step)
			}
		}
	}
	if strings.Join(order, ",") != "nix flake update,nix-update,flatpak update,nix profile upgrade" {
		t.Errorf("inputs, packages, flatpak, profile: %v", order)
	}

	h := updateFixture(t)
	h.e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates", Out: "app/io.x.App/x86_64/stable\t2.0\n"})
	h.e.Run.Rules = append([]run.Rule{{Prefix: "flatpak update", Fail: errors.New("exit 1")}}, h.e.Run.Rules...)
	if code := h.u.Update(update.Opts{Repo: h.repo, Yes: true, NoBuild: true}); code != exit.Error {
		t.Errorf("a failed Flatpak step counts in the exit code: %d", code)
	}

	g := updateFixture(t)
	g.e.Run.Missing = map[string]bool{"flatpak": true}
	g.u.Update(update.Opts{Repo: g.repo, Yes: true, NoBuild: true})
	if strings.Contains(g.out.String(), "Flatpak") {
		t.Errorf("without flatpak the step is skipped silently: %s", g.out)
	}
	k := newFixture(t, "", map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
	k.e.Run.Missing = map[string]bool{"nix": true, "flatpak": true}
	k.u.Update(update.Opts{Repo: k.repo, Sources: []string{"profile"}})
	if k.out.Len() != 0 {
		t.Errorf("without nix the profile step is skipped silently: %q", k.out)
	}
}

func TestUpdateNamesAcrossSources(t *testing.T) {
	f := updateFixture(t)
	f.e.Run.On(run.Rule{Prefix: "flatpak list --columns=application", Out: "io.x.App\nboth\n"})
	f.e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates", Out: "app/io.x.App/x86_64/stable\t2.0\n"})
	f.u.Update(update.Opts{Repo: f.repo, Names: []string{"io.x.App"}, Yes: true})
	if !f.called("flatpak update -y --noninteractive app/io.x.App/x86_64/stable") || f.called("nix flake update") || f.called("nix-update") {
		t.Errorf("a Flatpak app ID goes to the Flatpak step only: %v", f.e.Run.Calls())
	}
	r := updateFixture(t)
	r.e.Run.On(run.Rule{Prefix: "flatpak list --columns=application", Out: "org.gnome.Platform\n"})
	r.e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates", Out: "runtime/org.gnome.Platform/x86_64/50\t\n"})
	r.u.Update(update.Opts{Repo: r.repo, Names: []string{"org.gnome.Platform"}, Yes: true})
	if !r.called("flatpak update -y --noninteractive runtime/org.gnome.Platform/x86_64/50") {
		t.Errorf("a runtime can be named, as check suggests: %q %v", r.errb, r.e.Run.Calls())
	}

	g := updateFixture(t)
	g.e.Run.On(run.Rule{Prefix: "flatpak list --columns=application", Out: "both\n"})
	if code := g.u.Update(update.Opts{Repo: g.repo, Names: []string{"both"}}); code != exit.Error ||
		!strings.Contains(g.errb.String(), "both is an input, a package and a Flatpak app: pick one with --source input, --source package or --source flatpak") {
		t.Errorf("three-way: code %d err %q", code, g.errb)
	}
}

func TestUpdatePassesAllowDirtyToPackages(t *testing.T) {
	f := updateFixture(t)
	f.e.Run.Rules = append([]run.Rule{{Prefix: "git diff --quiet -- packages/padpoll", Fail: errors.New("dirty")}}, f.e.Run.Rules...)
	f.u.Update(update.Opts{Repo: f.repo, Names: []string{"padpoll"}, AllowDirty: true, NoBuild: true})
	if s := f.out.String(); !strings.Contains(s, "packages/padpoll has uncommitted changes: updating on top of them") || strings.Contains(s, "Update on top") {
		t.Errorf("--allow-dirty reaches the package step:\n%s", s)
	}
}

func TestUpdateNextOnceAtTheEnd(t *testing.T) {
	f := updateFixture(t)
	f.e.Run.On(run.Rule{Prefix: "nix flake update", Do: func(c run.Call) {
		os.WriteFile(filepath.Join(c.Args[3], "flake.lock"), []byte(testutil.LockJSON(map[string]string{
			"nixpkgs": "NixOS/nixpkgs@#" + strings.Repeat("c", 40), "both": "o/both@#" + strings.Repeat("b", 40)})), 0o644)
	}})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json", Out: `["desktop"]`})
	f.e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates", Out: "app/io.x.App/x86_64/stable\t2.0\n"})
	f.u.Update(update.Opts{Repo: f.repo, Yes: true, NoBuild: true})
	s := f.out.String()
	next := "Next: review with git diff, then nh os test " + f.repo + " -H desktop (nothing is committed or activated)"
	if strings.Count(s, "Next:") != 1 || !strings.HasSuffix(strings.TrimSpace(s), next) {
		t.Errorf("one Next line, last, after Flatpak:\n%s", s)
	}
	if strings.Contains(s, "transitive inputs changed") {
		t.Errorf("no transitive line when none changed:\n%s", s)
	}

	g := updateFixture(t)
	g.e.Run.On(run.Rule{Prefix: "nix eval --json", Out: `["desktop","laptop"]`})
	g.u.Update(update.Opts{Repo: g.repo, Names: []string{"padpoll"}, Yes: true, NoBuild: true})
	if !strings.Contains(g.out.String(), "nh os test "+g.repo+" -H <host>") {
		t.Errorf("a package alone also needs nh; with two hosts, a placeholder:\n%s", g.out)
	}

	h := flatpakFixture(t, "")
	h.u.Update(update.Opts{Repo: h.repo, Sources: []string{"flatpak"}, Yes: true})
	if strings.Contains(h.out.String(), "Next:") {
		t.Errorf("Flatpak installs itself: nothing for nh:\n%s", h.out)
	}
	k := updateFixture(t)
	k.e.Run.On(run.Rule{Prefix: "nix flake update", Do: func(c run.Call) {
		os.WriteFile(filepath.Join(c.Args[3], "flake.lock"), []byte(testutil.LockJSON(map[string]string{
			"nixpkgs": "NixOS/nixpkgs@#" + strings.Repeat("c", 40), "both": "o/both@#" + strings.Repeat("b", 40)})), 0o644)
	}})
	k.u.Update(update.Opts{Repo: k.repo, Sources: []string{"input"}, NoBuild: true}) // no terminal: every answer is no
	if strings.Contains(k.out.String(), "Next:") {
		t.Errorf("nothing written, no Next:\n%s", k.out)
	}
}

func TestUpdateNextTarget(t *testing.T) {
	moved := run.Rule{Prefix: "nix flake update", Do: func(c run.Call) {
		os.WriteFile(filepath.Join(c.Args[3], "flake.lock"), []byte(testutil.LockJSON(map[string]string{
			"nixpkgs": "NixOS/nixpkgs@#" + strings.Repeat("c", 40), "both": "o/both@#" + strings.Repeat("b", 40)})), 0o644)
	}}
	inputs := func(f *fixture, hosts ...string) string {
		f.e.Run.Rules = append([]run.Rule{moved, {Prefix: "nix eval --json", Out: `["desktop","laptop"]`}}, f.e.Run.Rules...)
		f.u.Update(update.Opts{Repo: f.repo, Sources: []string{"input"}, Hosts: hosts, Yes: true, NoBuild: true})
		return f.out.String()
	}
	f := updateFixture(t)
	if s := inputs(f, "laptop"); !strings.Contains(s, "nh os test "+f.repo+" -H laptop") {
		t.Errorf("the inputs alone, and the host given with -H:\n%s", s)
	}
	g := updateFixture(t)
	t.Chdir(g.repo)
	if s := inputs(g); !strings.Contains(s, "nh os test . -H <host>") {
		t.Errorf("from inside the repository, '.':\n%s", s)
	}
	h := updateFixture(t)
	t.Chdir(t.TempDir())
	t.Setenv("HOME", filepath.Dir(h.repo))
	if s := inputs(h); !strings.Contains(s, "nh os test ~/"+filepath.Base(h.repo)+" -H <host>") {
		t.Errorf("under the home directory, with ~:\n%s", s)
	}
}
