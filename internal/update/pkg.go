package update

import (
	"fmt"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/report"
)

type PkgOpts struct {
	Repo    string
	Names   []string // empty: every package
	NoBuild bool
	Yes     bool
	Report  string
}

func (u *Updater) nixUpdate() []string {
	if u.R.Has("nix-update") {
		return []string{"nix-update"}
	}
	return []string{"nix", "run", "nixpkgs#nix-update", "--"}
}

// Pkg shows what changes upstream for each outdated own package, asks, runs nix-update
// pinned to the version just reviewed, builds it and prints the diff. A failed update or
// build puts the package files back. The result is the worst code over all packages.
func (u *Updater) Pkg(o PkgOpts) int {
	pkgs, err := flake.Packages(u.R, o.Repo)
	if err != nil {
		return u.fail(err)
	}
	todo, err := flake.Select(pkgs, o.Names)
	if err != nil {
		return u.fail(err)
	}
	code := exit.OK
	for _, p := range todo {
		row := u.Check.Package(p, true)
		switch row.Status {
		case check.Error:
			u.T.Printf("%s %s: %s\n", u.T.Paint("ERROR", 31), p.Name, row.Note)
			code = max(code, exit.Error)
		case check.Outdated:
			code = max(code, u.updateOne(o, p, row))
		default:
			if len(o.Names) > 0 || row.Status == check.Info {
				u.T.Printf("%s %s %s %s\n", u.T.Paint(strings.ToUpper(row.Status), 32), p.Name, row.Current, row.Note)
			}
		}
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
	if !o.Yes && !t.Confirm(fmt.Sprintf("Update %s to %s?", p.Name, row.Latest)) {
		t.Println("Skipped.")
		return exit.OK
	}

	dir := "packages/" + p.Name
	if flake.Dirty(u.R, o.Repo, dir) {
		t.Println(t.Paint(dir+" has uncommitted changes: commit or stash them first.", 33))
		return exit.Pending
	}
	extra, _ := p.ExtraArgs()
	cmd := append(u.nixUpdate(), "--flake", p.Name, "--version="+row.Latest)
	cmd = append(cmd, extra...)
	if err := u.R.Stream(o.Repo, cmd[0], cmd[1:]...); err != nil {
		u.restore(o.Repo, dir)
		t.Println(t.Paint("nix-update failed; "+dir+" restored.", 31))
		u.appendReport(o.Report, md+"\n**nix-update failed.**\n")
		return exit.Error
	}
	if !o.NoBuild {
		t.Println(t.Bold("== nix build .#" + p.Name))
		if err := u.R.Stream(o.Repo, "nix", "build", "--no-link", ".#"+p.Name); err != nil {
			u.restore(o.Repo, dir)
			t.Println(t.Paint("build FAILED; "+dir+" restored.", 31))
			u.appendReport(o.Report, md+"\n**Build failed; not applied.**\n")
			return exit.Error
		}
	}
	diff, _, _ := u.R.Capture(o.Repo, "git", "diff", "--", dir)
	t.Println(diff)
	t.Println(t.Paint(p.Name+" updated (not committed).", 32))
	u.appendReport(o.Report, md+"\nBuild: ok.\n")
	return exit.OK
}

func (u *Updater) restore(repo, dir string) {
	if err := flake.Restore(u.R, repo, dir); err != nil {
		u.T.Errorln("nixup: restore failed:", err)
	}
}
