package check_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/testutil"
)

func checker(e *testutil.Env) *check.Checker { return &check.Checker{R: e.Run, GH: e.GH} }

func repoWithLock(t *testing.T, inputs map[string]string) string {
	t.Helper()
	return testutil.Tree(t, map[string]string{"flake.nix": "{}", "flake.lock": testutil.LockJSON(inputs)})
}

func byName(rows []check.Row) map[string]check.Row {
	m := map[string]check.Row{}
	for _, r := range rows {
		m[r.Name] = r
	}
	return m
}

func TestInputsPinnedTag(t *testing.T) {
	e := testutil.New(t)
	e.Tags("AvengeMedia/DankMaterialShell", "v1.6.2", "v1.6.3", "v2.0.0", "v2.1.0-rc.1")
	e.Tags("nix-community/lanzaboote", "v1.2.0", "v1.1.0")
	repo := repoWithLock(t, map[string]string{
		"dms":        "AvengeMedia/DankMaterialShell@v1.6.2#aaa",
		"lanzaboote": "nix-community/lanzaboote@v1.2.0#bbb",
	})
	rows := byName(checker(e).Inputs(repo, false))

	dms := rows["dms"]
	if dms.Status != check.Outdated || dms.Latest != "v2.0.0" || dms.Note != "nixup update dms" {
		t.Errorf("dms: %+v (prereleases must not count)", dms)
	}
	if !strings.Contains(dms.Warn, "MAJOR") || dms.Review != nil {
		t.Errorf("a major jump warns; no review unless asked: %+v", dms)
	}
	if lz := rows["lanzaboote"]; lz.Status != check.OK || lz.Note != "pinned tag" {
		t.Errorf("lanzaboote is current: %+v", lz)
	}
}

func TestInputsReviewForPinnedTag(t *testing.T) {
	e := testutil.New(t)
	e.Tags("o/dms", "v1.0.0", "v2.0.0")
	e.Compare("o/dms", "v1.0.0", "v2.0.0", 3, "feat: x")
	e.Releases("o/dms", testutil.Release{Tag: "v2.0.0", Body: "breaking things"})
	repo := repoWithLock(t, map[string]string{"dms": "o/dms@v1.0.0#aaa"})

	row := byName(checker(e).Inputs(repo, true))["dms"]
	if row.Review == nil || row.Review.Commits != 3 || len(row.Review.Releases) != 1 {
		t.Errorf("review = %+v", row.Review)
	}
}

func TestInputsBranch(t *testing.T) {
	e := testutil.New(t)
	e.Head("NixOS/nixpkgs", "", "bbb2222")
	e.Head("o/fork", "integration", "same111")
	e.Compare("NixOS/nixpkgs", "aaa1111", "bbb2222", 12, "fix: y")
	repo := repoWithLock(t, map[string]string{
		"nixpkgs": "NixOS/nixpkgs@#aaa1111",
		"fork":    "o/fork@integration#same111",
	})
	rows := byName(checker(e).Inputs(repo, true))

	np := rows["nixpkgs"]
	if np.Status != check.Outdated || np.Latest != "bbb2222" || np.Note != "nixup update nixpkgs" {
		t.Errorf("nixpkgs: %+v", np)
	}
	if np.Review == nil || np.Review.Commits != 12 {
		t.Errorf("branch inputs are reviewed by commit range: %+v", np.Review)
	}
	if rows["fork"].Status != check.OK {
		t.Errorf("fork at the branch head is current: %+v", rows["fork"])
	}
}

func TestInputsBranchNamedLikeAVersionLookalikeIsNotATag(t *testing.T) {
	e := testutil.New(t)
	e.Head("NixOS/nixpkgs", "release-25.11", "aaa1111")
	repo := repoWithLock(t, map[string]string{"stable": "NixOS/nixpkgs@release-25.11#aaa1111"})
	if row := checker(e).Inputs(repo, false)[0]; row.Status != check.OK || row.Note == "pinned tag" {
		t.Errorf("a release branch is followed like a branch: %+v", row)
	}
}

