package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/cli"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/testutil"
)

type fixture struct {
	e         *testutil.Env
	repo      string
	out, errb *bytes.Buffer
	deps      cli.Deps
}

func newFixture(t *testing.T, inputs map[string]string) *fixture {
	t.Helper()
	e := testutil.New(t)
	term, out, errb := testutil.Term("")
	repo := testutil.Tree(t, map[string]string{"flake.nix": "{}", "flake.lock": testutil.LockJSON(inputs)})
	return &fixture{e: e, repo: repo, out: out, errb: errb, deps: cli.Deps{T: term, R: e.Run, GH: e.GH, Env: func(string) string { return "" }}}
}

func (f *fixture) run(args ...string) int { return cli.Run(args, f.deps) }

func TestVersionAndHelp(t *testing.T) {
	f := newFixture(t, nil)
	if code := f.run("version"); code != exit.OK || strings.TrimSpace(f.out.String()) != "nixup dev" {
		t.Errorf("code %d out %q", code, f.out)
	}
	f.out.Reset()
	if code := f.run("help"); code != exit.OK || !strings.Contains(f.out.String(), "nixup lock") {
		t.Errorf("code %d out %q", code, f.out)
	}
}

func TestHelpMentionsHowTheFlakeIsFound(t *testing.T) {
	f := newFixture(t, nil)
	f.run("help")
	for _, want := range []string{"NIXUP_FLAKE", "NH_OS_FLAKE", "NH_FLAKE", "upwards"} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("help must mention %s: %q", want, f.out)
		}
	}
}

func TestVersionAndHelpFlags(t *testing.T) {
	for _, flag := range []string{"-version", "--version", "-v"} {
		f := newFixture(t, nil)
		if code := f.run(flag); code != exit.OK || strings.TrimSpace(f.out.String()) != "nixup dev" {
			t.Errorf("%s: code %d out %q", flag, code, f.out)
		}
	}
	for _, flag := range []string{"-h", "--help"} {
		f := newFixture(t, nil)
		if code := f.run(flag); code != exit.OK || !strings.Contains(f.out.String(), "nixup lock") {
			t.Errorf("%s: code %d out %q", flag, code, f.out)
		}
	}
	// A subcommand keeps its own -h.
	f := newFixture(t, nil)
	f.run("check", "-h")
	if !strings.Contains(f.errb.String(), "-only") {
		t.Errorf("check -h must list check's flags: %q", f.errb)
	}
}

func TestUnknownFlagExplainsTheAssumedCommand(t *testing.T) {
	f := newFixture(t, nil)
	if code := f.run("--bogus"); code != exit.Error {
		t.Errorf("code %d", code)
	}
	if !strings.Contains(f.errb.String(), "'check' was assumed") {
		t.Errorf("a flag error without a typed command must say check was assumed: %q", f.errb)
	}
	g := newFixture(t, nil)
	g.run("check", "--bogus")
	if strings.Contains(g.errb.String(), "was assumed") {
		t.Errorf("an explicit command needs no such hint: %q", g.errb)
	}
}

func TestVersionString(t *testing.T) {
	oldV, oldC := cli.Version, cli.Commit
	defer func() { cli.Version, cli.Commit = oldV, oldC }()
	cli.Version, cli.Commit = "1.2.3", "a4a2eed"
	f := newFixture(t, nil)
	f.run("--version")
	if got := strings.TrimSpace(f.out.String()); got != "nixup 1.2.3 (a4a2eed)" {
		t.Errorf("got %q", got)
	}
}

func TestUnknownCommand(t *testing.T) {
	f := newFixture(t, nil)
	if code := f.run("frobnicate"); code != exit.Error {
		t.Errorf("code %d", code)
	}
	if !strings.Contains(f.errb.String(), "nixup lock") || f.out.Len() != 0 {
		t.Errorf("usage goes to stderr: out=%q err=%q", f.out, f.errb)
	}
}

func TestFlagProblems(t *testing.T) {
	f := newFixture(t, nil)
	if code := f.run("check", "-C", f.repo, "--bogus"); code != exit.Error {
		t.Errorf("unknown flag: code %d", code)
	}
	if code := f.run("check", "-h"); code != exit.OK {
		t.Errorf("-h is not an error: code %d", code)
	}
	if code := f.run("check", "-C", filepath.Join(t.TempDir(), "nowhere")); code != exit.Error ||
		!strings.Contains(f.errb.String(), "no flake.nix") {
		t.Errorf("code %d err %q", code, f.errb)
	}
}

func TestCheckDefaultsToCheck(t *testing.T) {
	f := newFixture(t, map[string]string{"np": "o/np@#aaa"})
	f.e.Head("o/np", "", "aaa")
	// `nixup -C dir` without a subcommand is `nixup check -C dir`.
	if code := f.run("-C", f.repo, "--only", "inputs"); code != exit.OK {
		t.Errorf("code %d\n%s%s", code, f.out, f.errb)
	}
	if !strings.Contains(f.out.String(), "1 up to date") {
		t.Errorf("%q", f.out)
	}
}

