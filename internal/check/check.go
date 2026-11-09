// Package check answers "what is outdated?" for inputs, own packages, nix profile and
// Flatpak. Nothing in here writes to disk.
package check

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/par"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ver"
)

const (
	OK       = "ok"
	Outdated = "outdated"
	Info     = "info"
	Error    = "error"
)

// Sections in display order.
var Sections = []string{"inputs", "packages", "profile", "flatpak"}

type Row struct {
	Section string     `json:"section"`
	Name    string     `json:"name"`
	Current string     `json:"current"`
	Latest  string     `json:"latest,omitempty"`
	Status  string     `json:"status"`
	Note    string     `json:"note,omitempty"`
	Warn    string     `json:"warning,omitempty"`
	Hint    string     `json:"hint,omitempty"`
	Review  *gh.Review `json:"review,omitempty"`
}

type Checker struct {
	R  run.Runner
	GH *gh.Client
}

const workers = 6

func errRow(section, name string, err error) Row {
	return Row{Section: section, Name: name, Status: Error, Note: err.Error()}
}

// ExitCode: 2 if any check failed, 1 if anything is outdated, else 0.
func ExitCode(rows []Row) int {
	code := exit.OK
	for _, r := range rows {
		switch r.Status {
		case Error:
			return exit.Error
		case Outdated:
			code = exit.Pending
		}
	}
	return code
}