func TestInputsSkipsNonGithub(t *testing.T) {
	e := testutil.New(t)
	repo := testutil.Tree(t, map[string]string{"flake.nix": "{}", "flake.lock": `{"root":"r","nodes":{
		"r":{"inputs":{"priv":"priv"}},
		"priv":{"locked":{"rev":"x"},"original":{"type":"git","url":"ssh://git@example.org/x.git"}}}}`})
	if rows := checker(e).Inputs(repo, false); len(rows) != 0 {
		t.Errorf("only github inputs can be checked: %+v", rows)
	}
}

func TestInputsErrors(t *testing.T) {
	e := testutil.New(t)
	rows := checker(e).Inputs(t.TempDir(), false)
	if len(rows) != 1 || rows[0].Status != check.Error || rows[0].Name != "flake.lock" {
		t.Errorf("missing lock: %+v", rows)
	}

	e.Run.On(run.Rule{Prefix: "git ls-remote", Fail: errors.New("x"), Err: "fatal: unable to access\n"})
	repo := repoWithLock(t, map[string]string{"dms": "o/dms@v1.0.0#aaa", "np": "o/np@#bbb"})
	for _, r := range checker(e).Inputs(repo, false) {
		if r.Status != check.Error || !strings.Contains(r.Note, "unable to access") {
			t.Errorf("a network failure is a row, not a crash: %+v", r)
		}
	}
}

const pkgsJSON = `{
 "padpoll": {"version":"1.0.0","urls":["https://github.com/PabloStarOk/padpoll/releases/download/v1.0.0/x"],"homepage":"","update":["nix-update"]},
 "flectar": {"version":"0.1.0-alpha.7","urls":[],"homepage":"https://github.com/flectar/mail","update":["nix-update","--version=unstable"]},
 "current": {"version":"2.0.0","urls":["https://github.com/o/current/archive/v2.0.0.tar.gz"],"homepage":"","update":["nix-update"]},
 "nogithub": {"version":"1","urls":[],"homepage":"https://example.org","update":["nix-update"]},
 "noscript": {"version":"1","urls":["https://github.com/o/noscript/x"],"homepage":"","update":[]},
 "font": {"version":"0-unstable-1","urls":["https://example.org/f.ttf"],"homepage":"","update":[],"pinned":"changes with the plugin"}
}`

func TestPackages(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: pkgsJSON})
	e.Tags("PabloStarOk/padpoll", "v1.0.0", "v1.1.0", "v2.0.0-rc.1")
	e.Tags("flectar/mail", "v0.1.0-alpha.6", "v0.1.0-alpha.7", "v0.1.0-alpha.8")
	e.Tags("o/current", "v2.0.0", "v1.0.0")
	e.Compare("PabloStarOk/padpoll", "v1.0.0", "v1.1.0", 5, "build: bump")
	e.Releases("PabloStarOk/padpoll", testutil.Release{Tag: "v1.1.0", Body: "notes"})

	rows := byName(checker(e).Packages("/repo", true))
	if len(rows) != 6 {
		t.Fatalf("rows: %+v", rows)
	}
	p := rows["padpoll"]
	if p.Status != check.Outdated || p.Latest != "1.1.0" || p.Note != "nixup update padpoll" || p.Warn != "" {
		t.Errorf("padpoll: %+v", p)
	}
	if p.Review == nil || p.Review.Commits != 5 {
		t.Errorf("padpoll review: %+v", p.Review)
	}
	if f := rows["flectar"]; f.Status != check.Outdated || f.Latest != "0.1.0-alpha.8" {
		t.Errorf("--version=unstable lets prereleases through: %+v", f)
	}
	if rows["current"].Status != check.OK || rows["current"].Latest != "" {
		t.Errorf("current: %+v", rows["current"])
	}
	if r := rows["nogithub"]; r.Status != check.Info || !strings.Contains(r.Note, "no GitHub upstream") {
		t.Errorf("nogithub: %+v", r)
	}
	if r := rows["noscript"]; r.Status != check.Info || !strings.Contains(r.Note, "updateScript") {
		t.Errorf("noscript: %+v", r)
	}
	if r := rows["font"]; r.Status != check.OK || r.Note != "pinned: changes with the plugin" {
		t.Errorf("passthru.nixup.pinned: ok, with the reason, even without upstream or script: %+v", r)
	}
}