func TestCheckExitCodeAndJSON(t *testing.T) {
	f := newFixture(t, map[string]string{"np": "o/np@#aaa1111"})
	f.e.Head("o/np", "", "bbb2222")
	code := f.run("check", "-C", f.repo, "--only", "inputs", "--json")
	if code != exit.Pending {
		t.Errorf("outdated must exit 1, got %d", code)
	}
	var rows []check.Row
	if err := json.Unmarshal(f.out.Bytes(), &rows); err != nil {
		t.Fatalf("--json must be valid JSON: %v\n%s", err, f.out)
	}
	if len(rows) != 1 || rows[0].Name != "np" || rows[0].Status != check.Outdated || rows[0].Latest != "bbb2222" {
		t.Errorf("rows: %+v", rows)
	}
}

func TestCheckErrorExitCode(t *testing.T) {
	f := newFixture(t, map[string]string{"np": "o/np@#aaa"})
	f.e.Run.On(run.Rule{Prefix: "git ls-remote", Fail: os.ErrDeadlineExceeded, Err: "fatal: timeout\n"})
	if code := f.run("check", "-C", f.repo, "--only", "inputs"); code != exit.Error {
		t.Errorf("a failing check exits 2, got %d", code)
	}
}

func TestCheckMarkdown(t *testing.T) {
	f := newFixture(t, map[string]string{"np": "o/np@#aaa1111"})
	f.e.Head("o/np", "", "bbb2222")
	f.run("check", "-C", f.repo, "--only", "inputs", "--markdown", "--review")
	if !strings.Contains(f.out.String(), "### inputs / np `aaa1111 (") {
		t.Errorf("%q", f.out)
	}
	f2 := newFixture(t, map[string]string{"np": "o/np@#aaa"})
	f2.e.Head("o/np", "", "aaa")
	f2.run("check", "-C", f2.repo, "--only", "inputs", "--markdown")
	if f2.out.String() != "Everything is up to date.\n" {
		t.Errorf("%q", f2.out)
	}
}

func TestCheckUnknownSection(t *testing.T) {
	f := newFixture(t, nil)
	if code := f.run("check", "-C", f.repo, "--only", "inputs,bogus"); code != exit.Error ||
		!strings.Contains(f.errb.String(), `unknown section "bogus"`) {
		t.Errorf("code %d err %q", code, f.errb)
	}
	if f.e.Run.Count("git") != 0 {
		t.Error("validate before doing any work")
	}
}

func TestLockParsesFlagsAfterInputs(t *testing.T) {
	f := newFixture(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#aaa"})
	f.e.Run.On(run.Rule{Prefix: "git ls-files -z", Out: "flake.nix\x00flake.lock\x00"})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	// `nixup lock nixpkgs home-manager --no-build -C repo`: Go's flag package alone stops at "nixpkgs".
	f.run("lock", "nixpkgs", "home-manager", "--no-build", "-C", f.repo)
	for _, c := range f.e.Run.Calls() {
		if strings.HasPrefix(c.String(), "nix flake update") {
			if !strings.HasSuffix(c.String(), "nixpkgs home-manager") {
				t.Errorf("inputs were not forwarded: %s", c)
			}
			return
		}
	}
	t.Errorf("nix flake update did not run: %v", f.e.Run.Calls())
}

func TestLockRepeatedHostFlags(t *testing.T) {
	f := newFixture(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#aaa"})
	f.e.Run.On(run.Rule{Prefix: "git ls-files -z", Out: "flake.nix\x00flake.lock\x00"})
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix flake update", Do: func(c run.Call) {
		os.WriteFile(filepath.Join(c.Args[3], "flake.lock"), []byte(testutil.LockJSON(map[string]string{"nixpkgs": "NixOS/nixpkgs@#bbb"})), 0o644)
	}})
	f.run("lock", "-C", f.repo, "-H", "desktop", "-H", "zenbook")
	if f.e.Run.Count("nh os build") != 2 || f.e.Run.Count("nix eval --json") != 0 {
		t.Errorf("calls: %v", f.e.Run.Calls())
	}
}

