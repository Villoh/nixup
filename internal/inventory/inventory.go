// Package inventory lists what a flake-based system is made of (list) and looks a name up
// in every place a package can come from (search). Both only read.
package inventory

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/par"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ver"
)

// Item is one thing found: an input, a package, a profile entry or a Flatpak app.
type Item struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Note     string `json:"note,omitempty"`
	InSystem bool   `json:"in_system,omitempty"`
}

// Group is the items of one source, in display order.
type Group struct {
	Source string `json:"source"`
	Items  []Item `json:"items"`
	More   int    `json:"more,omitempty"` // matches left out by the limit
	Err    string `json:"error,omitempty"`
}

// List reads, without asking any remote, everything nixup watches: the flake's inputs and
// own packages, `nix profile` entries and installed Flatpak apps.
func List(r run.Runner, repo string) []Group {
	jobs := []func() Group{
		func() Group { return listInputs(repo) },
		func() Group { return listPackages(r, repo) },
		func() Group { return listProfile(r, repo) },
		func() Group { return listFlatpak(r) },
	}
	var out []Group
	for _, g := range par.Map(len(jobs), len(jobs), func(i int) Group { return jobs[i]() }) {
		if len(g.Items) > 0 || g.Err != "" {
			out = append(out, g)
		}
	}
	return out
}

func listInputs(repo string) Group {
	g := Group{Source: "inputs"}
	lock, err := flake.ReadLock(filepath.Join(repo, "flake.lock"))
	if err != nil {
		g.Err = err.Error()
		return g
	}
	for _, name := range lock.Names() {
		node, _ := lock.Input(name)
		it := Item{Name: name, Note: node.Original.Type}
		switch o := node.Original; {
		case o.Ref != "" && ver.Looks(o.Ref):
			it.Version = o.Ref
		case node.Locked.Rev != "":
			age := int(time.Since(time.Unix(node.Locked.LastModified, 0)).Hours() / 24)
			it.Version = fmt.Sprintf("%s (%dd)", flake.ShortRev(node.Locked.Rev), age)
		}
		if o := node.Original; o.Type == "github" {
			it.Note = "github:" + o.Owner + "/" + o.Repo
			if o.Ref != "" {
				it.Note += "/" + o.Ref
			}
		}
		g.Items = append(g.Items, it)
	}
	return g
}

func listPackages(r run.Runner, repo string) Group {
	g := Group{Source: "packages"}
	pkgs, err := flake.Packages(r, repo)
	if err != nil {
		g.Err = err.Error()
		return g
	}
	for _, p := range pkgs {
		g.Items = append(g.Items, Item{Name: p.Name, Version: p.Version, Note: p.Upstream()})
	}
	return g
}

func listProfile(r run.Runner, repo string) Group {
	g := Group{Source: "profile"}
	for _, row := range (&check.Checker{R: r}).Profile(repo) {
		g.Items = append(g.Items, Item{Name: row.Name, Version: row.Current, Note: row.Note})
	}
	return g
}

func listFlatpak(r run.Runner) Group {
	g := Group{Source: "flatpak"}
	if !r.Has("flatpak") {
		return g
	}
	out, errs, err := r.Capture("", "flatpak", "list", "--app", "--columns=application,version")
	if err != nil {
		g.Err = run.LastLine(errs)
		return g
	}
	for _, f := range tsv(out) {
		g.Items = append(g.Items, Item{Name: f[0], Version: at(f, 1)})
	}
	return g
}

// tsv splits tab-separated lines, skipping empty ones.
func tsv(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			rows = append(rows, strings.Split(line, "\t"))
		}
	}
	return rows
}

func at(f []string, i int) string {
	if i < len(f) {
		return strings.TrimSpace(f[i])
	}
	return ""
}

