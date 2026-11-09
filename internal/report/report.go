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
	"github.com/Villoh/nixup/internal/ver"
)

// maxNotes caps how many release notes are shown per update; the compare link has the rest.
const maxNotes = 3

// statusColor is the SGR color of each STATUS value.
var statusColor = map[string]int{"ok": 32, "update": 33, "major": 31, "0.x-minor": 31, "info": 36, "error": 31}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}

// status is the STATUS column: ok, update, info, error, or the kind of a likely
// breaking jump (major, 0.x-minor).
func status(r check.Row) string {
	switch {
	case r.Status == check.Outdated && r.Warn == ver.Major.Warning():
		return string(ver.Major)
	case r.Status == check.Outdated && r.Warn == ver.ZeroMinor.Warning():
		return string(ver.ZeroMinor)
	case r.Status == check.Outdated:
		return "update"
	}
	return r.Status
}

// Text prints check results as one table. Up-to-date rows are only counted unless all is
// set; with review, the upstream changes of each update follow the table.
func Text(t *ui.Term, rows []check.Row, all, review bool) {
	var shown []check.Row
	updates, breaking, info, errs, hidden := 0, 0, 0, 0, 0
	for _, r := range rows {
		switch st := status(r); {
		case r.Status == check.OK && !all:
			hidden++
			continue
		case r.Status == check.Outdated:
			updates++
			if st != "update" {
				breaking++
			}
		case r.Status == check.Info:
			info++
		case r.Status == check.Error:
			errs++
		}
		shown = append(shown, r)
	}
	if len(shown) > 0 {
		cells := make([][]string, len(shown))
		for i, r := range shown {
			action := r.Note
			if r.Hint != "" {
				action = strings.TrimPrefix(action+" · "+r.Hint, " · ")
			}
			cells[i] = []string{check.Source(r.Section), r.Name, r.Current, r.Latest, status(r), action}
		}
		Table(t, []string{"SOURCE", "NAME", "CURRENT", "LATEST", "STATUS", "ACTION"}, cells,
			func(row, col int) int {
				if col == 4 {
					return statusColor[cells[row][4]]
				}
				return 0
			})
		t.Println()
	}
	var sum []string
	if updates > 0 {
		u := fmt.Sprintf("%d to update", updates)
		if breaking > 0 {
			u += fmt.Sprintf(" (%d likely breaking)", breaking)
		}
		sum = append(sum, u)
	}
	if info > 0 {
		sum = append(sum, fmt.Sprintf("%d info", info))
	}
	if errs > 0 {
		sum = append(sum, plural(errs, "error", "errors"))
	}
	if hidden > 0 {
		sum = append(sum, fmt.Sprintf("%d up to date (--all to show them)", hidden))
	}
	if updates == 0 && errs == 0 {
		sum = append([]string{"Everything is up to date."}, sum...)
	}
	t.Println(t.Paint(strings.Join(sum, " · "), 2))
	if !review {
		return
	}
	for _, r := range shown {
		if r.Review != nil {
			t.Printf("\n%s %s  %s → %s\n", t.Bold(r.Name), t.Paint(check.Source(r.Section), 2), r.Current, r.Latest)
			t.Printf("%s", ReviewText(r.Review, "  "))
		}
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// lastWidth caps the last column so a long description does not wrap the table.
const lastWidth = 72

// Table prints rows under a dim header row. Every column is as wide as its widest cell
// and the last one is cut at lastWidth. color returns an SGR code for a cell, 0 for none.
func Table(t *ui.Term, header []string, rows [][]string, color func(row, col int) int) {
	last := len(header) - 1
	cut := func(c int, s string) string {
		if r := []rune(s); c == last && len(r) > lastWidth {
			return string(r[:lastWidth-1]) + "…"
		}
		return s
	}
	width := make([]int, len(header))
	for _, r := range append([][]string{header}, rows...) {
		for c, s := range r {
			width[c] = max(width[c], utf8.RuneCountInString(cut(c, s)))
		}
	}
	line := func(r []string, paint func(c int, s string) string) {
		var b strings.Builder
		for c, s := range r {
			s = cut(c, s)
			pad := strings.Repeat(" ", width[c]-utf8.RuneCountInString(s))
			if c < last {
				pad += "  "
			}
			b.WriteString(paint(c, s) + pad)
		}
		t.Println(strings.TrimRight(b.String(), " "))
	}
	line(header, func(_ int, s string) string { return t.Paint(s, 2) })
	for i, r := range rows {
		line(r, func(c int, s string) string {
			if color != nil && s != "" {
				if code := color(i, c); code != 0 {
					return t.Paint(s, code)
				}
			}
			return s
		})
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

// List prints the inventory as one table, the errors of failed sources, and a count.
func List(t *ui.Term, groups []inventory.Group) {
	var cells [][]string
	var sum []string
	for _, g := range groups {
		for _, it := range g.Items {
			cells = append(cells, []string{g.Source, it.Name, it.Version, it.Note})
		}
		if n := len(g.Items); n > 0 {
			sum = append(sum, plural(n, g.Source, g.Source+"s"))
		}
	}
	if len(cells) > 0 {
		Table(t, []string{"SOURCE", "NAME", "VERSION", "ORIGIN"}, cells, nil)
		t.Println()
	}
	printErrors(t, groups)
	t.Println(t.Paint(strings.Join(sum, " · "), 2))
}

// Search prints search results as one table, marking with a green ● what is installed.
func Search(t *ui.Term, groups []inventory.Group) {
	var cells [][]string
	more, rev := 0, ""
	for _, g := range groups {
		for _, it := range g.Items {
			mark := ""
			if it.InSystem {
				mark = "●"
			}
			cells = append(cells, []string{g.Source, it.Name, it.Version, mark, it.Note})
		}
		more += g.More
		if g.Rev != "" {
			rev = g.Rev
		}
	}
	if len(cells) > 0 {
		Table(t, []string{"SOURCE", "NAME", "VERSION", "INSTALLED", "DESCRIPTION"}, cells,
			func(_, col int) int {
				if col == 3 {
					return 32
				}
				return 0
			})
		t.Println()
	}
	printErrors(t, groups)
	sum := plural(len(cells), "result", "results")
	if rev != "" {
		sum += " (nixpkgs " + rev + ")"
	}
	if more > 0 {
		sum += fmt.Sprintf(" · %d more not shown (refine the search or raise --limit)", more)
	}
	t.Println(t.Paint(sum, 2))
}

func printErrors(t *ui.Term, groups []inventory.Group) {
	for _, g := range groups {
		if g.Err != "" {
			t.Println(t.Paint(g.Source+": "+g.Err, 31))
		}
	}
}
