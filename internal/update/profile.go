package update

import (
	"fmt"
	"strings"

	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
)

// ProfileOpts are the nix profile step of Update.
type ProfileOpts struct {
	Names  []string // empty: every entry
	Yes    bool
	Report string
}

// Profile upgrades the `nix profile` entries that follow a moving reference (such as
// flake:nixpkgs) with `nix profile upgrade`, after asking. An entry pinned to a revision
// cannot move, so it is only reported with what to do instead.
func (u *Updater) Profile(o ProfileOpts) int {
	t := u.T
	entries, err := u.Check.ProfileEntries()
	if err != nil {
		return u.fail(err)
	}
	var moving []string
	for _, e := range entries {
		if len(o.Names) > 0 && !contains(o.Names, e.Name) {
			continue
		}
		if e.Rev != "" {
			t.Printf("%s is pinned to %s: nix profile upgrade cannot move it. To update it, nix profile remove %s and install it again.\n",
				e.Name, flake.ShortRev(e.Rev), e.Name)
			continue
		}
		moving = append(moving, e.Name)
	}
	if len(moving) == 0 {
		if len(o.Names) == 0 {
			t.Println("No nix profile entry follows a moving reference: nothing to upgrade.")
		}
		return exit.OK
	}
	if !o.Yes && !t.Confirm(fmt.Sprintf("Upgrade %s (%s) now? This installs %s right away.",
		plural(len(moving), "nix profile entry", "nix profile entries"), strings.Join(moving, ", "), them(len(moving)))) {
		t.Println("Skipped.")
		return exit.OK
	}
	md := "### nix profile\n\nUpgraded: " + strings.Join(moving, ", ") + "\n"
	if err := u.R.Stream("", "nix", append([]string{"profile", "upgrade"}, moving...)...); err != nil {
		t.Println(t.Paint("nix profile upgrade failed.", 31))
		u.appendReport(o.Report, md+"\n**nix profile upgrade failed.**\n")
		return exit.Error
	}
	t.Println(t.Paint("nix profile upgraded.", 32) + " To undo it: nix profile rollback")
	u.appendReport(o.Report, md+"\nTo undo it: `nix profile rollback`\n")
	return exit.OK
}