var queryRe = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// pattern turns a query into a case-insensitive POSIX regex for builtins.match.
func pattern(q string) string {
	var b strings.Builder
	for _, c := range q {
		lo, up := unicode.ToLower(c), unicode.ToUpper(c)
		switch {
		case lo != up:
			fmt.Fprintf(&b, "[%c%c]", lo, up)
		case c == '.' || c == '+':
			b.WriteString(`\` + string(c))
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// searchExpr finds attribute names matching the query (exact, then prefix, then anywhere)
// in the flake's nixpkgs, its own packages and every input's packages, and reads the
// version and description of the first `limit` of each. Attributes that throw when used
// (aliases of packages removed from nixpkgs) are not results.
const searchExpr = `let
  f = builtins.getFlake (toString ./.);
  sys = %s; q = %s; limit = %d;
  hits = names: let
    m = re: builtins.filter (n: builtins.match re n != null) names;
    exact = m q; prefix = m (q + ".+");
    inner = builtins.filter (n: builtins.match (q + ".*") n == null) (m (".+" + q + ".*"));
  in exact ++ prefix ++ inner;
  take = l: builtins.genList (builtins.elemAt l) (if builtins.length l < limit then builtins.length l else limit);
  find = ps: let
    r = builtins.tryEval (builtins.attrNames ps);
    all = builtins.filter (n: (builtins.tryEval ps.${n}).success) (if r.success then hits r.value else []);
    one = n: let
      v = { name = n; pname = toString (ps.${n}.pname or ""); version = toString (ps.${n}.version or "");
            description = toString (ps.${n}.meta.description or ""); };
      t = builtins.tryEval (builtins.deepSeq v v);
    in if t.success then t.value else { name = n; pname = ""; version = ""; description = ""; };
  in { total = builtins.length all; items = map one (take all); };
  safe = x: let r = builtins.tryEval (x.packages.${sys} or {}); in if r.success then r.value else {};
in {
  nixpkgs = if f.inputs ? nixpkgs then find (f.inputs.nixpkgs.legacyPackages.${sys} or {}) else null;
  rev = if f.inputs ? nixpkgs then f.inputs.nixpkgs.shortRev or "" else "";
  own = find (safe f);
  inputs = builtins.mapAttrs (n: i: find (safe i)) (removeAttrs f.inputs [ "nixpkgs" "self" ]);
}`

type found struct {
	Total int `json:"total"`
	Items []struct {
		Name, Pname, Version, Description string
	} `json:"items"`
}

// Search looks query up, by name, in the flake's locked nixpkgs, its own packages, the
// packages of every input, and Flathub. Items in the running system are marked.
func Search(r run.Runner, repo, query string, limit int) ([]Group, error) {
	if !queryRe.MatchString(query) {
		return nil, fmt.Errorf("search for a package name: letters, digits and . _ + - only")
	}
	var nixGroups, fpGroups []Group
	var nixErr error
	par.Map(2, 2, func(i int) struct{} {
		if i == 0 {
			nixGroups, nixErr = searchNix(r, repo, query, limit)
		} else {
			fpGroups = searchFlatpak(r, query, limit)
		}
		return struct{}{}
	})
	if nixErr != nil {
		return nil, nixErr
	}
	return append(nixGroups, fpGroups...), nil
}

func searchNix(r run.Runner, repo, query string, limit int) ([]Group, error) {
	expr := fmt.Sprintf(searchExpr, strconv.Quote(flake.System()), strconv.Quote(pattern(query)), limit)
	out, errs, err := r.Capture(repo, "nix", "eval", "--json", "--impure", "--expr", expr)
	if err != nil {
		return nil, fmt.Errorf("searching the flake: %s", run.LastLine(errs))
	}
	var res struct {
		Nixpkgs *found           `json:"nixpkgs"`
		Rev     string           `json:"rev"`
		Own     found            `json:"own"`
		Inputs  map[string]found `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, fmt.Errorf("searching the flake: %w", err)
	}
	inSystem := closure(r)
	group := func(source string, f found) Group {
		g := Group{Source: source, More: f.Total - len(f.Items)}
		for _, it := range f.Items {
			pname := it.Pname
			if pname == "" {
				pname = it.Name
			}
			g.Items = append(g.Items, Item{Name: it.Name, Version: it.Version, Note: oneLine(it.Description),
				InSystem: it.Version != "" && inSystem[pname+"-"+it.Version]})
		}
		return g
	}
	var groups []Group
	if res.Nixpkgs != nil && res.Nixpkgs.Total > 0 {
		groups = append(groups, group(strings.TrimSpace("nixpkgs "+res.Rev), *res.Nixpkgs))
	}
	if res.Own.Total > 0 {
		groups = append(groups, group("packages (yours)", res.Own))
	}
	names := make([]string, 0, len(res.Inputs))
	for n := range res.Inputs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if res.Inputs[n].Total > 0 {
			groups = append(groups, group("input "+n, res.Inputs[n]))
		}
	}
	return groups, nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// closure is the set of "name-version" of every store path in the running system, or
// empty where there is none (CI, containers).
func closure(r run.Runner) map[string]bool {
	set := map[string]bool{}
	out, _, err := r.Capture("", "nix-store", "--query", "--requisites", "/run/current-system")
	if err != nil {
		return set
	}
	for _, p := range strings.Fields(out) {
		base := p[strings.LastIndex(p, "/")+1:]
		if len(base) > 33 && base[32] == '-' { // strip the 32-character hash
			set[base[33:]] = true
		}
	}
	return set
}

func searchFlatpak(r run.Runner, query string, limit int) []Group {
	if !r.Has("flatpak") {
		return nil
	}
	g := Group{Source: "flatpak"}
	out, errs, err := r.Capture("", "flatpak", "search", "--columns=application,version,name,description", query)
	if err != nil {
		g.Err = run.LastLine(errs)
		return []Group{g}
	}
	installed := map[string]bool{}
	if list, _, err := r.Capture("", "flatpak", "list", "--app", "--columns=application"); err == nil {
		for _, f := range tsv(list) {
			installed[f[0]] = true
		}
	}
	var hits [][]string
	seen := map[string]bool{} // an app is listed once per remote and branch
	for _, f := range tsv(out) {
		if len(f) >= 2 && !seen[f[0]] { // len skips "No matches found"
			seen[f[0]] = true
			hits = append(hits, f)
		}
	}
	// Same order as the Nix sources: the app called exactly that, then names that start
	// with it, then the rest (flatpak's own order puts themes and extensions first).
	rank := func(f []string) int {
		name, id := strings.ToLower(at(f, 2)), strings.ToLower(f[0])
		q := strings.ToLower(query)
		switch {
		case name == q || strings.HasSuffix(id, "."+q):
			return 0
		case strings.HasPrefix(name, q):
			return 1
		}
		return 2
	}
	sort.SliceStable(hits, func(i, j int) bool { return rank(hits[i]) < rank(hits[j]) })
	for _, f := range hits {
		if len(g.Items) == limit {
			g.More++
			continue
		}
		g.Items = append(g.Items, Item{Name: f[0], Version: at(f, 1), Note: oneLine(at(f, 3)), InSystem: installed[f[0]]})
	}
	if len(g.Items) == 0 {
		return nil
	}
	return []Group{g}
}
