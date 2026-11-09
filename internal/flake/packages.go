package flake

import (
	"encoding/json"
	"fmt"
	"sort"
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
	Update   []string `json:"update"` // passthru.updateScript command line, if any
}

const packagesQuery = `ps: builtins.mapAttrs (n: p: {
  version = p.version or "";
  urls = p.src.urls or [];
  homepage = p.meta.homepage or "";
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

// Upstream is "owner/repo" taken from the source URL or the homepage, or "".
func (p Package) Upstream() string {
	for _, u := range append(append([]string{}, p.URLs...), p.Homepage) {
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
