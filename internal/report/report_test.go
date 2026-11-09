package report_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/inventory"
	"github.com/Villoh/nixup/internal/report"
	"github.com/Villoh/nixup/internal/testutil"
	"github.com/Villoh/nixup/internal/ver"
)

func sampleReview() *gh.Review {
	return &gh.Review{
		Repo: "lgse/strata", From: "v0.21.0", To: "v1.1.1", Commits: 14,
		CompareURL: "https://github.com/lgse/strata/compare/v0.21.0...v1.1.1",
		Subjects:   []string{"feat: a", "fix: b"},
		Releases:   []gh.Note{{Tag: "v1.1.1", Body: "line1\n\nline2"}},
		SkippedPre: 2,
	}
}

func rows() []check.Row {
	return []check.Row{
		{Section: "inputs", Name: "nixpkgs", Current: "aaa1111", Status: check.OK},
		{Section: "inputs", Name: "dms", Current: "v1.6.2", Latest: "v2.0.0", Status: check.Outdated,
			Note: "pinned tag: edit flake.nix", Warn: ver.Major.Warning(), Review: sampleReview()},
		{Section: "packages", Name: "strata", Current: "0.21.0", Status: check.Info, Note: "no updateScript"},
		{Section: "flatpak", Name: "broken", Status: check.Error, Note: "no remotes"},
	}
}

// lines splits output into lines, dropping the trailing empty one.
func lines(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }

// col is the rune position where needle starts in line, or -1.
func col(line, needle string) int {
	i := strings.Index(line, needle)
	if i < 0 {
		return -1
	}
	return utf8.RuneCountInString(line[:i])
}

func TestTextIsOneTable(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Text(term, rows(), false, false)
	l := lines(out.String())
	if l[0] != "SOURCE   NAME    CURRENT  LATEST  STATUS  ACTION" {
		t.Errorf("header: %q", l[0])
	}
	want := []string{
		"input    dms     v1.6.2   v2.0.0  major   pinned tag: edit flake.nix",
		"package  strata  0.21.0           info    no updateScript",
		"flatpak  broken                   error   no remotes",
		"",
		"1 to update (1 likely breaking) · 1 info · 1 error · 1 up to date (--all to show them)",
	}
	if got := l[1:]; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, line := range l {
		if strings.HasPrefix(line, " ") || strings.HasSuffix(line, " ") {
			t.Errorf("no indentation, no trailing spaces: %q", line)
		}
	}
}

func TestTextAllAndReview(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Text(term, rows(), true, true)
	s := out.String()
	if l := lines(s); !strings.HasPrefix(l[1], "input    nixpkgs  aaa1111") || !strings.HasSuffix(l[1], "ok") {
		t.Errorf("--all shows up-to-date rows:\n%s", s)
	}
	if strings.Contains(s, "up to date (--all") {
		t.Errorf("nothing hidden with --all:\n%s", s)
	}
	review := strings.Index(s, "dms input  v1.6.2 → v2.0.0\n  source   https://github.com/lgse/strata")
	if review < 0 || review < strings.Index(s, "1 to update") {
		t.Errorf("reviews follow the table, one block per update:\n%s", s)
	}
}

func TestTextStatusKinds(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Text(term, []check.Row{
		{Section: "packages", Name: "a", Current: "0.6.1", Latest: "0.7.0", Status: check.Outdated, Warn: ver.ZeroMinor.Warning()},
		{Section: "packages", Name: "b", Current: "1.0", Latest: "1.1", Status: check.Outdated, Note: "nixup pkg b",
			Hint: "nixpkgs has b 1.1: pkgs.b could replace this package"},
		{Section: "packages", Name: "c", Current: "1", Status: check.Info, Hint: "nixpkgs has c 2: pkgs.c could replace this package"},
	}, false, false)
	l := lines(out.String())
	if !strings.Contains(l[1], "0.x-minor") || !strings.Contains(l[2], " update ") {
		t.Errorf("status names the kind of jump:\n%s", out)
	}
	if !strings.HasSuffix(l[2], "nixup pkg b · nixpkgs has b 1.1: pkgs.b could replace this package") ||
		!strings.HasSuffix(l[3], "info       nixpkgs has c 2: pkgs.c could replace this package") {
		t.Errorf("the nixpkgs hint goes in ACTION, after the note if any:\n%s", out)
	}
}

func TestTextEverythingUpToDate(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Text(term, []check.Row{{Section: "inputs", Name: "x", Status: check.OK}}, false, false)
	if got := out.String(); got != "Everything is up to date. · 1 up to date (--all to show them)\n" {
		t.Errorf("no table, just the summary: %q", got)
	}
}

func TestTableColumnsAndCuts(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Table(term, []string{"A", "B", "LAST"}, [][]string{
		{"ünïcode", "x", strings.Repeat("d", 100)},
		{"a", "", ""},
	}, nil)
	l := lines(out.String())
	if col(l[0], "B") != col(l[1], "x") || col(l[1], "x") != len("ünïcode")-2+2 {
		t.Errorf("columns as wide as their widest cell, counted in runes:\n%s", out)
	}
	if !strings.HasSuffix(l[1], strings.Repeat("d", 71)+"…") || strings.Contains(l[1], strings.Repeat("d", 72)) {
		t.Errorf("the last column is cut:\n%s", out)
	}
	if l[2] != "a" {
		t.Errorf("empty cells leave no trailing spaces: %q", l[2])
	}
}