func TestPackageMajorJumpWarns(t *testing.T) {
	e := testutil.New(t)
	e.Tags("lgse/strata", "v0.21.0", "v1.1.1")
	p := flake.Package{Name: "strata", Version: "0.21.0", Homepage: "https://github.com/lgse/strata", Update: []string{"nix-update"}}
	row := checker(e).Package(p, false)
	if row.Status != check.Outdated || !strings.Contains(row.Warn, "MAJOR") {
		t.Errorf("0.21 -> 1.1 must warn: %+v", row)
	}
}

func TestPackageStablePackageIgnoresPrereleaseTags(t *testing.T) {
	e := testutil.New(t)
	e.Tags("o/r", "v1.0.0", "v1.1.0-rc.1")
	p := flake.Package{Name: "x", Version: "1.0.0", Homepage: "https://github.com/o/r", Update: []string{"nix-update"}}
	if row := checker(e).Package(p, false); row.Status != check.OK {
		t.Errorf("an rc is not an update: %+v", row)
	}
}

func TestPackageTagLookupFailureIsARow(t *testing.T) {
	e := testutil.New(t)
	e.Tags("o/r") // repo without tags
	p := flake.Package{Name: "x", Version: "1.0.0", Homepage: "https://github.com/o/r", Update: []string{"nix-update"}}
	if row := checker(e).Package(p, false); row.Status != check.Error {
		t.Errorf("%+v", row)
	}
}

func TestPackagesEvalFailure(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"), Err: "error: boom\n"})
	rows := checker(e).Packages("/repo", false)
	if len(rows) != 1 || rows[0].Status != check.Error || !strings.Contains(rows[0].Note, "boom") {
		t.Errorf("%+v", rows)
	}
}

func TestProfile(t *testing.T) {
	sys, old := strings.Repeat("e", 40), strings.Repeat("a", 40)
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "nix profile list --json", Out: `{"elements":{
		"qtdeclarative":{"originalUrl":"github:NixOS/nixpkgs/` + old + `"},
		"hello":{"originalUrl":"github:NixOS/nixpkgs/` + sys + `"},
		"tracked":{"originalUrl":"flake:nixpkgs"},
		"other":{"originalUrl":"github:o/tool/` + old + `"}}}`})
	repo := repoWithLock(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#" + sys})

	rows := byName(checker(e).Profile(repo))
	if q := rows["qtdeclarative"]; q.Status != check.Outdated || q.Current != "aaaaaaa" || q.Latest != "eeeeeee" ||
		q.Note != "pinned: nix profile remove, then install again" {
		t.Errorf("pinned to an old nixpkgs, and what to do: %+v", q)
	}
	if rows["other"].Status != check.OK || rows["other"].Current != "aaaaaaa" {
		t.Errorf("a revision of another repository is not compared with nixpkgs: %+v", rows["other"])
	}
	if rows["hello"].Status != check.OK || rows["tracked"].Status != check.OK || rows["tracked"].Note != "nixup update tracked" {
		t.Errorf("rows: %+v", rows)
	}
}

