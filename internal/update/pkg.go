package update

import (
	"fmt"
	"regexp"
	"slices"
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
	t.Println(t.Paint("Updating "+p.File(o.Repo)+" with nix-update…", 2))
	// Quiet: its evaluations and diff are summarised below; they matter only on failure.
	if out, errs, err := u.R.Capture(o.Repo, cmd[0], cmd[1:]...); err != nil {
		u.restore(restore)
		t.Errorln(strings.TrimSpace(out + "\n" + errs))
		t.Println(t.Paint("nix-update failed; "+dir+" restored.", 31))
		u.appendReport(o.Report, md+"\n**nix-update failed.**\n")
		return exit.Error
	}
	// Always: the edited recipe must build before the update counts.
	t.Println(t.Bold("Checking that " + p.Name + " builds…"))
	if err := u.R.Stream(o.Repo, "nix", "build", "--no-link", ".#"+p.Name); err != nil {
		u.restore(restore)
		t.Println(t.Paint("build FAILED; "+dir+" restored.", 31))
		u.appendReport(o.Report, md+"\n**Build failed; not applied.**\n")
		return exit.Error
	}
	diff, _, _ := u.R.Capture(o.Repo, "git", "diff", "--", dir)
	summary, edits := summarizeDiff(diff)
	t.Println(t.Paint(p.Name+" updated to "+row.Latest+".", 32))
	md += "\nUpdated" + summary + "\n"
	if edits != "" {
		t.Println(t.Paint("Besides version and hash, nix-update edited:", 33))
		t.Printf("%s", edits)
		md += "\nBesides version and hash, nix-update edited:\n\n```diff\n" + edits + "```\n"
	}
	u.edited = true
	u.appendReport(o.Report, md+"\nBuild: ok.\n")
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

var attrLine = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_'-]*)\s*=`)

// routine are the attributes nix-update is expected to rewrite.
func routine(attr string) bool {
	switch attr {
	case "version", "hash", "sha256", "rev":
		return true
	}
	return strings.HasSuffix(attr, "Hash") // vendorHash, cargoHash, npmDepsHash...
}

// summarizeDiff reads the diff of a package update. summary is ": file.nix (version,
// hash)" for the report; extra lists what deserves a look: changed lines that are not
// routine, and other files (lock files nix-update regenerates), counted, not shown.
func summarizeDiff(diff string) (summary, extra string) {
	type file struct {
		name     string
		attrs    []string
		add, del int
	}
	var files []*file
	var odd strings.Builder
	for _, l := range strings.Split(diff, "\n") {
		if strings.HasPrefix(l, "diff --git ") {
			_, b, _ := strings.Cut(l, " b/")
			files = append(files, &file{name: b})
			continue
		}
		if len(files) == 0 || len(l) == 0 || (l[0] != '+' && l[0] != '-') || strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---") {
			continue
		}
		f := files[len(files)-1]
		if l[0] == '+' {
			f.add++
		} else {
			f.del++
		}
		if !strings.HasSuffix(f.name, ".nix") || strings.TrimSpace(l[1:]) == "" {
			continue
		}
		if m := attrLine.FindStringSubmatch(l[1:]); m != nil && routine(m[1]) {
			if !slices.Contains(f.attrs, m[1]) {
				f.attrs = append(f.attrs, m[1])
			}
			continue
		}
		odd.WriteString("  " + f.name + ": " + l + "\n")
	}
	var parts []string
	for _, f := range files {
		switch {
		case !strings.HasSuffix(f.name, ".nix"):
			parts = append(parts, fmt.Sprintf("%s (+%d −%d lines)", f.name, f.add, f.del))
			fmt.Fprintf(&odd, "  %s: +%d −%d lines\n", f.name, f.add, f.del)
		case len(f.attrs) > 0:
			parts = append(parts, f.name+" ("+strings.Join(f.attrs, ", ")+")")
		default:
			parts = append(parts, f.name)
		}
	}
	if len(parts) == 0 {
		return ".", odd.String()
	}
	return ": " + strings.Join(parts, ", "), odd.String()
}