func TestTableColorsDoNotBreakAlignment(t *testing.T) {
	term, out, _ := testutil.Term("")
	term.Color = true
	report.Table(term, []string{"S", "N"}, [][]string{{"major", "x"}, {"ok", "y"}}, func(_, c int) int {
		if c == 0 {
			return 31
		}
		return 0
	})
	l := lines(out.String())
	strip := func(s string) string {
		for strings.Contains(s, "\x1b[") {
			i := strings.Index(s, "\x1b[")
			s = s[:i] + s[i+strings.Index(s[i:], "m")+1:]
		}
		return s
	}
	if col(strip(l[1]), "x") != col(strip(l[2]), "y") || !strings.Contains(l[1], "\x1b[31mmajor\x1b[0m  x") {
		t.Errorf("padding is added outside the color codes:\n%q", l)
	}
}

func TestMarkdown(t *testing.T) {
	md := report.Markdown(rows())
	for _, want := range []string{
		"### inputs / dms `v1.6.2 → v2.0.0`", "> ⚠ MAJOR version jump",
		"[14 commits](https://github.com/lgse/strata/compare/v0.21.0...v1.1.1)", "- feat: a",
		"<details><summary>v1.1.1</summary>", "### flatpak / broken", "Error: no remotes",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "nixpkgs") || strings.Contains(md, "strata `") {
		t.Errorf("only outdated and failing rows belong in a PR body:\n%s", md)
	}
}

func TestMarkdownNothingPending(t *testing.T) {
	got := report.Markdown([]check.Row{{Status: check.OK}, {Status: check.Info}})
	if got != "Everything is up to date.\n" {
		t.Errorf("got %q", got)
	}
}

func TestReviewTextLimitsNotes(t *testing.T) {
	r := sampleReview()
	r.Releases = nil
	for _, tag := range []string{"v1.4.0", "v1.3.0", "v1.2.0", "v1.1.0", "v1.0.0"} {
		r.Releases = append(r.Releases, gh.Note{Tag: tag, Body: "notes for " + tag})
	}
	text := report.ReviewText(r, "  ")
	if strings.Count(text, "notes    v") != 3 || !strings.Contains(text, "(+2 more releases") {
		t.Errorf("at most 3 notes and a pointer to the rest:\n%s", text)
	}
	if !strings.Contains(text, "(2 prereleases in between not shown)") {
		t.Errorf("skipped prereleases are mentioned:\n%s", text)
	}
	md := report.ReviewMarkdown(r)
	if strings.Count(md, "<details>") != 3 {
		t.Errorf("markdown also caps notes:\n%s", md)
	}
}

func TestReviewTextTruncatesLongBodies(t *testing.T) {
	r := sampleReview()
	r.Releases = []gh.Note{{Tag: "v2", Body: strings.Repeat("line\n", 30)}}
	text := report.ReviewText(r, "")
	if strings.Count(text, "line") > 6 || !strings.Contains(text, "…") {
		t.Errorf("long release notes are cut:\n%s", text)
	}
}

func TestReviewWithError(t *testing.T) {
	r := &gh.Review{Err: "github repos/x: 404 Not Found"}
	if got := report.ReviewText(r, "> "); got != "> review unavailable: github repos/x: 404 Not Found\n" {
		t.Errorf("got %q", got)
	}
	if got := report.ReviewMarkdown(r); !strings.Contains(got, "review unavailable") {
		t.Errorf("got %q", got)
	}
}

func TestList(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.List(term, []inventory.Group{
		{Source: "input", Items: []inventory.Item{{Name: "nixpkgs", Version: "e7439b6 · 32d", Note: "github:NixOS/nixpkgs/nixos-unstable"}}},
		{Source: "package", Items: []inventory.Item{{Name: "a", Version: "1"}, {Name: "b", Version: "2", Note: "github:o/b"}}},
		{Source: "flatpak", Err: "no remotes"},
	})
	want := `SOURCE   NAME     VERSION        ORIGIN
input    nixpkgs  e7439b6 · 32d  github:NixOS/nixpkgs/nixos-unstable
package  a        1
package  b        2              github:o/b

flatpak: no remotes
1 input · 2 packages
`
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestSearch(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Search(term, []inventory.Group{
		{Source: "nixpkgs", Rev: "abc", More: 3, Items: []inventory.Item{
			{Name: "ripgrep", Version: "15.2.0", Note: "Fast grep", InSystem: true},
			{Name: "ripgrep-all", Version: "0.10.10"}}},
		{Source: "input:strata", Items: []inventory.Item{{Name: "strata", Version: "1.1.1", Note: "Files"}}},
	})
	want := `SOURCE        NAME         VERSION  INSTALLED  DESCRIPTION
nixpkgs       ripgrep      15.2.0   ●          Fast grep
nixpkgs       ripgrep-all  0.10.10
input:strata  strata       1.1.1               Files

3 results (nixpkgs abc) · 3 more not shown (refine the search or raise --limit)
`
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