func TestProfileUnavailable(t *testing.T) {
	e := testutil.New(t)
	e.Run.Missing["nix"] = true
	if rows := checker(e).Profile("/repo"); rows != nil {
		t.Errorf("without nix there is nothing to check: %+v", rows)
	}

	e2 := testutil.New(t)
	e2.Run.On(run.Rule{Prefix: "nix profile", Fail: errors.New("x"), Err: "error: not supported\n"})
	if rows := checker(e2).Profile("/repo"); len(rows) != 1 || rows[0].Status != check.Error {
		t.Errorf("%+v", rows)
	}
	e3 := testutil.New(t)
	e3.Run.On(run.Rule{Prefix: "nix profile", Out: "garbage"})
	if rows := checker(e3).Profile("/repo"); len(rows) != 1 || rows[0].Status != check.Error {
		t.Errorf("unreadable output is an error row, not silence: %+v", rows)
	}
}

func TestFlatpak(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates --columns=ref,version", Out: "runtime/org.gnome.Platform/x86_64/50\t\n\n" +
		"runtime/org.gnome.Platform/x86_64/50\t\n" + // listed twice: shown once
		" app/co.hyprlab.Hylki/x86_64/stable \t\n" + // remote publishes no version
		"app/io.x.New/x86_64/stable\t1.42.0\n" +
		"app/io.x.Same/x86_64/stable\t2.0\n" +
		"app/io.x.Two/x86_64/beta\t3.1\n"})
	e.Run.On(run.Rule{Prefix: "flatpak list --columns=ref,version", Out: "org.gnome.Platform/x86_64/50\t\n" +
		"co.hyprlab.Hylki/x86_64/stable\t1.41.1\nio.x.New/x86_64/stable\t1.41.1\nio.x.Same/x86_64/stable\t2.0\n" +
		"io.x.Two/x86_64/stable\t3.0\nio.x.Two/x86_64/beta\t3.1-rc\n"})
	rows := checker(e).Flatpak()
	got := map[string][2]string{}
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
		got[r.Name] = [2]string{r.Current, r.Latest}
	}
	if strings.Join(names, ",") != "org.gnome.Platform,co.hyprlab.Hylki,io.x.New,io.x.Same,io.x.Two" {
		t.Fatalf("one row per ref, in order, blanks and spaces dropped: %v", names)
	}
	for _, r := range rows {
		if r.Note != "nixup update "+r.Name {
			t.Errorf("ACTION points at nixup update: %+v", r)
		}
	}
	for name, want := range map[string][2]string{
		"org.gnome.Platform": {"50", "new build"},   // a runtime: its branch, rebuilt
		"co.hyprlab.Hylki":   {"1.41.1", "unknown"}, // the remote does not say
		"io.x.New":           {"1.41.1", "1.42.0"},  // both versions known
		"io.x.Same":          {"2.0", "new build"},  // same version offered again
		"io.x.Two":           {"3.1-rc", "3.1"},     // the installed version of that branch
	} {
		if got[name] != want {
			t.Errorf("%s: got %v, want %v", name, got[name], want)
		}
	}

	e.Run.Missing["flatpak"] = true
	if rows := checker(e).Flatpak(); rows != nil {
		t.Error("no flatpak, no section")
	}

	e2 := testutil.New(t)
	e2.Run.On(run.Rule{Prefix: "flatpak", Fail: errors.New("x"), Err: "error: no remotes\n"})
	if rows := checker(e2).Flatpak(); len(rows) != 1 || rows[0].Status != check.Error {
		t.Errorf("%+v", rows)
	}
}

func TestAllRunsOnlyRequestedSections(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "flatpak remote-ls", Out: "runtime/org.gnome.Platform/x86_64/50\t\n"})
	rows := checker(e).All("/repo", []string{"flatpak"}, false)
	if len(rows) != 1 || rows[0].Section != "flatpak" {
		t.Errorf("rows: %+v", rows)
	}
	if e.Run.Count("nix") != 0 || e.Run.Count("git") != 0 {
		t.Errorf("other sections must not run: %v", e.Run.Calls())
	}

	e2 := testutil.New(t)
	repo := repoWithLock(t, map[string]string{"np": "o/np@#aaa"})
	e2.Head("o/np", "", "aaa")
	e2.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: "{}"})
	rows = checker(e2).All(repo, nil, false)
	if len(rows) == 1 && (rows[0].Status != check.OK || rows[0].Latest != rows[0].Current) {
		t.Errorf("an up-to-date row shows its version as the latest: %+v", rows[0])
	}
	if len(rows) != 1 || rows[0].Name != "np" {
		t.Errorf("no filter runs every section: %+v", rows)
	}
}