// All runs the requested sections (all when only is empty) concurrently.
func (c *Checker) All(repo string, only []string, review bool) []Row {
	jobs := map[string]func() []Row{
		"inputs":   func() []Row { return c.Inputs(repo, review) },
		"packages": func() []Row { return c.Packages(repo, review) },
		"profile":  func() []Row { return c.Profile(repo) },
		"flatpak":  c.Flatpak,
	}
	var names []string
	for _, s := range Sections {
		if len(only) == 0 || contains(only, s) {
			names = append(names, s)
		}
	}
	var rows []Row
	for _, group := range par.Map(len(names), len(names), func(i int) []Row { return jobs[names[i]]() }) {
		rows = append(rows, group...)
	}
	return rows
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Inputs compares tag-pinned inputs with the newest tag and branch inputs with the remote head.
func (c *Checker) Inputs(repo string, review bool) []Row {
	lock, err := flake.ReadLock(filepath.Join(repo, "flake.lock"))
	if err != nil {
		return []Row{errRow("inputs", "flake.lock", err)}
	}
	var names []string
	for _, n := range lock.Names() {
		if node, _ := lock.Input(n); node.Original.Type == "github" {
			names = append(names, n)
		}
	}
	return par.Map(len(names), workers, func(i int) Row {
		node, _ := lock.Input(names[i])
		return c.input(names[i], node, review)
	})
}

func (c *Checker) input(name string, node flake.Node, review bool) Row {
	id := node.Original.Owner + "/" + node.Original.Repo
	ref, rev := node.Original.Ref, node.Locked.Rev
	age := int(time.Since(time.Unix(node.Locked.LastModified, 0)).Hours() / 24)

	if ref != "" && ver.Looks(ref) { // release tag: `nix flake update` never moves it
		top, err := c.GH.LatestTag(id, ver.Prefix(ref), ver.IsPre(ref))
		if err != nil {
			return errRow("inputs", name, err)
		}
		if !ver.Newer(top, ref) {
			return Row{Section: "inputs", Name: name, Current: ref, Status: OK, Note: "pinned tag"}
		}
		row := Row{Section: "inputs", Name: name, Current: ref, Latest: top, Status: Outdated,
			Note: "pinned tag: edit flake.nix", Warn: ver.Bump(ref, top).Warning()}
		if review {
			row.Review = c.GH.Review(id, ref, top, true)
		}
		return row
	}

	head, err := c.GH.Head(id, ref)
	if err != nil {
		return errRow("inputs", name, err)
	}
	if head == rev {
		return Row{Section: "inputs", Name: name, Current: flake.ShortRev(rev), Status: OK}
	}
	row := Row{Section: "inputs", Name: name, Current: fmt.Sprintf("%s (%dd)", flake.ShortRev(rev), age),
		Latest: flake.ShortRev(head), Status: Outdated, Note: "nixup lock " + name}
	if review {
		row.Review = c.GH.Review(id, rev, head, false)
	}
	return row
}

// Packages checks every package of the flake's own `packages` output.
func (c *Checker) Packages(repo string, review bool) []Row {
	pkgs, err := flake.Packages(c.R, repo)
	if err != nil {
		return []Row{errRow("packages", "packages", err)}
	}
	rows := par.Map(len(pkgs), workers, func(i int) Row { return c.Package(pkgs[i], review) })
	var found map[string][]flake.Package
	if lock, err := flake.ReadLock(filepath.Join(repo, "flake.lock")); err == nil {
		if _, ok := lock.Input("nixpkgs"); ok {
			found, _ = flake.InNixpkgs(c.R, repo, pkgs) // ponytail: a failed lookup only loses the hint
		}
	}
	for i, p := range pkgs {
		rows[i] = nixpkgsHint(rows[i], p, found[p.Name])
	}
	return rows
}

// nixpkgsHint points out a package that the flake's own nixpkgs already has, as the same
// project, at the same or a newer version: the custom package can probably go.
func nixpkgsHint(row Row, p flake.Package, candidates []flake.Package) Row {
	for _, q := range candidates {
		if q.Version == "" || !flake.SameProject(p, q) || ver.Newer(p.Version, q.Version) {
			continue
		}
		row.Hint = fmt.Sprintf("nixpkgs has %s %s: pkgs.%s could replace this package", q.Name, q.Version, q.Name)
		if row.Status == OK {
			row.Status = Info // shown without --all, without changing the exit code
		}
		break
	}
	return row
}

// Package compares one package with the newest upstream release.
func (c *Checker) Package(p flake.Package, review bool) Row {
	row := Row{Section: "packages", Name: p.Name, Current: p.Version}
	id := p.Upstream()
	switch {
	case id == "":
		row.Status, row.Note = Info, "no GitHub upstream: check by hand"
		return row
	case len(p.Update) == 0:
		row.Status, row.Note = Info, "no passthru.updateScript"
		return row
	}
	_, allowPre := p.ExtraArgs()
	pre := allowPre || ver.IsPre(p.Version)
	var top string
	var err error
	if p.UsesReleases() { // mirror nix-update: only published releases count, not every tag
		top, err = c.GH.LatestRelease(id, pre)
	} else {
		top, err = c.GH.LatestTag(id, "", pre)
	}
	if err != nil {
		return errRow("packages", p.Name, err)
	}
	if !ver.Newer(top, p.Version) {
		row.Status = OK
		return row
	}
	row.Latest, row.Status = ver.Strip(top), Outdated
	row.Warn = ver.Bump(p.Version, top).Warning()
	row.Note = "nixup pkg " + p.Name
	if review {
		row.Review = c.GH.Review(id, c.GH.TagFor(id, p.Version), top, true)
	}
	return row
}

var nixpkgsRev = regexp.MustCompile(`/([0-9a-f]{40})`)

// Profile flags `nix profile` entries pinned to a nixpkgs other than the system one.
func (c *Checker) Profile(repo string) []Row {
	if !c.R.Has("nix") {
		return nil
	}
	out, errs, err := c.R.Capture("", "nix", "profile", "list", "--json")
	if err != nil {
		return []Row{{Section: "profile", Name: "nix profile", Status: Error, Note: run.LastLine(errs)}}
	}
	var data struct {
		Elements map[string]struct {
			OriginalURL string `json:"originalUrl"`
		} `json:"elements"`
	}
	if json.Unmarshal([]byte(out), &data) != nil {
		return nil
	}
	system := ""
	if lock, err := flake.ReadLock(filepath.Join(repo, "flake.lock")); err == nil {
		node, _ := lock.Input("nixpkgs")
		system = node.Locked.Rev
	}
	var names []string
	for n := range data.Elements {
		names = append(names, n)
	}
	sort.Strings(names)
	var rows []Row
	for _, n := range names {
		m := nixpkgsRev.FindStringSubmatch(data.Elements[n].OriginalURL)
		switch {
		case m == nil:
			rows = append(rows, Row{Section: "profile", Name: n, Current: "tracks flake ref", Status: OK,
				Note: "nix profile upgrade " + n})
		case system != "" && m[1] != system:
			rows = append(rows, Row{Section: "profile", Name: n, Current: flake.ShortRev(m[1]), Latest: flake.ShortRev(system),
				Status: Outdated, Note: "pinned to an old nixpkgs"})
		default:
			rows = append(rows, Row{Section: "profile", Name: n, Current: flake.ShortRev(m[1]), Status: OK})
		}
	}
	return rows
}

// Flatpak lists applications and runtimes with a pending update.
func (c *Checker) Flatpak() []Row {
	if !c.R.Has("flatpak") {
		return nil
	}
	out, errs, err := c.R.Capture("", "flatpak", "remote-ls", "--updates", "--columns=application")
	if err != nil {
		return []Row{{Section: "flatpak", Name: "flatpak", Status: Error, Note: run.LastLine(errs)}}
	}
	var rows []Row
	seen := map[string]bool{} // one application can be listed for several branches
	for _, line := range strings.Split(out, "\n") {
		if app := strings.TrimSpace(line); app != "" && !seen[app] {
			seen[app] = true
			rows = append(rows, Row{Section: "flatpak", Name: app, Latest: "update", Status: Outdated,
				Note: "flatpak update"})
		}
	}
	return rows
}