func TestPkgFlagsAfterNames(t *testing.T) {
	f := newFixture(t, nil)
	f.e.Run.On(run.Rule{Prefix: "git diff --quiet"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"padpoll":{"version":"1.0.0","urls":["https://github.com/o/padpoll/x"],"homepage":"","update":["nix-update"]}}`})
	f.e.Tags("o/padpoll", "v1.0.0", "v1.1.0")
	// The bug this guards: `pkg padpoll --no-build -y` once read "--no-build" as a package name.
	if code := f.run("pkg", "padpoll", "--no-build", "-y", "-C", f.repo); code != exit.OK {
		t.Fatalf("code %d\n%s%s", code, f.out, f.errb)
	}
	if f.e.Run.Count("nix-update --flake padpoll --version=1.1.0") != 1 || f.e.Run.Count("nix build") != 0 {
		t.Errorf("calls: %v", f.e.Run.Calls())
	}
}

func TestPkgUnknownName(t *testing.T) {
	f := newFixture(t, nil)
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: "{}"})
	if code := f.run("pkg", "nope", "-C", f.repo); code != exit.Error || !strings.Contains(f.errb.String(), "no such package: nope") {
		t.Errorf("code %d err %q", code, f.errb)
	}
}

func TestFlakeFromEnvironment(t *testing.T) {
	f := newFixture(t, map[string]string{"np": "o/np@#aaa"})
	f.e.Head("o/np", "", "aaa")
	other := t.TempDir()
	t.Chdir(other) // nowhere near a flake
	f.deps.Env = func(k string) string {
		if k == "NH_FLAKE" {
			return f.repo + "#desktop"
		}
		return ""
	}
	if code := f.run("check", "--only", "inputs", "--json"); code != exit.OK {
		t.Fatalf("code %d\n%s%s", code, f.out, f.errb)
	}
	if !strings.Contains(f.errb.String(), "using "+f.repo+" ($NH_FLAKE)") {
		t.Errorf("must say which flake it picked, on stderr: %q", f.errb)
	}
	var rows []check.Row
	if err := json.Unmarshal(f.out.Bytes(), &rows); err != nil {
		t.Errorf("the notice must not pollute stdout (--json): %v", err)
	}
}

func TestFlagBeatsEnvironmentAndStaysQuiet(t *testing.T) {
	f := newFixture(t, map[string]string{"np": "o/np@#aaa"})
	f.e.Head("o/np", "", "aaa")
	f.deps.Env = func(string) string { return "/does/not/exist" }
	if code := f.run("check", "-C", f.repo, "--only", "inputs"); code != exit.OK {
		t.Fatalf("code %d\n%s", code, f.errb)
	}
	if strings.Contains(f.errb.String(), "using") {
		t.Errorf("an explicit -C needs no notice: %q", f.errb)
	}
}

func TestBadEnvironmentFlakeIsAnError(t *testing.T) {
	f := newFixture(t, nil)
	f.deps.Env = func(k string) string {
		if k == "NIXUP_FLAKE" {
			return "github:Villoh/nixos-config"
		}
		return ""
	}
	if code := f.run("check"); code != exit.Error || !strings.Contains(f.errb.String(), "$NIXUP_FLAKE") {
		t.Errorf("code %d err %q", code, f.errb)
	}
	if f.e.Run.Count("git") != 0 {
		t.Error("fail before doing any work")
	}
}

func TestUnknownCommandSaysSo(t *testing.T) {
	f := newFixture(t, nil)
	if code := f.run("lis"); code != exit.Error || !strings.Contains(f.errb.String(), `unknown command "lis"`) || !strings.Contains(f.errb.String(), "nixup list") {
		t.Errorf("code %d err %q", code, f.errb)
	}
}

func TestListCommand(t *testing.T) {
	f := newFixture(t, map[string]string{"dms": "o/dms@v1.6.2#" + strings.Repeat("1", 40)})
	f.e.Run.Missing = map[string]bool{"flatpak": true}
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: "{}"})
	if code := f.run("list", "-C", f.repo); code != exit.OK || !strings.Contains(f.out.String(), "v1.6.2") {
		t.Errorf("code %d out %q err %q", code, f.out, f.errb)
	}
	f.out.Reset()
	f.run("list", "-C", f.repo, "--json")
	var groups []map[string]any
	if err := json.Unmarshal(f.out.Bytes(), &groups); err != nil || len(groups) == 0 || groups[0]["source"] != "inputs" {
		t.Errorf("json: %v %q", err, f.out)
	}
}

func TestSearchCommand(t *testing.T) {
	f := newFixture(t, nil)
	f.e.Run.Missing = map[string]bool{"flatpak": true}
	f.e.Run.On(run.Rule{Prefix: "nix eval --json --impure", Out: `{"rev":"abc","nixpkgs":{"total":1,"items":[{"name":"rg","pname":"rg","version":"1","description":"d"}]},"own":{"total":0,"items":[]},"inputs":{}}`})
	if code := f.run("search", "rg", "-C", f.repo, "--limit", "3"); code != exit.OK || !strings.Contains(f.out.String(), "nixpkgs abc") {
		t.Errorf("code %d out %q err %q", code, f.out, f.errb)
	}
	for _, args := range [][]string{{"search"}, {"search", "a", "b"}} {
		g := newFixture(t, nil)
		if code := g.run(append(args, "-C", g.repo)...); code != exit.Error || !strings.Contains(g.errb.String(), "usage: nixup search") {
			t.Errorf("%v: code %d err %q", args, code, g.errb)
		}
	}
	h := newFixture(t, nil)
	h.e.Run.Missing = map[string]bool{"flatpak": true}
	h.e.Run.On(run.Rule{Prefix: "nix eval", Out: `{"rev":"","nixpkgs":{"total":0,"items":[]},"own":{"total":0,"items":[]},"inputs":{}}`})
	if code := h.run("search", "zz", "-C", h.repo); code != exit.Pending || !strings.Contains(h.out.String(), "Nothing called") {
		t.Errorf("nothing found is exit 1: code %d out %q", code, h.out)
	}
}