func TestExitCode(t *testing.T) {
	ok, out, bad := check.Row{Status: check.OK}, check.Row{Status: check.Outdated}, check.Row{Status: check.Error}
	cases := []struct {
		rows []check.Row
		want int
	}{
		{nil, exit.OK},
		{[]check.Row{ok, {Status: check.Info}}, exit.OK},
		{[]check.Row{ok, out}, exit.Pending},
		{[]check.Row{out, bad, ok}, exit.Error},
		{[]check.Row{bad, out}, exit.Error},
	}
	for i, c := range cases {
		if got := check.ExitCode(c.rows); got != c.want {
			t.Errorf("case %d: got %d, want %d", i, got, c.want)
		}
	}
}

// The case that broke: a repo whose tags include unrelated series (v97.000, v111111) next to
// its real versions. Tags would say 111111; the published releases say 5.000.
func TestPackageFollowsReleasesWhenTheUpdateScriptDoes(t *testing.T) {
	e := testutil.New(t)
	e.Tags("g/sans", "v4.007", "v5.000", "v97.000", "v123.011", "v111111")
	e.Releases("g/sans",
		testutil.Release{Tag: "v5.000", Body: "now"},
		testutil.Release{Tag: "v4.007", Body: "before"},
	)
	byTags := flake.Package{Name: "sans", Version: "5.000", Homepage: "https://github.com/g/sans", Update: []string{"nix-update"}}
	byReleases := byTags
	byReleases.Update = []string{"nix-update", "--use-github-releases"}

	if row := checker(e).Package(byTags, false); row.Status != check.Outdated || row.Latest != "111111" {
		t.Errorf("without the flag nixup reads tags, as nix-update does: %+v", row)
	}
	if row := checker(e).Package(byReleases, false); row.Status != check.OK {
		t.Errorf("with --use-github-releases the junk tags must be ignored: %+v", row)
	}
}

func TestPackageReleasesDetectsARealUpdate(t *testing.T) {
	e := testutil.New(t)
	e.Tags("g/sans", "v4.007", "v5.000", "v111111")
	e.Releases("g/sans", testutil.Release{Tag: "v5.000"}, testutil.Release{Tag: "v4.007"})
	e.Compare("g/sans", "v4.007", "v5.000", 3, "feat: x")
	p := flake.Package{Name: "sans", Version: "4.007", Homepage: "https://github.com/g/sans", Update: []string{"nix-update", "--use-github-releases"}}
	row := checker(e).Package(p, true)
	if row.Status != check.Outdated || row.Latest != "5.000" || row.Review == nil || row.Review.Commits != 3 {
		t.Errorf("%+v", row)
	}
}

func TestPackageReleasesLookupFailure(t *testing.T) {
	e := testutil.New(t) // no releases route: 404
	p := flake.Package{Name: "x", Version: "1.0.0", Homepage: "https://github.com/o/none", Update: []string{"nix-update", "--use-github-releases"}}
	if row := checker(e).Package(p, false); row.Status != check.Error {
		t.Errorf("%+v", row)
	}
}

