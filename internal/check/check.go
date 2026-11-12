// Package check answers "what is outdated?" for inputs, own packages, nix profile and
// Flatpak. Nothing in here writes to disk.
package check

import (
	"cmp"
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

// Sources are the singular --source names of Sections, in the same order.
func Sources() []string {
	out := make([]string, len(Sections))
	for i, s := range Sections {
		out[i] = Source(s)
	}
	return out
}

// Sections in display order, the order update runs them in.
var Sections = []string{"inputs", "packages", "flatpak", "profile"}

// Source is the singular name of a section, as shown in the SOURCE column and accepted
// by --source: input, package, flatpak, profile.
func Source(section string) string { return strings.TrimSuffix(section, "s") }

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
		for _, r := range group {
			if r.Status == OK && r.Latest == "" {
				r.Latest = r.Current // up to date: the latest is what you have
			}
			rows = append(rows, r)
		}
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
	follows := c.Follows(repo, lock)
	return par.Map(len(names), workers, func(i int) Row {
		node, _ := lock.Input(names[i])
		if f, ok := follows[names[i]]; ok && PinnedTag(node) {
			return c.followedInput(names[i], node, f, review)
		}
		return c.input(names[i], node, review)
	})
}

// Follow is an input pinned to a tag that follows a nixpkgs package (a `# nixup: follows
// nixpkgs#<attr>` comment in flake.nix): the flake provides a module for the package
// nixpkgs builds, so the tag moves to nixpkgs' version, not to upstream's newest.
type Follow struct {
	Attr    string
	Version string // in the locked nixpkgs
	Err     string // why the version is unknown
}

// Follows are the followed inputs of the flake, with their package's version in the
// nixpkgs it locks. Problems are kept per input, so each row can say why.
func (c *Checker) Follows(repo string, lock *flake.Lock) map[string]Follow {
	m, err := flake.Follows(repo)
	if err != nil || len(m) == 0 {
		return nil
	}
	out := map[string]Follow{}
	var attrs []string
	for name, attr := range m {
		out[name] = Follow{Attr: attr}
		attrs = append(attrs, attr)
	}
	var versions map[string]string
	if _, ok := lock.Input("nixpkgs"); !ok {
		err = fmt.Errorf("the flake has no nixpkgs input")
	} else {
		versions, err = flake.NixpkgsVersions(c.R, repo, attrs)
	}
	for name, f := range out {
		switch {
		case err != nil:
			f.Err = err.Error()
		case versions[f.Attr] == "":
			f.Err = "nixpkgs has no " + f.Attr
		default:
			f.Version = versions[f.Attr]
		}
		out[name] = f
	}
	return out
}

// FollowTarget is the tag a followed input moves to: the upstream tag of nixpkgs' version
// when it is newer than the pinned one, else "".
func (c *Checker) FollowTarget(node flake.Node, f Follow) (string, error) {
	if f.Err != "" {
		return "", fmt.Errorf("follows nixpkgs#%s: %s", f.Attr, f.Err)
	}
	ref := node.Original.Ref
	if !ver.Newer(f.Version, ref) {
		return "", nil
	}
	repo := node.Original.Owner + "/" + node.Original.Repo
	tags, err := c.GH.Tags(repo)
	if err != nil {
		return "", err
	}
	prefix := ver.Prefix(ref)
	for _, t := range tags {
		if ver.HasPrefix(t, prefix) && ver.Strip(t[len(prefix):]) == f.Version {
			return t, nil
		}
	}
	return "", fmt.Errorf("nixpkgs has %s %s, but %s has no tag for it", f.Attr, f.Version, repo)
}

func (c *Checker) followedInput(name string, node flake.Node, f Follow, review bool) Row {
	ref := node.Original.Ref
	target, err := c.FollowTarget(node, f)
	if err != nil {
		return errRow("inputs", name, err)
	}
	what := "nixpkgs " + f.Attr + " " + f.Version
	if target != "" {
		row := Row{Section: "inputs", Name: name, Current: ref, Latest: target, Status: Outdated,
			Note: "nixup update " + name, Warn: ver.Bump(ref, target).Warning()}
		if review {
			row.Review = c.GH.Review(node.Original.Owner+"/"+node.Original.Repo, ref, target, true)
		}
		return row
	}
	row := Row{Section: "inputs", Name: name, Current: ref, Latest: ref, Status: OK, Note: "follows " + what}
	if ver.Newer(ref, f.Version) {
		row.Status, row.Note = Info, "ahead of "+what
		return row
	}
	if top, err := c.NewerTag(node); err == nil && top != "" {
		row.Status, row.Note = Info, "follows "+what+"; upstream has "+top
	}
	return row
}

// PinnedTag reports whether a GitHub input is pinned to a release tag, which
// `nix flake update` never moves: nixup update rewrites it in flake.nix instead.
func PinnedTag(node flake.Node) bool {
	return node.Original.Type == "github" && node.Original.Ref != "" && ver.Looks(node.Original.Ref)
}

