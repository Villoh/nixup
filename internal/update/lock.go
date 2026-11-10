package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/par"
	"github.com/Villoh/nixup/internal/report"
)

type LockOpts struct {
	Repo       string
	Inputs     []string // empty: all
	Hosts      []string // empty: every nixosConfiguration
	NoBuild    bool
	Yes        bool
	AllowDirty bool // go on over an uncommitted flake.lock without asking (-y does too)
	Report     string
}

// Lock updates flake.lock in a scratch copy, shows what moved, builds every host there and
// only then, after a yes, writes the new lock into the repo.
func (u *Updater) Lock(o LockOpts) int {
	t := u.T
	lockPath := filepath.Join(o.Repo, "flake.lock")
	oldData, err := os.ReadFile(lockPath)
	if err != nil {
		return u.fail(err)
	}
	old, err := flake.ParseLock(oldData)
	if err != nil {
		return u.fail(err)
	}
	if flake.Dirty(u.R, o.Repo, "flake.lock") && !u.onDirty("flake.lock", u.uncommittedInputs(o.Repo, old), o.AllowDirty || o.Yes) {
		return exit.OK
	}
	tmp, err := os.MkdirTemp("", "nixup-")
	if err != nil {
		return u.fail(err)
	}
	defer os.RemoveAll(tmp)
	if err := flake.ScratchCopy(u.R, o.Repo, tmp); err != nil {
		return u.fail(err)
	}

	what := "(all inputs)"
	if len(o.Inputs) > 0 {
		what = strings.Join(o.Inputs, " ")
	}
	t.Println(t.Bold("== nix flake update " + what))
	if err := u.R.Stream("", "nix", append([]string{"flake", "update", "--flake", tmp}, o.Inputs...)...); err != nil {
		return u.fail(err)
	}
	newData, err := os.ReadFile(filepath.Join(tmp, "flake.lock"))
	if err != nil {
		return u.fail(err)
	}
	if string(newData) == string(oldData) {
		t.Println("flake.lock is already up to date.")
		return exit.OK
	}
	cur, err := flake.ParseLock(newData)
	if err != nil {
		return u.fail(err)
	}

	md := u.showChanges(old, cur)

	if !o.NoBuild {
		hosts := o.Hosts
		if len(hosts) == 0 {
			if hosts, err = flake.Hosts(u.R, tmp); err != nil {
				return u.fail(err)
			}
		}
		for _, h := range hosts {
			t.Println(t.Bold("\n== nh os build -H " + h + " (scratch copy, nothing is activated)"))
			if err := u.R.Stream("", "nh", "os", "build", tmp, "-H", h, "-o", filepath.Join(tmp, "result")); err != nil {
				t.Println(t.Paint("build of "+h+" FAILED: flake.lock left untouched.", 31))
				u.appendReport(o.Report, md+fmt.Sprintf("\n**Build of `%s` failed.**\n", h))
				return exit.Error
			}
			md += fmt.Sprintf("\nBuild of `%s`: ok.\n", h)
		}
	}
	u.appendReport(o.Report, md)

	if !o.Yes && !t.Confirm("\nWrite this flake.lock to the repo?") {
		t.Println("Discarded. Nothing changed.")
		return exit.OK
	}
	if err := os.WriteFile(lockPath, newData, 0o644); err != nil {
		return u.fail(err)
	}
	t.Println(t.Paint("flake.lock updated (not committed, nothing activated).", 32))
	t.Println("Next: git diff flake.lock && nix flake check && nh os test . -H <host>")
	return exit.OK
}

// uncommittedInputs names the inputs the working flake.lock moved since the last commit,
// as " (a, b)", or "" when that cannot be told.
func (u *Updater) uncommittedInputs(repo string, working *flake.Lock) string {
	out, _, err := u.R.Capture(repo, "git", "show", "HEAD:flake.lock")
	if err != nil {
		return ""
	}
	head, err := flake.ParseLock([]byte(out))
	if err != nil {
		return ""
	}
	changes, _ := flake.Diff(head, working)
	var names []string
	for _, c := range changes {
		names = append(names, c.Name)
	}
	if len(names) == 0 {
		return ""
	}
	return " (" + strings.Join(names, ", ") + ")"
}

// showChanges prints each moved input with its upstream review and returns the same as Markdown.
func (u *Updater) showChanges(old, cur *flake.Lock) string {
	t := u.T
	changes, transitive := flake.Diff(old, cur)
	reviews := par.Map(len(changes), 6, func(i int) *gh.Review {
		if c := changes[i]; c.Repo != "" {
			return u.GH.Review(c.Repo, c.Old, c.New, false)
		}
		return nil
	})
	var md strings.Builder
	md.WriteString("### flake.lock\n\n")
	for i, c := range changes {
		t.Printf("\n%s  %s -> %s\n", t.Bold(c.Name), flake.ShortRev(c.Old), flake.ShortRev(c.New))
		fmt.Fprintf(&md, "#### %s `%s` → `%s`\n\n", c.Name, flake.ShortRev(c.Old), flake.ShortRev(c.New))
		if r := reviews[i]; r != nil {
			t.Printf("%s", report.ReviewText(r, "  "))
			md.WriteString(report.ReviewMarkdown(r) + "\n")
		}
	}
	t.Printf("\n(+ %d transitive inputs changed)\n", transitive)
	fmt.Fprintf(&md, "%d inputs changed, %d transitive.\n", len(changes), transitive)
	return md.String()
}