func TestPackagesPointsAtNixpkgs(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{
		"caught-up": {"version":"2.0.0","urls":["https://github.com/o/caught/archive/v2.0.0.tar.gz"],"homepage":"","update":["nix-update"]},
		"ahead":     {"version":"3.12.0","urls":[],"homepage":"https://ahead.example","update":[]},
		"hex-bin":   {"version":"3.3.0","urls":["https://github.com/anomalyco/hex/x"],"homepage":"","update":[]},
		"tool-bin":  {"version":"1.0.0","urls":["https://github.com/o/tool/x"],"homepage":"","update":[]}}`})
	e.Run.On(run.Rule{Prefix: "nix eval --json --inputs-from . nixpkgs#", Out: `{
		"caught-up": {"version":"2.1.0","urls":[],"homepage":"https://github.com/o/caught"},
		"ahead":     {"version":"3.8.0","urls":[],"homepage":"https://ahead.example"},
		"hex":       {"version":"9.0.0","urls":["https://github.com/sitkevij/hex/x"],"homepage":""},
		"tool":      {"version":"1.0.0","urls":["https://github.com/o/tool/y"],"homepage":""}}`})
	e.Tags("o/caught", "v2.0.0") // up to date upstream: the row would be "ok" and hidden
	repo := repoWithLock(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#" + strings.Repeat("e", 40)})

	rows := byName(checker(e).Packages(repo, false))
	if r := rows["caught-up"]; !strings.Contains(r.Hint, "nixpkgs has caught-up 2.1.0") || r.Status != check.Info {
		t.Errorf("nixpkgs is newer: hint, shown as info: %+v", r)
	}
	if r := rows["tool-bin"]; !strings.Contains(r.Hint, "pkgs.tool could replace") {
		t.Errorf("same version, found under the name without -bin: %+v", r)
	}
	if r := rows["ahead"]; r.Hint != "" {
		t.Errorf("ours is newer: no hint: %+v", r)
	}
	if r := rows["hex-bin"]; r.Hint != "" {
		t.Errorf("a namesake from another repository is not the same program: %+v", r)
	}
	if code := check.ExitCode([]check.Row{rows["caught-up"]}); code != 0 {
		t.Errorf("a hint never changes the exit code: %d", code)
	}
}

func TestPackagesSkipsNixpkgsLookupWithoutTheInput(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"x":{"version":"1","urls":[],"homepage":"https://x.example","update":[]}}`})
	repo := repoWithLock(t, map[string]string{"other": "o/other@#" + strings.Repeat("a", 40)})
	checker(e).Packages(repo, false)
	if n := e.Run.Count("nix eval --json --inputs-from"); n != 0 {
		t.Errorf("without a nixpkgs input nix would use the global registry: %d lookups", n)
	}
}

