package check_test

import (
	"errors"
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
	if dms.Status != check.Outdated || dms.Latest != "v2.0.0" {
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
	if np.Status != check.Outdated || np.Latest != "bbb2222" || np.Note != "nixup lock nixpkgs" {
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
 "noscript": {"version":"1","urls":["https://github.com/o/noscript/x"],"homepage":"","update":[]}
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
	if len(rows) != 5 {
		t.Fatalf("rows: %+v", rows)
	}
	p := rows["padpoll"]
	if p.Status != check.Outdated || p.Latest != "1.1.0" || p.Note != "nixup pkg padpoll" || p.Warn != "" {
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
		"tracked":{"originalUrl":"flake:nixpkgs"}}}`})
	repo := repoWithLock(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#" + sys})

	rows := byName(checker(e).Profile(repo))
	if q := rows["qtdeclarative"]; q.Status != check.Outdated || q.Current != "aaaaaaa" || q.Latest != "eeeeeee" {
		t.Errorf("pinned to an old nixpkgs: %+v", q)
	}
	if rows["hello"].Status != check.OK || rows["tracked"].Status != check.OK {
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
	if rows := checker(e3).Profile("/repo"); rows != nil {
		t.Errorf("unreadable output is ignored: %+v", rows)
	}
}

func TestFlatpak(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "flatpak remote-ls --updates", Out: "org.gnome.Platform\n\norg.gnome.Platform\n co.hyprlab.Hylki \n"})
	rows := checker(e).Flatpak()
	if len(rows) != 2 || rows[0].Name != "org.gnome.Platform" || rows[1].Name != "co.hyprlab.Hylki" {
		t.Errorf("duplicates collapse, blanks and spaces are dropped: %+v", rows)
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
	e.Run.On(run.Rule{Prefix: "flatpak remote-ls", Out: "org.gnome.Platform\n"})
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