// NewerTag is the highest tag of a pinned input's repository with the same prefix, if it
// is newer than the pinned one, else "".
func (c *Checker) NewerTag(node flake.Node) (string, error) {
	ref := node.Original.Ref
	top, err := c.GH.LatestTag(node.Original.Owner+"/"+node.Original.Repo, ver.Prefix(ref), ver.IsPre(ref))
	if err != nil || !ver.Newer(top, ref) {
		return "", err
	}
	return top, nil
}

func (c *Checker) input(name string, node flake.Node, review bool) Row {
	id := node.Original.Owner + "/" + node.Original.Repo
	ref, rev := node.Original.Ref, node.Locked.Rev
	age := int(time.Since(time.Unix(node.Locked.LastModified, 0)).Hours() / 24)

	if PinnedTag(node) {
		top, err := c.NewerTag(node)
		if err != nil {
			return errRow("inputs", name, err)
		}
		if top == "" {
			return Row{Section: "inputs", Name: name, Current: ref, Status: OK, Note: "pinned tag"}
		}
		row := Row{Section: "inputs", Name: name, Current: ref, Latest: top, Status: Outdated,
			Note: "nixup update " + name, Warn: ver.Bump(ref, top).Warning()}
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
	row := Row{Section: "inputs", Name: name, Current: fmt.Sprintf("%s · %dd", flake.ShortRev(rev), age),
		Latest: flake.ShortRev(head), Status: Outdated, Note: "nixup update " + name}
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
			row.Latest = row.Current // still up to date: All fills this only for ok rows
		}
		break
	}
	return row
}

// packageFromURL compares a package with the version its passthru.nixup.versionUrl
// serves. The review, as for any package, compares the GitHub tags of both versions.
func (c *Checker) packageFromURL(p flake.Package, id string, review bool) Row {
	row := Row{Section: "packages", Name: p.Name, Current: p.Version}
	latest, err := c.GH.Text(p.VersionURL)
	if err != nil {
		return errRow("packages", p.Name, err)
	}
	if !ver.Newer(latest, p.Version) {
		row.Status = OK
		return row
	}
	row.Latest, row.Status = latest, Outdated
	row.Warn = ver.Bump(p.Version, latest).Warning()
	row.Note = "nixup update " + p.Name
	if review && id != "" {
		row.Review = c.GH.Review(id, c.GH.TagFor(id, p.Version, nil), c.GH.TagFor(id, latest, nil), true)
	}
	return row
}

// Package compares one package with the newest upstream release.
func (c *Checker) Package(p flake.Package, review bool) Row {
	row := Row{Section: "packages", Name: p.Name, Current: p.Version}
	id := p.Upstream()
	switch {
	case p.Pinned != "": // kept on purpose: nothing to look up
		row.Status, row.Note = OK, "pinned: "+p.Pinned
		return row
	case p.VersionURL != "":
		return c.packageFromURL(p, id, review)
	case id == "":
		row.Status, row.Note = Info, "no GitHub upstream: check by hand"
		return row
	case len(p.Update) == 0:
		row.Status, row.Note = Info, "no passthru.updateScript"
		return row
	case !p.NixUpdate():
		row.Status, row.Note = Info, "updateScript is not nix-update: set passthru.nixup.versionUrl"
		return row
	}
	_, allowPre := p.ExtraArgs()
	pre := allowPre || ver.IsPre(p.Version)
	extract, err := p.TagVersion()
	if err != nil {
		return errRow("packages", p.Name, err)
	}
	var top, latest string
	if p.UsesReleases() { // mirror nix-update: only published releases count, not every tag
		top, latest, err = c.GH.LatestRelease(id, extract, pre)
	} else {
		top, latest, err = c.GH.LatestVersionTag(id, extract, pre)
	}
	if err != nil {
		return errRow("packages", p.Name, err)
	}
	if !ver.Newer(latest, p.Version) {
		row.Status = OK
		return row
	}
	row.Latest, row.Status = latest, Outdated
	row.Warn = ver.Bump(p.Version, latest).Warning()
	row.Note = "nixup update " + p.Name
	if review {
		row.Review = c.GH.Review(id, c.GH.TagFor(id, p.Version, extract), top, true)
	}
	return row
}

var nixpkgsRev = regexp.MustCompile(`/([0-9a-f]{40})`)

// ProfileEntry is one `nix profile` element. Rev is the nixpkgs-style revision it is
// pinned to, "" when it follows a moving reference such as flake:nixpkgs.
type ProfileEntry struct{ Name, URL, Rev string }

