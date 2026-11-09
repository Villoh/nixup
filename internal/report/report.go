// Package report renders check results and upstream reviews as terminal text or Markdown.
package report

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/inventory"
	"github.com/Villoh/nixup/internal/ui"
)

// maxNotes caps how many release notes are shown per update; the compare link has the rest.
const maxNotes = 3

var (
	colors = map[string]int{check.OK: 32, check.Outdated: 33, check.Info: 36, check.Error: 31}
	marks  = map[string]string{check.OK: "ok", check.Outdated: "UPDATE", check.Info: "info", check.Error: "ERROR"}
)

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}

func versionText(r check.Row) string {
	if r.Latest == "" {
		return r.Current
	}
	return r.Current + " -> " + r.Latest
}

// Text prints rows grouped by section. Up-to-date rows are only counted unless all is set.
// Name and version columns are as wide as the longest shown value, the same in every section.
func Text(t *ui.Term, rows []check.Row, all, review bool) {
	nameW, verW := 0, 0
	for _, r := range rows {
		if r.Status != check.OK || all {
			nameW = max(nameW, utf8.RuneCountInString(r.Name))
			verW = max(verW, utf8.RuneCountInString(versionText(r)))
		}
	}
	for _, sec := range check.Sections {
		var group []check.Row
		for _, r := range rows {
			if r.Section == sec {
				group = append(group, r)
			}
		}
		if len(group) == 0 {
			continue
		}
		t.Println("\n" + t.Bold(sec))
		hidden := 0
		for _, r := range group {
			if r.Status == check.OK && !all {
				hidden++
				continue
			}
			mark := t.Paint(fmt.Sprintf("%-6s", marks[r.Status]), colors[r.Status])
			line := fmt.Sprintf("  %s %-*s   %-*s   %s", mark, nameW, r.Name, verW, versionText(r), r.Note)
			t.Println(strings.TrimRight(line, " "))
			if r.Warn != "" {
				t.Println("         " + t.Paint("⚠ "+r.Warn, 33))
			}
			if r.Hint != "" {
				t.Println("         " + t.Paint("→ "+r.Hint, 36))
			}
			if review && r.Review != nil {
				t.Printf("%s", ReviewText(r.Review, "         "))
			}
		}
		if hidden > 0 {
			t.Printf("  %d up to date (use --all to list)\n", hidden)
		}
	}
}

// Markdown lists what is outdated or failing, for a PR body or a CI summary.
func Markdown(rows []check.Row) string {
	var b strings.Builder
	n := 0
	for _, r := range rows {
		if r.Status != check.Outdated && r.Status != check.Error {
			continue
		}
		n++
		version := r.Current
		if r.Latest != "" {
			version += " → " + r.Latest
		}
		fmt.Fprintf(&b, "### %s / %s `%s`\n\n", r.Section, r.Name, version)
		if r.Status == check.Error {
			fmt.Fprintf(&b, "Error: %s\n\n", r.Note)
		}
		if r.Warn != "" {
			fmt.Fprintf(&b, "> ⚠ %s\n\n", r.Warn)
		}
		if r.Hint != "" {
			fmt.Fprintf(&b, "> → %s\n\n", r.Hint)
		}
		if r.Review != nil {
			b.WriteString(ReviewMarkdown(r.Review) + "\n")
		}
	}
	if n == 0 {
		return "Everything is up to date.\n"
	}
	return b.String()
}

// ReviewText renders a review indented under its row.
func ReviewText(r *gh.Review, indent string) string {
	var b strings.Builder
	line := func(f string, a ...any) { fmt.Fprintf(&b, indent+f+"\n", a...) }
	if r.Err != "" {
		line("review unavailable: %s", r.Err)
		return b.String()
	}
	line("source   https://github.com/%s", r.Repo)
	line("changes  %d commits  %s", r.Commits, r.CompareURL)
	for _, s := range r.Subjects {
		line("  · %s", s)
	}
	for i, n := range r.Releases {
		if i == maxNotes {
			line("(+%d more releases: see the compare link)", len(r.Releases)-maxNotes)
			break
		}
		line("notes    %s", n.Tag)
		for _, l := range strings.Split(firstLines(n.Body, 6), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				line("         %s", l)
			}
		}
	}
	if r.SkippedPre > 0 {
		line("(%d prereleases in between not shown)", r.SkippedPre)
	}
	return b.String()
}

// ReviewMarkdown renders a review for a PR body.
func ReviewMarkdown(r *gh.Review) string {
	if r.Err != "" {
		return fmt.Sprintf("_review unavailable: %s_\n", r.Err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[%s](https://github.com/%s) · [%d commits](%s)\n\n", r.Repo, r.Repo, r.Commits, r.CompareURL)
	for _, s := range r.Subjects {
		fmt.Fprintf(&b, "- %s\n", s)
	}
	for i, n := range r.Releases {
		if i == maxNotes {
			break
		}
		fmt.Fprintf(&b, "\n<details><summary>%s</summary>\n\n%s\n\n</details>\n", n.Tag, firstLines(n.Body, 25))
	}
	return b.String()
}

// noteWidth caps descriptions in Inventory so one long line does not wrap the table.
const noteWidth = 72

// Inventory prints list and search results: one block per source, aligned columns, a
// green ● before what is in the running system.
func Inventory(t *ui.Term, groups []inventory.Group) {
	nameW, verW := 0, 0
	for _, g := range groups {
		for _, it := range g.Items {
			nameW = max(nameW, utf8.RuneCountInString(it.Name))
			verW = max(verW, utf8.RuneCountInString(it.Version))
		}
	}
	for _, g := range groups {
		t.Println("\n" + t.Bold(g.Source))
		if g.Err != "" {
			t.Println("  " + t.Paint("ERROR "+g.Err, 31))
		}
		for _, it := range g.Items {
			mark := " "
			if it.InSystem {
				mark = t.Paint("●", 32)
			}
			note := it.Note
			if r := []rune(note); len(r) > noteWidth {
				note = string(r[:noteWidth-1]) + "…"
			}
			line := fmt.Sprintf("  %s %-*s   %-*s   %s", mark, nameW, it.Name, verW, it.Version, note)
			t.Println(strings.TrimRight(line, " "))
		}
		if g.More > 0 {
			t.Printf("    … %d more (refine the search or raise --limit)\n", g.More)
		}
	}
}
