package report_test

import (
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/report"
	"github.com/Villoh/nixup/internal/testutil"
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
			Note: "pinned tag: edit flake.nix", Warn: "MAJOR version jump", Review: sampleReview()},
		{Section: "packages", Name: "strata", Current: "0.21.0", Status: check.Info, Note: "no updateScript"},
		{Section: "flatpak", Name: "broken", Status: check.Error, Note: "no remotes"},
	}
}

func TestTextHidesUpToDateRows(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Text(term, rows(), false, false)
	s := out.String()
	for _, want := range []string{"inputs", "UPDATE", "dms", "v1.6.2 -> v2.0.0", "⚠ MAJOR version jump", "ERROR", "no remotes", "1 up to date (use --all to list)"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "nixpkgs") {
		t.Errorf("up-to-date rows are only counted:\n%s", s)
	}
	if strings.Contains(s, "source ") {
		t.Errorf("no review without the flag:\n%s", s)
	}
}

func TestTextAllAndReview(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Text(term, rows(), true, true)
	s := out.String()
	for _, want := range []string{"nixpkgs", "source   https://github.com/lgse/strata", "14 commits", "· feat: a"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "up to date (use") {
		t.Errorf("nothing is hidden with --all:\n%s", s)
	}
	// Sections follow the fixed order, not the order of the rows.
	if strings.Index(s, "inputs") > strings.Index(s, "packages") || strings.Index(s, "packages") > strings.Index(s, "flatpak") {
		t.Errorf("section order:\n%s", s)
	}
}

func TestTextOmitsEmptySections(t *testing.T) {
	term, out, _ := testutil.Term("")
	report.Text(term, []check.Row{{Section: "inputs", Name: "x", Status: check.OK}}, false, false)
	if strings.Contains(out.String(), "flatpak") || strings.Contains(out.String(), "profile") {
		t.Errorf("%q", out.String())
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
