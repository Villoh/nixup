package update

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/report"
)

type PkgOpts struct {
	Repo       string
	Names      []string // empty: every package
	NoBuild    bool
	Yes        bool
	AllowDirty bool // go on over uncommitted changes without asking (-y does too)
	Report     string
}

func (u *Updater) nixUpdate() []string {
	if u.R.Has("nix-update") {
		return []string{"nix-update"}
	}
	return []string{"nix", "run", "nixpkgs#nix-update", "--"}
}

// Pkg shows what changes upstream for each outdated own package, asks, runs nix-update
// pinned to the version just reviewed, builds it and prints the diff. A failed update or
// build puts the package files back as they were, uncommitted changes included. The result is the worst code over all packages.
func (u *Updater) Pkg(o PkgOpts) int {
	pkgs, err := flake.Packages(u.R, o.Repo)
	if err != nil {
		return u.fail(err)
	}
	todo, err := flake.Select(pkgs, o.Names)
	if err != nil {
		return u.fail(err)
	}
	code, outdated := exit.OK, 0
	for _, p := range todo {
		row := u.Check.Package(p, true)
		switch row.Status {
		case check.Error:
			u.T.Printf("%s %s: %s\n", u.T.Paint("ERROR", 31), p.Name, row.Note)
			code = max(code, exit.Error)
		case check.Outdated:
			outdated++
			code = max(code, u.updateOne(o, p, row))
		default:
			if len(o.Names) > 0 || row.Status == check.Info {
				u.T.Printf("%s %s %s %s\n", u.T.Paint(strings.ToUpper(row.Status), 32), p.Name, row.Current, row.Note)
			}
		}
	}
	if outdated == 0 && len(o.Names) == 0 {
		u.T.Println("Every package is up to date.")
	}
	return code
}

func (u *Updater) updateOne(o PkgOpts, p flake.Package, row check.Row) int {
	t := u.T
	t.Printf("\n%s  %s -> %s\n", t.Bold(p.Name), row.Current, row.Latest)
	md := fmt.Sprintf("### %s `%s` → `%s`\n\n", p.Name, row.Current, row.Latest)
	if row.Warn != "" {
		t.Println("  " + t.Paint("⚠ "+row.Warn, 33))
		md += "> ⚠ " + row.Warn + "\n\n"
	}
	if row.Review != nil {
		t.Printf("%s", report.ReviewText(row.Review, "  "))
		md += report.ReviewMarkdown(row.Review)
	}
	dir := p.Path(o.Repo) // known before any question, so a refusal never follows a yes
	if dir == "" {
		t.Println(t.Paint("cannot tell which file in this flake defines "+p.Name+" (meta.position "+
			strconv.Quote(p.Position)+"): not updating it.", 31))
		return exit.Error
	}
	if flake.Dirty(u.R, o.Repo, dir) && !u.onDirty(dir, "", o.AllowDirty || o.Yes) {
		return exit.OK
	}
	if !o.Yes && !t.Confirm(fmt.Sprintf("Update %s to %s?", p.Name, row.Latest)) {
		t.Println("Skipped.")
		return exit.OK
	}
	restore, err := flake.Snapshot(o.Repo, dir)
	if err != nil {
		return u.fail(err)
	}
	extra, _ := p.ExtraArgs()
	cmd := append(u.nixUpdate(), "--flake", p.Name, "--version="+row.Latest)
	cmd = append(cmd, extra...)
	if err := u.R.Stream(o.Repo, cmd[0], cmd[1:]...); err != nil {
		u.restore(restore)
		t.Println(t.Paint("nix-update failed; "+dir+" restored.", 31))
		u.appendReport(o.Report, md+"\n**nix-update failed.**\n")
		return exit.Error
	}
	if !o.NoBuild {
		t.Println(t.Bold("Building " + p.Name + "…"))
		if err := u.R.Stream(o.Repo, "nix", "build", "--no-link", ".#"+p.Name); err != nil {
			u.restore(restore)
			t.Println(t.Paint("build FAILED; "+dir+" restored.", 31))
			u.appendReport(o.Report, md+"\n**Build failed; not applied.**\n")
			return exit.Error
		}
	}
	diff, _, _ := u.R.Capture(o.Repo, "git", "diff", "--", dir)
	t.Println(diff)
	t.Println(t.Paint(p.Name+" updated.", 32))
	u.edited = true
	built := "Build: ok."
	if o.NoBuild {
		built = "Build: skipped (--no-build)."
	}
	u.appendReport(o.Report, md+"\n"+built+"\n")
	return exit.OK
}

func (u *Updater) restore(restore func() error) {
	if err := restore(); err != nil {
		u.T.Errorln("nixup: restore failed:", err)
	}
}

// onDirty decides whether to go on over uncommitted changes in path. They are kept, so
// it only asks; allow (--allow-dirty or -y) says yes up front.
func (u *Updater) onDirty(path, detail string, allow bool) bool {
	msg := path + " has uncommitted changes" + detail
	if allow {
		u.T.Println(u.T.Paint(msg+": updating on top of them; they are kept.", 2))
		return true
	}
	if u.T.Confirm(msg + ". Update on top of them? They are kept.") {
		return true
	}
	u.T.Println("Skipped.")
	return false
}
