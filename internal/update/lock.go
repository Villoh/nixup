package update

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/par"
	"github.com/Villoh/nixup/internal/report"
	"github.com/Villoh/nixup/internal/ver"
)

type LockOpts struct {
	Repo       string
	Inputs     []string // empty: all
	Hosts      []string // empty: every nixosConfiguration
	Build      bool     // build the hosts in the scratch copy before writing
	Yes        bool
	AllowDirty bool // go on over an uncommitted flake.nix without asking (-y does too)
	Report     string
}

// Lock updates flake.lock in a scratch copy, shows what moved, builds every host there and
// only then, after a yes, writes the new lock into the repo. Inputs pinned to a release
// tag are moved by rewriting the tag in flake.nix, which is then written too.
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
	if flake.Dirty(u.R, o.Repo, "flake.lock") {
		// Nothing is lost either way, so no question: only say what the result will carry
		// besides this update.
		if others, ok := u.uncommittedOthers(o.Repo, old, o.Inputs); !ok || len(others) > 0 {
			which := ""
			if len(others) > 0 {
				which = " (" + strings.Join(others, ", ") + ")"
			}
			t.Println(t.Paint("flake.lock already has uncommitted changes"+which+": they stay, and this update goes on top.", 2))
		}
	}
	bumps := u.tagBumps(old, o.Inputs)
	if len(bumps) > 0 && flake.Dirty(u.R, o.Repo, "flake.nix") && !u.onDirty("flake.nix", "", o.AllowDirty || o.Yes) {
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
	for _, b := range bumps {
		if err := flake.ReplaceTag(filepath.Join(tmp, "flake.nix"), b.old, b.new); err != nil {
			return u.fail(fmt.Errorf("%s: %w; change it in flake.nix by hand", b.input, err))
		}
		t.Printf("Moving the %s tag in flake.nix: %s -> %s\n", b.input, b.old, b.new)
		if b.warn != "" {
			t.Println("  " + t.Paint("⚠ "+b.warn, 33))
		}
	}

	what := "every input"
	if len(o.Inputs) > 0 {
		what = strings.Join(o.Inputs, ", ")
	}
	t.Println(t.Paint("Updating "+what+" in a scratch copy…", 2))
	// Quiet: what moved is summarised below; nix's own lines only matter when it fails.
	if _, errs, err := u.R.Capture("", "nix", append([]string{"flake", "update", "--flake", tmp}, o.Inputs...)...); err != nil {
		return u.fail(fmt.Errorf("nix flake update failed:\n%s", strings.TrimSpace(errs)))
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
	for _, b := range bumps { // nix, not the text edit, has the last word on what is pinned
		if node, _ := cur.Input(b.input); node.Original.Ref != b.new {
			return u.fail(fmt.Errorf("after the edit, flake.lock pins %s to %q, not %s: nothing written", b.input, node.Original.Ref, b.new))
		}
	}

	md := u.showChanges(old, cur)
	for _, b := range bumps {
		md += fmt.Sprintf("\nTag of `%s` moved in flake.nix: `%s` → `%s`.\n", b.input, b.old, b.new)
	}

	if o.Build {
		hosts := o.Hosts
		if len(hosts) == 0 {
			if hosts, err = flake.Hosts(u.R, tmp); err != nil {
				return u.fail(err)
			}
		}
		for _, h := range hosts {
			t.Println(t.Bold("\nBuilding " + h + " in the scratch copy (nothing is activated)…"))
			if err := u.R.Stream("", "nh", "os", "build", tmp, "-H", h, "-o", filepath.Join(tmp, "result")); err != nil {
				t.Println(t.Paint("build of "+h+" FAILED: flake.lock left untouched.", 31))
				u.appendReport(o.Report, md+fmt.Sprintf("\n**Build of `%s` failed.**\n", h))
				return exit.Error
			}
			md += fmt.Sprintf("\nBuild of `%s`: ok.\n", h)
		}
	}
	u.appendReport(o.Report, md)

	files := "flake.lock"
	if len(bumps) > 0 {
		files = "flake.lock and flake.nix"
	}
	if !o.Yes && !t.Confirm("\nWrite this "+files+" to the repo?") {
		t.Println("Discarded. Nothing changed.")
		return exit.OK
	}
	if len(bumps) > 0 {
		nix, err := os.ReadFile(filepath.Join(tmp, "flake.nix"))
		if err == nil {
			err = os.WriteFile(filepath.Join(o.Repo, "flake.nix"), nix, 0o644)
		}
		if err != nil {
			return u.fail(err)
		}
	}
	if err := os.WriteFile(lockPath, newData, 0o644); err != nil {
		return u.fail(err)
	}
	t.Println(t.Paint(files+" updated.", 32))
	u.edited = true
	return exit.OK
}

// tagBump is an input pinned to a release tag that Lock moves by rewriting flake.nix.
type tagBump struct{ input, old, new, warn string }

// tagBumps are the pinned tags with a newer release. Without names a likely breaking jump
// is left alone and only reported: it moves when you name the input.
func (u *Updater) tagBumps(lock *flake.Lock, names []string) []tagBump {
	scope := names
	if len(scope) == 0 {
		scope = lock.Names()
	}
	var bumps []tagBump
	for _, name := range scope {
		node, ok := lock.Input(name)
		if !ok || !check.PinnedTag(node) {
			continue
		}
		top, err := u.Check.NewerTag(node)
		if err != nil {
			u.T.Printf("%s: cannot list its tags (%v): its tag stays.\n", name, err)
			continue
		}
		if top == "" {
			continue
		}
		b := tagBump{name, node.Original.Ref, top, ver.Bump(node.Original.Ref, top).Warning()}
		if b.warn != "" && len(names) == 0 {
			u.T.Printf("%s %s -> %s is a likely breaking jump: left alone. Run nixup update %s to move it.\n", name, b.old, b.new, name)
			continue
		}
		bumps = append(bumps, b)
	}
	return bumps
}

// uncommittedOthers names the inputs the working flake.lock moved since the last commit
// that this update does not cover (named: the others; no names: none). ok is false when
// it cannot be told what changed.
func (u *Updater) uncommittedOthers(repo string, working *flake.Lock, scope []string) (others []string, ok bool) {
	out, _, err := u.R.Capture(repo, "git", "show", "HEAD:flake.lock")
	if err != nil {
		return nil, false
	}
	head, err := flake.ParseLock([]byte(out))
	if err != nil {
		return nil, false
	}
	changes, _ := flake.Diff(head, working)
	if len(changes) == 0 {
		return nil, false // something moved, but no direct input: say so without names
	}
	for _, c := range changes {
		if len(scope) > 0 && !slices.Contains(scope, c.Name) {
			others = append(others, c.Name)
		}
	}
	return others, true
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
	if transitive > 0 {
		t.Printf("\n(+ %d transitive inputs changed)\n", transitive)
	}
	fmt.Fprintf(&md, "%d inputs changed, %d transitive.\n", len(changes), transitive)
	return md.String()
}
