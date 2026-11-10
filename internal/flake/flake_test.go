package flake_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/testutil"
)

func lock(t *testing.T, inputs map[string]string) *flake.Lock {
	t.Helper()
	l, err := flake.ParseLock([]byte(testutil.LockJSON(inputs)))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestLockDirectSkipsFollows(t *testing.T) {
	l, err := flake.ParseLock([]byte(`{"root":"root","nodes":{
		"root":{"inputs":{"nixpkgs":"nixpkgs","hm":["nixpkgs"],"dms":"dms_2"}},
		"nixpkgs":{"locked":{"rev":"a"}},"dms_2":{"locked":{"rev":"b"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Names(); !reflect.DeepEqual(got, []string{"dms", "nixpkgs"}) {
		t.Errorf("names = %v: a follows list is not an input", got)
	}
	if n, ok := l.Input("dms"); !ok || n.Locked.Rev != "b" {
		t.Errorf("Input must resolve the node key (dms -> dms_2): %+v", n)
	}
	if _, ok := l.Input("hm"); ok {
		t.Error("hm only follows nixpkgs")
	}
}

func TestParseLockRejectsGarbage(t *testing.T) {
	if _, err := flake.ParseLock([]byte("not json")); err == nil || !strings.Contains(err.Error(), "flake.lock") {
		t.Errorf("err = %v", err)
	}
	if _, err := flake.ReadLock(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a missing file is an error")
	}
}

func TestDiff(t *testing.T) {
	old := lock(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#aaa", "dms": "x/dms@v1.6.2#bbb", "sub": "x/sub@#ccc"})
	cur := lock(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#zzz", "dms": "x/dms@v1.6.2#bbb", "sub": "x/sub@#ccc"})
	changes, transitive := flake.Diff(old, cur)
	want := []flake.Change{{Name: "nixpkgs", Repo: "NixOS/nixpkgs", Old: "aaa", New: "zzz"}}
	if !reflect.DeepEqual(changes, want) || transitive != 0 {
		t.Errorf("changes=%+v transitive=%d", changes, transitive)
	}
	if c, n := flake.Diff(old, old); len(c) != 0 || n != 0 {
		t.Error("identical locks produce no diff")
	}
}

func TestDiffCountsTransitiveAndNewInputs(t *testing.T) {
	old, _ := flake.ParseLock([]byte(`{"root":"r","nodes":{"r":{"inputs":{"a":"a"}},"a":{"locked":{"rev":"1"}},"dep":{"locked":{"rev":"1"}}}}`))
	cur, _ := flake.ParseLock([]byte(`{"root":"r","nodes":{"r":{"inputs":{"a":"a","b":"b"}},"a":{"locked":{"rev":"1"}},"b":{"locked":{"rev":"9"},"original":{"type":"github","owner":"o","repo":"b","ref":"main"}},"dep":{"locked":{"rev":"2"}}}}`))
	changes, transitive := flake.Diff(old, cur)
	if len(changes) != 1 || changes[0].Name != "b" || changes[0].Old != "" || changes[0].Repo != "o/b" || changes[0].Ref != "main" {
		t.Errorf("a new input shows as a change from nothing: %+v", changes)
	}
	if transitive != 1 {
		t.Errorf("transitive = %d, want 1", transitive)
	}
}

func TestFindRepo(t *testing.T) {
	root := testutil.Tree(t, map[string]string{"flake.nix": "{}", "flake.lock": "{}", "modules/home/x.nix": ""})
	for _, start := range []string{root, filepath.Join(root, "modules", "home")} {
		got, err := flake.FindRepo(start)
		if err != nil || got != root {
			t.Errorf("FindRepo(%s) = %q, %v", start, got, err)
		}
	}
	if _, err := flake.FindRepo(filepath.Join(t.TempDir(), "nowhere")); err == nil {
		t.Error("no flake above: must be an error")
	}
	onlyNix := testutil.Tree(t, map[string]string{"flake.nix": "{}"})
	if _, err := flake.FindRepo(onlyNix); err == nil {
		t.Error("flake.nix without flake.lock is not enough")
	}
}

func TestHosts(t *testing.T) {
	f := (&run.Fake{}).On(run.Rule{Prefix: "nix eval --json /tmp/x#nixosConfigurations", Out: `["desktop","zenbook"]`})
	got, err := flake.Hosts(f, "/tmp/x")
	if err != nil || !reflect.DeepEqual(got, []string{"desktop", "zenbook"}) {
		t.Errorf("hosts = %v, %v", got, err)
	}
	bad := (&run.Fake{}).On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"), Err: "error: flake has no nixosConfigurations\n"})
	if _, err := flake.Hosts(bad, "/tmp/x"); err == nil || !strings.Contains(err.Error(), "no nixosConfigurations") {
		t.Errorf("err = %v", err)
	}
	junk := (&run.Fake{}).On(run.Rule{Prefix: "nix eval", Out: "<<>>"})
	if _, err := flake.Hosts(junk, "/tmp/x"); err == nil {
		t.Error("unparseable output is an error")
	}
}

func TestDirtyAndRestore(t *testing.T) {
	clean := &run.Fake{}
	if flake.Dirty(clean, "/r", "flake.lock") {
		t.Error("git diff --quiet succeeding means clean")
	}
	dirty := (&run.Fake{}).On(run.Rule{Prefix: "git diff --quiet", Fail: errors.New("exit 1")})
	if !flake.Dirty(dirty, "/r", "flake.lock") {
		t.Error("git diff --quiet failing means dirty")
	}
	f := &run.Fake{}
	if err := flake.Restore(f, "/r", "packages/x"); err != nil {
		t.Fatal(err)
	}
	if f.Count("git checkout -- packages/x") != 1 {
		t.Errorf("calls: %v", f.Calls())
	}
	bad := (&run.Fake{}).On(run.Rule{Prefix: "git checkout", Fail: errors.New("x"), Err: "pathspec did not match\n"})
	if err := flake.Restore(bad, "/r", "nope"); err == nil || !strings.Contains(err.Error(), "pathspec") {
		t.Errorf("err = %v", err)
	}
}

const pkgJSON = `{
 "padpoll": {"version":"1.1.0","urls":["https://github.com/PabloStarOk/padpoll/releases/download/v1.1.0/x.tar.gz"],"homepage":"","update":["/nix/store/x/bin/nix-update"]},
 "flectar-mail": {"version":"0.1.0-alpha.7","urls":[],"homepage":"https://github.com/flectar/mail","update":["/nix/store/x/bin/nix-update","--version=unstable","--use-github-releases"]},
 "tunnel": {"version":"1","urls":[],"homepage":"https://example.org","update":[],"pinned":"on purpose"}
}`

func TestPackages(t *testing.T) {
	f := (&run.Fake{}).On(run.Rule{Prefix: "nix eval --json .#packages.", Out: pkgJSON})
	pkgs, err := flake.Packages(f, "/repo")
	if err != nil || len(pkgs) != 3 {
		t.Fatalf("pkgs=%v err=%v", pkgs, err)
	}
	if pkgs[0].Name != "flectar-mail" || pkgs[2].Name != "tunnel" {
		t.Errorf("must be sorted by name: %v", pkgs)
	}
	if f.Calls()[0].Dir != "/repo" {
		t.Error("must evaluate from the repo")
	}
	if apply := f.Calls()[0].Args[len(f.Calls()[0].Args)-1]; !strings.Contains(apply, "position = p.meta.position") {
		t.Errorf("pkg needs meta.position to know which directory to guard and restore: %s", apply)
	}
}

func TestPackagesNoOutputIsNotAnError(t *testing.T) {
	f := (&run.Fake{}).On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"),
		Err: "error: flake 'git+file:///r' does not provide attribute 'packages.x86_64-linux'\n"})
	pkgs, err := flake.Packages(f, "/repo")
	if err != nil || pkgs != nil {
		t.Errorf("pkgs=%v err=%v", pkgs, err)
	}
	broken := (&run.Fake{}).On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"), Err: "error: syntax error\n"})
	if _, err := flake.Packages(broken, "/repo"); err == nil || !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("a real eval error must surface: %v", err)
	}
}

func TestPackageUpstreamAndArgs(t *testing.T) {
	f := (&run.Fake{}).On(run.Rule{Prefix: "nix eval", Out: pkgJSON})
	pkgs, _ := flake.Packages(f, "/repo")
	byName := map[string]flake.Package{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if got := byName["padpoll"].Upstream(); got != "PabloStarOk/padpoll" {
		t.Errorf("from src url: %q", got)
	}
	if got := byName["flectar-mail"].Upstream(); got != "flectar/mail" {
		t.Errorf("from homepage: %q", got)
	}
	if got := byName["tunnel"].Upstream(); got != "" {
		t.Errorf("no github url: %q", got)
	}
	if byName["tunnel"].Pinned != "on purpose" || byName["padpoll"].Pinned != "" {
		t.Errorf("pinned is read from the evaluation: %+v", byName["tunnel"])
	}
	for _, update := range [][]string{
		{"nix-update", "--url", "https://github.com/herdrdev/herdr-app"},
		{"nix-update", "--url=https://github.com/herdrdev/herdr-app"},
	} {
		hosted := flake.Package{URLs: []string{"https://releases.herdr.dev/app/1/x.tar.gz"}, Homepage: "https://github.com/o/other", Update: update}
		if got := hosted.Upstream(); got != "herdrdev/herdr-app" {
			t.Errorf("--url wins over the source and the homepage, as in nix-update: %v -> %q", update, got)
		}
		if args, _ := hosted.ExtraArgs(); len(args) == 0 || !strings.Contains(strings.Join(args, " "), "https://github.com/herdrdev/herdr-app") {
			t.Errorf("--url is passed on to nix-update: %v", args)
		}
	}
	args, pre := byName["flectar-mail"].ExtraArgs()
	if !pre || !reflect.DeepEqual(args, []string{"--use-github-releases"}) {
		t.Errorf("args=%v pre=%v: --version=unstable only allows prereleases", args, pre)
	}
	if args, pre := byName["padpoll"].ExtraArgs(); pre || len(args) != 0 {
		t.Errorf("args=%v pre=%v", args, pre)
	}
	if args, _ := (flake.Package{Update: []string{"nix-update", "--version=skip", "--build"}}).ExtraArgs(); !reflect.DeepEqual(args, []string{"--build"}) {
		t.Errorf("--version=* must never be forwarded: %v", args)
	}
}

func TestSelect(t *testing.T) {
	all := []flake.Package{{Name: "padpoll"}, {Name: "strata"}, {Name: "wealthfolio"}}
	got, err := flake.Select(all, []string{"wealthfolio", "padpoll"})
	if err != nil || len(got) != 2 || got[0].Name != "wealthfolio" {
		t.Errorf("got %v, %v: only what was asked, in that order", got, err)
	}
	if got, _ := flake.Select(all, nil); len(got) != 3 {
		t.Error("no names means all packages")
	}
	if _, err := flake.Select(all, []string{"padpoll", "nope"}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v", err)
	}
}

func TestScratchCopyWithFakeListing(t *testing.T) {
	repo := testutil.Tree(t, map[string]string{"flake.nix": "{}", "modules/a.nix": "a", "gone.nix": "x"})
	if err := os.Chmod(filepath.Join(repo, "modules/a.nix"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("flake.nix", filepath.Join(repo, "link")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(repo, "gone.nix")) // listed by git but deleted in the working tree
	listing := "flake.nix\x00modules/a.nix\x00gone.nix\x00link\x00"
	f := (&run.Fake{}).On(run.Rule{Prefix: "git ls-files -z", Out: listing})

	dst := t.TempDir()
	if err := flake.ScratchCopy(f, repo, dst); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"flake.nix", "modules/a.nix"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Errorf("%s not copied", rel)
		}
	}
	for _, rel := range []string{"gone.nix", "link"} {
		if _, err := os.Lstat(filepath.Join(dst, rel)); err == nil {
			t.Errorf("%s must be skipped (deleted file / symlink)", rel)
		}
	}
	if st, _ := os.Stat(filepath.Join(dst, "modules/a.nix")); st.Mode().Perm()&0o100 == 0 {
		t.Error("the executable bit must survive")
	}
}

func TestScratchCopyListingError(t *testing.T) {
	f := (&run.Fake{}).On(run.Rule{Prefix: "git ls-files", Fail: errors.New("x"), Err: "fatal: not a git repository\n"})
	if err := flake.ScratchCopy(f, t.TempDir(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("err = %v", err)
	}
}

// With real git: ignored files stay out, uncommitted and untracked ones go in.
func TestScratchCopyAndDirtyWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := testutil.Tree(t, map[string]string{
		"flake.nix": "{}", "flake.lock": "{}", ".gitignore": "result\n", "result": "ignored", "tracked.nix": "v1",
	})
	r := run.Exec{}
	git := func(args ...string) {
		t.Helper()
		if _, errs, err := r.Capture(repo, "git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...); err != nil {
			t.Fatalf("git %v: %s", args, errs)
		}
	}
	git("init", "-q")
	git("add", "flake.nix", "flake.lock", ".gitignore", "tracked.nix")
	git("commit", "-qm", "init")

	if flake.Dirty(r, repo, "flake.lock") {
		t.Error("fresh commit is clean")
	}
	os.WriteFile(filepath.Join(repo, "tracked.nix"), []byte("v2 uncommitted"), 0o644)
	os.WriteFile(filepath.Join(repo, "new.nix"), []byte("untracked"), 0o644)
	if !flake.Dirty(r, repo, "tracked.nix") || flake.Dirty(r, repo, "flake.lock") {
		t.Error("Dirty must look only at the given paths")
	}

	dst := t.TempDir()
	if err := flake.ScratchCopy(r, repo, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "tracked.nix")); string(b) != "v2 uncommitted" {
		t.Errorf("the copy must contain uncommitted edits, got %q", b)
	}
	if _, err := os.Stat(filepath.Join(dst, "new.nix")); err != nil {
		t.Error("untracked files are part of the build")
	}
	if _, err := os.Stat(filepath.Join(dst, "result")); err == nil {
		t.Error("ignored files must not be copied")
	}

	if err := flake.Restore(r, repo, "tracked.nix"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "tracked.nix")); string(b) != "v1" {
		t.Errorf("Restore must bring back the committed content, got %q", b)
	}
}

func env(kv ...string) func(string) string {
	return func(k string) string {
		for i := 0; i < len(kv); i += 2 {
			if kv[i] == k {
				return kv[i+1]
			}
		}
		return ""
	}
}

func flakeDir(t *testing.T) string {
	t.Helper()
	return testutil.Tree(t, map[string]string{"flake.nix": "{}", "flake.lock": "{}"})
}

func TestResolvePrecedence(t *testing.T) {
	t.Chdir(t.TempDir()) // outside any flake: the variables are used
	flagDir, nixup, nhOS, nh := flakeDir(t), flakeDir(t), flakeDir(t), flakeDir(t)
	cases := []struct {
		name       string
		flag       string
		env        func(string) string
		want, from string
	}{
		{"flag beats everything", flagDir, env("NIXUP_FLAKE", nixup, "NH_FLAKE", nh), flagDir, "-C"},
		{"NIXUP_FLAKE beats nh's", "", env("NIXUP_FLAKE", nixup, "NH_OS_FLAKE", nhOS, "NH_FLAKE", nh), nixup, "$NIXUP_FLAKE"},
		{"NH_OS_FLAKE beats NH_FLAKE", "", env("NH_OS_FLAKE", nhOS, "NH_FLAKE", nh), nhOS, "$NH_OS_FLAKE"},
		{"NH_FLAKE is reused", "", env("NH_FLAKE", nh), nh, "$NH_FLAKE"},
		{"nh's host suffix is dropped", "", env("NH_FLAKE", nh+"#desktop"), nh, "$NH_FLAKE"},
		{"path: prefix is accepted", "", env("NIXUP_FLAKE", "path:"+nixup), nixup, "$NIXUP_FLAKE"},
	}
	for _, c := range cases {
		got, from, err := flake.Resolve(c.flag, c.env)
		if err != nil || got != c.want || from != c.from {
			t.Errorf("%s: got %q from %q, err %v; want %q from %q", c.name, got, from, err, c.want, c.from)
		}
	}
}

func TestResolveTheFlakeYouAreInBeatsVariables(t *testing.T) {
	here, configured := flakeDir(t), flakeDir(t)
	sub := filepath.Join(here, "modules")
	os.MkdirAll(sub, 0o755)
	t.Chdir(sub)
	got, from, err := flake.Resolve("", env("NIXUP_FLAKE", configured, "NH_FLAKE", "github:not/even/valid"))
	if want, _ := filepath.EvalSymlinks(here); err != nil || from != "" || (got != here && got != want) {
		t.Errorf("got %q from %q, %v; want %q: inside a flake, that flake (variables are not even read)", got, from, err, here)
	}
	if got, _, _ := flake.Resolve(configured, env("NIXUP_FLAKE", configured)); got != configured {
		t.Errorf("-C still beats the flake you are in: %q", got)
	}
}

func TestResolveTildeAndRelative(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "cfg"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(home, "cfg", "flake.nix"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(home, "cfg", "flake.lock"), []byte("{}"), 0o644)
	got, _, err := flake.Resolve("", env("NIXUP_FLAKE", "~/cfg#desktop"))
	if err != nil || got != filepath.Join(home, "cfg") {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestResolveFallsBackToSearchingUpwards(t *testing.T) {
	root := flakeDir(t)
	sub := filepath.Join(root, "modules")
	os.MkdirAll(sub, 0o755)
	t.Chdir(sub)
	got, from, err := flake.Resolve("", env())
	if err != nil || from != "" {
		t.Fatalf("got %q from %q, %v", got, from, err)
	}
	if want, _ := filepath.EvalSymlinks(root); got != want && got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestResolveErrors(t *testing.T) {
	// A configured path that is wrong is an error, never silently replaced by the current directory
	// or by a parent: the user asked for that exact flake.
	inside := flakeDir(t)
	child := filepath.Join(inside, "sub")
	os.MkdirAll(child, 0o755)
	t.Chdir(t.TempDir()) // outside any flake, so the variables are read
	for name, e := range map[string]func(string) string{
		"parent flake is not used": env("NIXUP_FLAKE", child),
		"missing directory":        env("NIXUP_FLAKE", filepath.Join(inside, "nope")),
		"github reference":         env("NH_FLAKE", "github:Villoh/nixos-config"),
		"git url":                  env("NH_FLAKE", "git+https://example.org/x.git"),
	} {
		if _, _, err := flake.Resolve("", e); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	_, _, err := flake.Resolve("", env("NH_FLAKE", "github:Villoh/nixos-config"))
	if err == nil || !strings.Contains(err.Error(), "$NH_FLAKE") || !strings.Contains(err.Error(), "not a local directory") {
		t.Errorf("the error must name the variable and the problem: %v", err)
	}

	_, _, err = flake.Resolve("", env())
	if err == nil || !strings.Contains(err.Error(), "NIXUP_FLAKE") {
		t.Errorf("with nothing configured, the error must say how to configure it: %v", err)
	}
}

func TestUsesReleases(t *testing.T) {
	yes := flake.Package{Update: []string{"nix-update", "--use-github-releases"}}
	no := flake.Package{Update: []string{"nix-update", "--version=unstable"}}
	if !yes.UsesReleases() || no.UsesReleases() || (flake.Package{}).UsesReleases() {
		t.Error("UsesReleases must look for --use-github-releases only")
	}
	// It is also forwarded to nix-update, which needs it to resolve the same version.
	if args, _ := yes.ExtraArgs(); !reflect.DeepEqual(args, []string{"--use-github-releases"}) {
		t.Errorf("args = %v", args)
	}
}

func TestInNixpkgs(t *testing.T) {
	f := &run.Fake{}
	f.On(run.Rule{Prefix: "nix eval --json --inputs-from . nixpkgs#legacyPackages.", Out: `{
		"wealthfolio": {"version":"3.8.0","urls":[],"homepage":"https://wealthfolio.app/"},
		"handy": {"version":"0.9.6","urls":["https://github.com/cjpais/Handy/archive/v0.9.6.tar.gz"],"homepage":"https://handy.computer"}}`})
	pkgs := []flake.Package{{Name: "wealthfolio"}, {Name: "handy-bin"}, {Name: "bad name"}, {Name: "nothere"}}
	found, err := flake.InNixpkgs(f, "/repo", pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if w := found["wealthfolio"]; len(w) != 1 || w[0].Name != "wealthfolio" || w[0].Version != "3.8.0" {
		t.Errorf("wealthfolio: %+v", w)
	}
	if h := found["handy-bin"]; len(h) != 1 || h[0].Name != "handy" || h[0].Upstream() != "cjpais/Handy" {
		t.Errorf("foo-bin is also looked up as foo: %+v", h)
	}
	if _, ok := found["nothere"]; ok {
		t.Errorf("absent package: %+v", found)
	}
	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("one evaluation for all packages: %v", calls)
	}
	apply := calls[0].Args[len(calls[0].Args)-1]
	for _, want := range []string{`"wealthfolio"`, `"handy-bin"`, `"handy"`, `"nothere"`} {
		if !strings.Contains(apply, want) {
			t.Errorf("%s not looked up: %s", want, apply)
		}
	}
	if strings.Contains(apply, "bad name") {
		t.Errorf("names that are not attribute names are skipped: %s", apply)
	}

	f2 := &run.Fake{}
	f2.On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"), Err: "error: no nixpkgs\n"})
	if _, err := flake.InNixpkgs(f2, "/repo", pkgs); err == nil || !strings.Contains(err.Error(), "no nixpkgs") {
		t.Errorf("eval failure: %v", err)
	}
}

func TestSameProject(t *testing.T) {
	gh := func(repo string) flake.Package {
		return flake.Package{URLs: []string{"https://github.com/" + repo + "/archive/v1.tar.gz"}}
	}
	for _, c := range []struct {
		a, b flake.Package
		want bool
	}{
		{gh("cjpais/Handy"), gh("cjpais/handy"), true},   // same repo, any case
		{gh("anomalyco/hex"), gh("sitkevij/hex"), false}, // a namesake
		{flake.Package{Homepage: "https://wealthfolio.app"}, flake.Package{Homepage: "http://www.wealthfolio.app/"}, true},
		{flake.Package{Homepage: "https://a.org"}, flake.Package{Homepage: "https://b.org"}, false},
		{flake.Package{}, flake.Package{}, false}, // no evidence is not a match
	} {
		if got := flake.SameProject(c.a, c.b); got != c.want {
			t.Errorf("SameProject(%+v, %+v) = %v", c.a, c.b, got)
		}
	}
}

func TestTagVersion(t *testing.T) {
	for _, c := range []struct {
		update []string
		tag    string
		want   string
		ok     bool
	}{
		{[]string{"nix-update", "--version-regex", "desktop-v(.*)"}, "desktop-v0.6.1", "0.6.1", true},
		{[]string{"nix-update", "--version-regex", "desktop-v(.*)"}, "desktop-minimal-v0.6.1", "", false},
		{[]string{"nix-update", "--version-regex", "desktop-v(.*)"}, "x-desktop-v0.6.1", "", false}, // anchored at the start, like re.match
		{[]string{"nix-update", "--version-regex=v(\\d+)_(\\d+)"}, "v1_2", "1.2", true},             // groups joined with "."
		{[]string{"nix-update", "--version-regex", "(\\d+)-?(rc\\d+)?"}, "3", "3", true},            // empty groups are dropped
	} {
		p := flake.Package{Name: "x", Update: c.update}
		extract, err := p.TagVersion()
		if err != nil || extract == nil {
			t.Fatalf("%v: %v", c.update, err)
		}
		if got, ok := extract(c.tag); got != c.want || ok != c.ok {
			t.Errorf("%v on %q = %q, %v; want %q, %v", c.update[1:], c.tag, got, ok, c.want, c.ok)
		}
	}
	if e, err := (flake.Package{Update: []string{"nix-update"}}).TagVersion(); e != nil || err != nil {
		t.Errorf("without --version-regex the default rules apply: %v", err)
	}
	if _, err := (flake.Package{Name: "x", Update: []string{"nix-update", "--version-regex", "("}}).TagVersion(); err == nil {
		t.Error("a broken regex is an error")
	}
}

func TestPackagePath(t *testing.T) {
	repo := testutil.Tree(t, map[string]string{"flake.nix": "{}", "default.nix": "{}",
		"packages/prebuilt/kopuz/package.nix": "{}", "packages/meiro/package.nix": "{}"})
	// A file that exists next to the repository, not in it: never a path to restore.
	os.WriteFile(filepath.Join(filepath.Dir(repo), "escape.nix"), []byte("{}"), 0o644)
	for pos, want := range map[string]string{
		"/nix/store/abc-source/packages/prebuilt/kopuz/package.nix:83": "packages/prebuilt/kopuz",
		"/nix/store/abc-source/packages/meiro/package.nix:7":           "packages/meiro",
		"/nix/store/abc-source/flake.nix:40":                           "flake.nix", // top level: the file, never the whole repo
		"/nix/store/abc-source/default.nix:2":                          "default.nix",
		"/nix/store/abc-source/pkgs/missing/package.nix:1":             "", // not in this flake
		"/nix/store/abc-source/../escape.nix:1":                        "",
		"/etc/nixos/configuration.nix:1":                               "",
		"":                                                             "",
	} {
		if got := (flake.Package{Position: pos}).Path(repo); got != want {
			t.Errorf("Path(%q) = %q, want %q", pos, got, want)
		}
	}
}
