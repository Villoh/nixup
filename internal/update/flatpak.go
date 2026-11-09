package update

import (
	"fmt"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/report"
)

// FlatpakOpts are the Flatpak step of Update.
type FlatpakOpts struct {
	IDs    []string // empty: every pending update
	Yes    bool
	Report string
}

// Flatpak shows the pending Flatpak updates, asks once, and runs `flatpak update` on
// exactly those refs. Unlike the other steps it installs right away, so it first notes
// the deployed commit of each ref and afterwards prints how to go back to it.
func (u *Updater) Flatpak(o FlatpakOpts) int {
	t := u.T
	all, err := u.Check.FlatpakUpdates()
	if err != nil {
		return u.fail(err)
	}
	var todo []check.FlatpakUpdate
	for _, up := range all {
		if len(o.IDs) == 0 || contains(o.IDs, up.ID) {
			todo = append(todo, up)
		}
	}
	for _, id := range o.IDs {
		if !contains(ids(todo), id) {
			t.Println(id + " is up to date.")
		}
	}
	if len(todo) == 0 {
		if len(o.IDs) == 0 {
			t.Println("Every Flatpak is up to date.")
		}
		return exit.OK
	}

	cells := make([][]string, len(todo))
	md := "### Flatpak\n\n| App | Current | Latest |\n|---|---|---|\n"
	for i, up := range todo {
		cells[i] = []string{"flatpak", up.ID, up.Current, up.Latest}
		md += fmt.Sprintf("| %s | %s | %s |\n", up.ID, up.Current, up.Latest)
	}
	report.Table(t, []string{"SOURCE", "NAME", "CURRENT", "LATEST"}, cells, nil)
	t.Println()

	old := map[string]string{} // ref -> deployed commit, for the way back
	for _, up := range todo {
		if out, _, err := u.R.Capture("", "flatpak", "info", "--show-commit", up.Ref); err == nil {
			old[up.Ref] = strings.TrimSpace(out)
		}
	}
	if !o.Yes && !t.Confirm(fmt.Sprintf("Update %s now? This installs %s right away.", plural(len(todo), "Flatpak ref", "Flatpak refs"), them(len(todo)))) {
		t.Println("Skipped.")
		return exit.OK
	}
	refs := make([]string, len(todo))
	for i, up := range todo {
		refs[i] = up.Ref
	}
	runErr := u.R.Stream("", "flatpak", append([]string{"update", "-y", "--noninteractive"}, refs...)...)

	var back []string
	for _, up := range todo {
		if c := old[up.Ref]; c != "" {
			back = append(back, fmt.Sprintf("flatpak update --commit=%s %s", c, up.Ref))
		}
	}
	if runErr != nil {
		t.Println(t.Paint("flatpak update failed; some refs may have been updated.", 31))
	} else {
		t.Println(t.Paint("Flatpak updated.", 32))
	}
	if len(back) > 0 {
		t.Println("To go back to the previous version of one:")
		for _, b := range back {
			t.Println("  " + b)
		}
		md += "\nTo go back:\n\n```bash\n" + strings.Join(back, "\n") + "\n```\n"
	}
	if runErr != nil {
		u.appendReport(o.Report, md+"\n**flatpak update failed.**\n")
		return exit.Error
	}
	u.appendReport(o.Report, md)
	return exit.OK
}

func ids(ups []check.FlatpakUpdate) []string {
	var out []string
	for _, up := range ups {
		out = append(out, up.ID)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