// ProfileEntries lists `nix profile`, sorted by name; none without nix.
func (c *Checker) ProfileEntries() ([]ProfileEntry, error) {
	if !c.R.Has("nix") {
		return nil, nil
	}
	out, errs, err := c.R.Capture("", "nix", "profile", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("nix profile list: %s", run.LastLine(errs))
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var data struct {
		Elements map[string]struct {
			OriginalURL string `json:"originalUrl"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, fmt.Errorf("nix profile list: %w", err)
	}
	var entries []ProfileEntry
	for name, e := range data.Elements {
		pe := ProfileEntry{Name: name, URL: e.OriginalURL}
		if m := nixpkgsRev.FindStringSubmatch(e.OriginalURL); m != nil {
			pe.Rev = m[1]
		}
		entries = append(entries, pe)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// Profile reports `nix profile` entries. One that follows a moving reference can be
// upgraded by nixup update; one pinned to a nixpkgs other than the flake's cannot, and
// says what to do instead. A revision of any other repository says nothing about age.
func (c *Checker) Profile(repo string) []Row {
	entries, err := c.ProfileEntries()
	if err != nil {
		return []Row{{Section: "profile", Name: "nix profile", Status: Error, Note: err.Error()}}
	}
	system := ""
	if lock, err := flake.ReadLock(filepath.Join(repo, "flake.lock")); err == nil {
		node, _ := lock.Input("nixpkgs")
		system = node.Locked.Rev
	}
	var rows []Row
	for _, e := range entries {
		switch {
		case e.Rev == "":
			rows = append(rows, Row{Section: "profile", Name: e.Name, Current: "tracks flake ref", Status: OK,
				Note: "nixup update " + e.Name})
		case system != "" && e.Rev != system && strings.Contains(strings.ToLower(e.URL), "nixpkgs"):
			rows = append(rows, Row{Section: "profile", Name: e.Name, Current: flake.ShortRev(e.Rev), Latest: flake.ShortRev(system),
				Status: Outdated, Note: "pinned: nix profile remove, then install again"})
		default:
			rows = append(rows, Row{Section: "profile", Name: e.Name, Current: flake.ShortRev(e.Rev), Status: OK})
		}
	}
	return rows
}

// FlatpakUpdate is one application or runtime with a pending update.
type FlatpakUpdate struct{ Ref, ID, Current, Latest string }

// FlatpakInstalled are the IDs of the installed applications and runtimes; none without
// flatpak. Runtimes are included because check suggests `nixup update <runtime>` too.
func (c *Checker) FlatpakInstalled() map[string]bool {
	apps := map[string]bool{}
	if !c.R.Has("flatpak") {
		return apps
	}
	if out, _, err := c.R.Capture("", "flatpak", "list", "--columns=application"); err == nil {
		for _, f := range fields(out) {
			apps[f[0]] = true
		}
	}
	return apps
}

// FlatpakUpdates lists applications and runtimes with a pending update, with the
// installed and the new version. A runtime has no version, so its branch is shown and
// its updates are new builds; an app whose remote publishes no version (a .flatpakref
// remote) is unknown.
func (c *Checker) FlatpakUpdates() ([]FlatpakUpdate, error) {
	if !c.R.Has("flatpak") {
		return nil, nil
	}
	out, errs, err := c.R.Capture("", "flatpak", "remote-ls", "--updates", "--columns=ref,version")
	if err != nil {
		return nil, fmt.Errorf("flatpak remote-ls: %s", run.LastLine(errs))
	}
	installed := map[string]string{} // "id/arch/branch" -> version
	if list, _, err := c.R.Capture("", "flatpak", "list", "--columns=ref,version"); err == nil {
		for _, f := range fields(list) {
			_, id, arch, branch := parseRef(f[0])
			installed[id+"/"+arch+"/"+branch] = f[1]
		}
	}
	var ups []FlatpakUpdate
	seen := map[string]bool{}
	for _, f := range fields(out) {
		ref, latest := f[0], f[1]
		kind, id, arch, branch := parseRef(ref)
		if id == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		current := installed[id+"/"+arch+"/"+branch]
		switch {
		case kind == "runtime":
			current, latest = cmp.Or(current, branch), "new build"
		case latest == "":
			latest = "unknown"
		case latest == current:
			latest = "new build"
		}
		ups = append(ups, FlatpakUpdate{Ref: ref, ID: id, Current: current, Latest: latest})
	}
	return ups, nil
}

// Flatpak reports the pending Flatpak updates.
func (c *Checker) Flatpak() []Row {
	ups, err := c.FlatpakUpdates()
	if err != nil {
		return []Row{{Section: "flatpak", Name: "flatpak", Status: Error, Note: err.Error()}}
	}
	var rows []Row
	for _, u := range ups {
		rows = append(rows, Row{Section: "flatpak", Name: u.ID, Current: u.Current, Latest: u.Latest,
			Status: Outdated, Note: "nixup update " + u.ID})
	}
	return rows
}

// parseRef splits a flatpak ref: "app/org.example.App/x86_64/stable" (remote-ls) or
// "org.example.App/x86_64/stable" (flatpak list, which leaves the kind out).
func parseRef(ref string) (kind, id, arch, branch string) {
	p := strings.Split(ref, "/")
	switch len(p) {
	case 4:
		return p[0], p[1], p[2], p[3]
	case 3:
		return "", p[0], p[1], p[2]
	}
	return "", "", "", ""
}

// fields splits flatpak's tab-separated columns into two trimmed fields per line,
// skipping empty lines.
func fields(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		f := append(strings.Split(line, "\t"), "", "")[:2]
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		if f[0] != "" {
			rows = append(rows, f)
		}
	}
	return rows
}