func TestPackagesNixpkgsLookupFailureKeepsTheRows(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"x":{"version":"1","urls":[],"homepage":"","update":[]}}`})
	e.Run.On(run.Rule{Prefix: "nix eval --json --inputs-from", Fail: errors.New("x"), Err: "error: boom\n"})
	repo := repoWithLock(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#" + strings.Repeat("e", 40)})
	rows := checker(e).Packages(repo, false)
	if len(rows) != 1 || rows[0].Status != check.Info || rows[0].Hint != "" {
		t.Errorf("%+v", rows)
	}
}

func TestPackageFollowsVersionRegex(t *testing.T) {
	e := testutil.New(t)
	e.Tags("elianiva/meiro", "desktop-minimal-v0.6.1", "desktop-v0.6.1", "desktop-minimal-v0.8.0", "desktop-v0.7.0")
	p := flake.Package{Name: "meiro", Version: "0.6.1", Homepage: "https://github.com/elianiva/meiro",
		Update: []string{"nix-update", "--version-regex", "desktop-v(.*)"}}
	row := checker(e).Package(p, true)
	if row.Status != check.Outdated || row.Latest != "0.7.0" || !strings.Contains(row.Warn, "0.x") {
		t.Errorf("the update script's regex picks the series and reads the version: %+v", row)
	}
	if row.Review == nil || row.Review.From != "desktop-v0.6.1" || row.Review.To != "desktop-v0.7.0" {
		t.Errorf("the review compares the real tags: %+v", row.Review)
	}
}

// versionServer serves body at /version, the way versions.brave.com serves a channel.
func versionServer(t *testing.T, status int, body string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/version"
}

// Brave tags Release, Beta and Nightly builds alike: GitHub would say 1.102.1, the release
// channel says 1.99.56. passthru.nixup.versionUrl makes nixup read the channel.
func TestPackageReadsTheVersionURL(t *testing.T) {
	e := testutil.New(t)
	e.Tags("brave/brave-browser", "v1.99.55", "v1.99.56", "v1.102.1")
	e.Compare("brave/brave-browser", "v1.99.55", "v1.99.56", 4, "Release v1.99.56")
	p := flake.Package{Name: "brave-origin", Version: "1.99.55",
		URLs:       []string{"https://github.com/brave/brave-browser/releases/download/v1.99.55/brave-origin_1.99.55_amd64.deb"},
		Update:     []string{"/nix/store/x-update-brave-origin"},
		VersionURL: versionServer(t, 200, "1.99.56\n")}
	row := checker(e).Package(p, true)
	if row.Status != check.Outdated || row.Latest != "1.99.56" || row.Note != "nixup update brave-origin" {
		t.Fatalf("%+v", row)
	}
	if row.Review == nil || row.Review.Commits != 4 {
		t.Errorf("the review compares the two version tags: %+v", row.Review)
	}

	p.Version = "1.99.56"
	if row := checker(e).Package(p, false); row.Status != check.OK || row.Latest != "" {
		t.Errorf("same version: %+v", row)
	}
	p.Version = "1.99.57"
	if row := checker(e).Package(p, false); row.Status != check.OK {
		t.Errorf("a version newer than the channel is not an update: %+v", row)
	}
}

func TestPackageVersionURLNeedsNoGitHubUpstream(t *testing.T) {
	e := testutil.New(t)
	noUpstream := flake.Package{Name: "y", Version: "1.0.0", VersionURL: versionServer(t, 200, "1.1.0")}
	if row := checker(e).Package(noUpstream, true); row.Status != check.Outdated || row.Review != nil {
		t.Errorf("no GitHub upstream is fine with a versionUrl: %+v", row)
	}
}

func TestPackageVersionURLFailuresAreRows(t *testing.T) {
	e := testutil.New(t)
	for name, url := range map[string]string{
		"404":   versionServer(t, 404, "1.1.0"),
		"html":  versionServer(t, 200, "<html><body>moved</body></html>"),
		"empty": versionServer(t, 200, "  \n"),
	} {
		p := flake.Package{Name: "x", Version: "1.0.0", VersionURL: url}
		if row := checker(e).Package(p, false); row.Status != check.Error {
			t.Errorf("%s: %+v", name, row)
		}
	}
}

func TestPackageWithAScriptOfItsOwnSaysSo(t *testing.T) {
	e := testutil.New(t)
	e.Tags("brave/brave-browser", "v1.99.55", "v1.102.1")
	p := flake.Package{Name: "brave-origin", Version: "1.99.55", Homepage: "https://github.com/brave/brave-browser",
		Update: []string{"/nix/store/x-update-brave-origin"}}
	row := checker(e).Package(p, false)
	if row.Status != check.Info || !strings.Contains(row.Note, "passthru.nixup.versionUrl") {
		t.Errorf("a custom script must not be read as nix-update (it would say 1.102.1): %+v", row)
	}
}

// dms follows nixpkgs' dux-shell: its tag moves to nixpkgs' version, never past it.
const followsNix = `{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    # nixup: follows nixpkgs#dux-shell
    # (a note under the marker is fine)
    dms = {
      url = "github:o/dux/v2.1.1";
    };
    other.url = "github:o/other/v1.0.0";
  };
}`

func followsRepo(t *testing.T, e *testutil.Env, pinned, nixpkgsVersion string) string {
	t.Helper()
	repo := testutil.Tree(t, map[string]string{"flake.nix": followsNix, "flake.lock": testutil.LockJSON(map[string]string{
		"dms": "o/dux@" + pinned + "#aaa", "other": "o/other@v1.0.0#bbb", "nixpkgs": "NixOS/nixpkgs@nixos-unstable#ccc"})})
	e.Head("NixOS/nixpkgs", "nixos-unstable", "ccc")
	e.Tags("o/other", "v1.0.0", "v1.1.0")
	out := `{}`
	if nixpkgsVersion != "" {
		out = `{"dux-shell":{"version":"` + nixpkgsVersion + `","urls":[],"homepage":""}}`
	}
	e.Run.On(run.Rule{Prefix: "nix eval --json --inputs-from . nixpkgs#legacyPackages.", Out: out})
	return repo
}

func TestInputsFollowingNixpkgs(t *testing.T) {
	for _, c := range []struct {
		name, pinned, nixpkgs string
		tags                  []string
		status, latest, note  string
	}{
		{"same as nixpkgs, nothing newer", "v2.1.1", "2.1.1", []string{"v2.1.1"}, check.OK, "v2.1.1", "follows nixpkgs dux-shell 2.1.1"},
		{"upstream ahead of nixpkgs: information only", "v2.1.1", "2.1.1", []string{"v2.1.1", "v2.1.2"}, check.Info, "v2.1.1", "follows nixpkgs dux-shell 2.1.1; upstream has v2.1.2"},
		{"nixpkgs moved: to its version, not upstream's newest", "v2.1.1", "2.1.2", []string{"v2.1.1", "v2.1.2", "v2.2.0"}, check.Outdated, "v2.1.2", "nixup update dms"},
		{"pin ahead of nixpkgs", "v2.1.2", "2.1.1", []string{"v2.1.1", "v2.1.2"}, check.Info, "v2.1.2", "ahead of nixpkgs dux-shell 2.1.1"},
	} {
		e := testutil.New(t)
		repo := followsRepo(t, e, c.pinned, c.nixpkgs)
		e.Tags("o/dux", c.tags...)
		rows := byName(checker(e).Inputs(repo, false))
		if r := rows["dms"]; r.Status != c.status || r.Latest != c.latest || r.Note != c.note {
			t.Errorf("%s: %+v", c.name, r)
		}
		if r := rows["other"]; r.Status != check.Outdated || r.Latest != "v1.1.0" {
			t.Errorf("%s: an input without the marker keeps the newest tag: %+v", c.name, r)
		}
	}
}

func TestInputsFollowingNixpkgsProblemsAreRows(t *testing.T) {
	e := testutil.New(t)
	repo := followsRepo(t, e, "v2.1.1", "") // nixpkgs has no dux-shell
	e.Tags("o/dux", "v2.1.1", "v2.1.2")
	if r := byName(checker(e).Inputs(repo, false))["dms"]; r.Status != check.Error || !strings.Contains(r.Note, "nixpkgs has no dux-shell") {
		t.Errorf("%+v", r)
	}
	g := testutil.New(t)
	repo = followsRepo(t, g, "v2.1.1", "2.1.3")
	g.Tags("o/dux", "v2.1.1", "v2.1.2")
	if r := byName(checker(g).Inputs(repo, false))["dms"]; r.Status != check.Error || !strings.Contains(r.Note, "has no tag for it") {
		t.Errorf("nixpkgs' version without an upstream tag: %+v", r)
	}
}

func TestFollows(t *testing.T) {
	repo := testutil.Tree(t, map[string]string{"flake.nix": followsNix + `
# nixup: follows nixpkgs#lost

# nixup: follows nixpkgs#zen
inputs.zen.url = "github:o/zen/v1";`})
	got, err := flake.Follows(repo)
	if err != nil || len(got) != 2 || got["dms"] != "dux-shell" || got["zen"] != "zen" {
		t.Errorf("%v %v", got, err)
	}
}
