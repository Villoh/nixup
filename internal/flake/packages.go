package flake

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/run"
)

// Package is a package defined in the flake's own `packages.<system>` output.
type Package struct {
	Name     string   `json:"-"`
	Version  string   `json:"version"`
	URLs     []string `json:"urls"`
	Homepage string   `json:"homepage"`
	Update   []string `json:"update"`   // passthru.updateScript command line, if any
	Position string   `json:"position"` // meta.position: "<store path>/<file>:<line>"
	Pinned   string   `json:"pinned"`   // passthru.nixup.pinned: why it is kept where it is
}

const packagesQuery = `ps: builtins.mapAttrs (n: p: {
  version = p.version or "";
  urls = p.src.urls or [];
  homepage = p.meta.homepage or "";
  position = p.meta.position or "";
  pinned = p.passthru.nixup.pinned or "";
  update = if p ? passthru && p.passthru ? updateScript
    then map toString (if builtins.isList p.passthru.updateScript then p.passthru.updateScript else p.passthru.updateScript.command or [])
    else [];
}) ps`

// Packages evaluates the flake's own packages. A flake without a packages output has none.
func Packages(r run.Runner, repo string) ([]Package, error) {
	out, errs, err := r.Capture(repo, "nix", "eval", "--json", ".#packages."+System(), "--apply", packagesQuery)
	if err != nil {
		if strings.Contains(errs, "does not provide attribute") {
			return nil, nil
		}
		return nil, fmt.Errorf("listing packages: %s", run.LastLine(errs))
	}
	m := map[string]Package{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}
	pkgs := make([]Package, 0, len(m))
	for name, p := range m {
		p.Name = name
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
	return pkgs, nil
}

// Select returns the named packages (all when names is empty), or an error naming the
// first one that does not exist.
func Select(pkgs []Package, names []string) ([]Package, error) {
	if len(names) == 0 {
		return pkgs, nil
	}
	byName := map[string]Package{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	var out []Package
	for _, n := range names {
		p, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("no such package: %s", n)
		}
		out = append(out, p)
	}
	return out, nil
}

// Path is what nix-update edits for this package, relative to the flake: the directory of
// the file that defines it (meta.position, the same thing nix-update uses), or that file
// alone when it sits at the top of the flake, so a restore never touches the whole
// repository. "" when the file is not part of this flake.
func (p Package) Path(repo string) string {
	rel := p.File(repo)
	if rel == "" {
		return ""
	}
	if dir := filepath.Dir(rel); dir != "." {
		return dir
	}
	return rel
}

// File is the file that defines the package, relative to the repository (from
// meta.position), or "" when it is not part of this flake.
func (p Package) File(repo string) string {
	file := p.Position
	if i := strings.LastIndex(file, ":"); i >= 0 {
		file = file[:i] // drop the line number
	}
	_, rel, ok := strings.Cut(file, "-source/") // flake sources are copied to /nix/store/<hash>-source
	if !ok || rel == "" || strings.HasPrefix(rel, "..") || !exists(filepath.Join(repo, rel)) {
		return ""
	}
	return rel
}

// Upstream is "owner/repo" taken from the update script's --url (where nix-update looks
// for versions when the source is hosted elsewhere), the source URL or the homepage, or "".
func (p Package) Upstream() string {
	var urls []string
	for i, a := range p.Update {
		switch {
		case a == "--url" && i+1 < len(p.Update):
			urls = append(urls, p.Update[i+1])
		case strings.HasPrefix(a, "--url="):
			urls = append(urls, strings.TrimPrefix(a, "--url="))
		}
	}
	for _, u := range append(append(urls, p.URLs...), p.Homepage) {
		if r := gh.RepoFromURL(u); r != "" {
			return r
		}
	}
	return ""
}

// UsesReleases reports whether the package's update script asks nix-update to follow
// GitHub releases instead of tags (--use-github-releases).
func (p Package) UsesReleases() bool {
	for _, a := range p.Update {
		if a == "--use-github-releases" {
			return true
		}
	}
	return false
}

// TagVersion reads a version out of a tag the way nix-update does when the update script
// passes --version-regex: the regex must match at the start of the tag, and its non-empty
// groups joined by "." are the version (nix-update's extract_version). Without the flag
// it returns nil, and the default rules apply.
func (p Package) TagVersion() (gh.Extract, error) {
	expr := ""
	for i, a := range p.Update {
		if v, ok := strings.CutPrefix(a, "--version-regex="); ok {
			expr = v
		} else if a == "--version-regex" && i+1 < len(p.Update) {
			expr = p.Update[i+1]
		}
	}
	if expr == "" {
		return nil, nil
	}
	re, err := regexp.Compile(`^(?:` + expr + `)`)
	if err != nil {
		return nil, fmt.Errorf("%s: bad --version-regex: %w", p.Name, err)
	}
	return func(tag string) (string, bool) {
		m := re.FindStringSubmatch(tag)
		if m == nil {
			return "", false
		}
		var groups []string
		for _, g := range m[1:] {
			if g != "" {
				groups = append(groups, g)
			}
		}
		return strings.Join(groups, "."), len(groups) > 0
	}, nil
}

// ExtraArgs are the nix-update flags the package declares. --version=* is dropped, since
// nixup always pins the version it just reviewed; --version=unstable only sets allowPre.
func (p Package) ExtraArgs() (args []string, allowPre bool) {
	for i, a := range p.Update {
		switch {
		case i == 0: // the nix-update binary
		case a == "--version=unstable":
			allowPre = true
		case strings.HasPrefix(a, "--version"):
		default:
			args = append(args, a)
		}
	}
	return
}

// nixpkgsQuery evaluates, in the flake's nixpkgs, the attributes it is given. Missing
// attributes and the ones that fail to evaluate (aliases of removed packages throw) are
// left out.
const nixpkgsQuery = `p: builtins.listToAttrs (builtins.filter (x: x.value != null) (map (a: {
  name = a;
  value = let
    v = if p ? ${a} then {
      version = toString (p.${a}.version or "");
      urls = map toString (p.${a}.src.urls or []);
      homepage = toString (p.${a}.meta.homepage or "");
    } else null;
    r = builtins.tryEval (builtins.deepSeq v v);
  in if r.success then r.value else null;
}) [ %s ]))`

var attrName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_'-]*$`)

// nixpkgsNames are the attributes a package may have in nixpkgs: its own name and, for
// "foo-bin", also "foo" (the usual name of the build from source).
func nixpkgsNames(name string) []string {
	if base, ok := strings.CutSuffix(name, "-bin"); ok && base != "" {
		return []string{name, base}
	}
	return []string{name}
}

// InNixpkgs looks each package up by name in the nixpkgs revision locked by the flake
// (`--inputs-from`) and returns, per package name, what exists there, in lookup order.
// Only call it when the flake has an input called nixpkgs: otherwise nix falls back to
// the global registry and answers about a nixpkgs the flake does not use.
func InNixpkgs(r run.Runner, repo string, pkgs []Package) (map[string][]Package, error) {
	var quoted []string
	seen := map[string]bool{}
	for _, p := range pkgs {
		for _, n := range nixpkgsNames(p.Name) {
			if attrName.MatchString(n) && !seen[n] {
				seen[n] = true
				quoted = append(quoted, strconv.Quote(n))
			}
		}
	}
	if len(quoted) == 0 {
		return nil, nil
	}
	out, errs, err := r.Capture(repo, "nix", "eval", "--json", "--inputs-from", ".",
		"nixpkgs#legacyPackages."+System(), "--apply", fmt.Sprintf(nixpkgsQuery, strings.Join(quoted, " ")))
	if err != nil {
		return nil, fmt.Errorf("looking packages up in nixpkgs: %s", run.LastLine(errs))
	}
	m := map[string]Package{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("looking packages up in nixpkgs: %w", err)
	}
	found := map[string][]Package{}
	for _, p := range pkgs {
		for _, n := range nixpkgsNames(p.Name) {
			if q, ok := m[n]; ok {
				q.Name = n
				found[p.Name] = append(found[p.Name], q)
			}
		}
	}
	return found, nil
}

// SameProject reports whether two packages are positively the same software: they share a
// GitHub repository (from the sources or the homepage) or the same homepage. Without
// evidence the answer is no, so a namesake never passes for the real thing.
func SameProject(a, b Package) bool {
	if ra, rb := a.Upstream(), b.Upstream(); ra != "" && strings.EqualFold(ra, rb) {
		return true
	}
	ha, hb := site(a.Homepage), site(b.Homepage)
	return ha != "" && ha == hb
}

func site(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return strings.TrimSuffix(strings.TrimPrefix(u, "www."), "/")
}
