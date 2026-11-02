package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type lockNode struct {
	Inputs map[string]json.RawMessage `json:"inputs"`
	Locked struct {
		Rev          string `json:"rev"`
		LastModified int64  `json:"lastModified"`
	} `json:"locked"`
	Original struct {
		Type  string `json:"type"`
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		Ref   string `json:"ref"`
	} `json:"original"`
}

type lockFile struct {
	Root  string              `json:"root"`
	Nodes map[string]lockNode `json:"nodes"`
}

func readLock(path string) (*lockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseLock(data)
}

func parseLock(data []byte) (*lockFile, error) {
	var l lockFile
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("flake.lock: %w", err)
	}
	return &l, nil
}

// direct returns the flake's own inputs (name -> node key); `follows` lists are skipped.
func (l *lockFile) direct() map[string]string {
	out := map[string]string{}
	for name, raw := range l.Nodes[l.Root].Inputs {
		var key string
		if json.Unmarshal(raw, &key) == nil {
			out[name] = key
		}
	}
	return out
}

func (l *lockFile) names() []string {
	var n []string
	for k := range l.direct() {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

type change struct {
	Name, Repo, Ref, Old, New string
}

// diffLocks lists direct inputs whose revision moved, plus how many other nodes moved.
func diffLocks(old, cur *lockFile) ([]change, int) {
	od, cd := old.direct(), cur.direct()
	isDirect := map[string]bool{}
	var out []change
	for _, name := range cur.names() {
		node := cur.Nodes[cd[name]]
		isDirect[cd[name]] = true
		o := old.Nodes[od[name]]
		if o.Locked.Rev != node.Locked.Rev {
			c := change{Name: name, Ref: node.Original.Ref, Old: o.Locked.Rev, New: node.Locked.Rev}
			if node.Original.Type == "github" {
				c.Repo = node.Original.Owner + "/" + node.Original.Repo
			}
			out = append(out, c)
		}
	}
	trans := 0
	for key, node := range cur.Nodes {
		if key != cur.Root && !isDirect[key] && node.Locked.Rev != old.Nodes[key].Locked.Rev {
			trans++
		}
	}
	return out, trans
}

// findRepo walks upwards from dir until it finds flake.nix + flake.lock.
func findRepo(dir string) (string, error) {
	p, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if exists(filepath.Join(p, "flake.nix")) && exists(filepath.Join(p, "flake.lock")) {
			return p, nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", fmt.Errorf("no flake.nix + flake.lock found from %s", dir)
		}
		p = parent
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func nixSystem() string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	return arch + "-" + runtime.GOOS
}

// hosts lists nixosConfigurations of the flake.
func hosts(flake string) ([]string, error) {
	out, errs, err := capture("", "nix", "eval", "--json", flake+"#nixosConfigurations",
		"--apply", "builtins.attrNames")
	if err != nil {
		return nil, fmt.Errorf("listing hosts: %s", lastLine(errs))
	}
	var h []string
	return h, json.Unmarshal([]byte(out), &h)
}

// ownPackage is a package defined in the flake's own packages output.
type ownPackage struct {
	Name     string   `json:"-"`
	Version  string   `json:"version"`
	URLs     []string `json:"urls"`
	Homepage string   `json:"homepage"`
	Update   []string `json:"update"`
}

const pkgQuery = `ps: builtins.mapAttrs (n: p: {
  version = p.version or "";
  urls = p.src.urls or [];
  homepage = p.meta.homepage or "";
  update = if p ? passthru && p.passthru ? updateScript
    then map toString (if builtins.isList p.passthru.updateScript then p.passthru.updateScript else p.passthru.updateScript.command or [])
    else [];
}) ps`

func ownPackages(repo string) ([]ownPackage, error) {
	out, errs, err := capture(repo, "nix", "eval", "--json", ".#packages."+nixSystem(), "--apply", pkgQuery)
	if err != nil {
		if strings.Contains(errs, "does not provide attribute") {
			return nil, nil
		}
		return nil, fmt.Errorf("listing packages: %s", lastLine(errs))
	}
	m := map[string]ownPackage{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, err
	}
	var pkgs []ownPackage
	for n, p := range m {
		p.Name = n
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
	return pkgs, nil
}

// upstream is "owner/repo" derived from the package source or homepage.
func (p ownPackage) upstream() string {
	for _, u := range append(append([]string{}, p.URLs...), p.Homepage) {
		if r := githubRepo(u); r != "" {
			return r
		}
	}
	return ""
}

// extraArgs are the nix-update flags declared by the package (minus --version=*, which
// nixup always pins to the reviewed target).
func (p ownPackage) extraArgs() (args []string, allowPre bool) {
	for i, a := range p.Update {
		switch {
		case i == 0:
		case a == "--version=unstable":
			allowPre = true
		case strings.HasPrefix(a, "--version"):
		default:
			args = append(args, a)
		}
	}
	return
}
