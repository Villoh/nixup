// Package flake reads what nixup needs from a flake: flake.lock, the repo layout,
// the hosts, and the packages defined in the flake's own `packages` output.
package flake

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
)

type Node struct {
	Inputs map[string]json.RawMessage `json:"inputs"`
	Locked struct {
		Rev          string `json:"rev"`
		URL          string `json:"url"`
		LastModified int64  `json:"lastModified"`
	} `json:"locked"`
	Original struct {
		Type  string `json:"type"`
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		Ref   string `json:"ref"`
		URL   string `json:"url"`
	} `json:"original"`
}

// channelRe matches a NixOS channel tarball, the way the NixOS wiki writes the nixpkgs input:
// https://channels.nixos.org/nixos-unstable/nixexprs.tar.xz
var channelRe = regexp.MustCompile(`^(https://(?:channels\.nixos\.org|nixos\.org/channels)/[^/?]+)/nixexprs\.tar\.(?:xz|gz)(?:\?|$)`)

// Channel is the URL of a channel tarball input's directory, "" for any other input.
func (n Node) Channel() string {
	if n.Original.Type != "tarball" {
		return ""
	}
	if m := channelRe.FindStringSubmatch(n.Original.URL); m != nil {
		return m[1]
	}
	return ""
}

// Repo is "owner/name" for a github input, or NixOS/nixpkgs for a channel tarball; else "".
func (n Node) Repo() string {
	switch {
	case n.Original.Type == "github":
		return n.Original.Owner + "/" + n.Original.Repo
	case n.Channel() != "":
		return "NixOS/nixpkgs"
	}
	return ""
}

// Rev is the locked commit. A tarball lock may carry it only as `rev=` in the locked URL.
func (n Node) Rev() string {
	if n.Locked.Rev != "" {
		return n.Locked.Rev
	}
	if u, err := url.Parse(n.Locked.URL); err == nil {
		return u.Query().Get("rev")
	}
	return ""
}

type Lock struct {
	Root  string          `json:"root"`
	Nodes map[string]Node `json:"nodes"`
}

func ReadLock(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseLock(data)
}

func ParseLock(data []byte) (*Lock, error) {
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("flake.lock: %w", err)
	}
	return &l, nil
}

// Direct returns the flake's own inputs as name -> node key. `follows` lists are skipped.
func (l *Lock) Direct() map[string]string {
	out := map[string]string{}
	for name, raw := range l.Nodes[l.Root].Inputs {
		var key string
		if json.Unmarshal(raw, &key) == nil {
			out[name] = key
		}
	}
	return out
}

// Names are the direct input names, sorted.
func (l *Lock) Names() []string {
	var n []string
	for k := range l.Direct() {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// Input returns a direct input by name.
func (l *Lock) Input(name string) (Node, bool) {
	key, ok := l.Direct()[name]
	return l.Nodes[key], ok
}

// ShortRev abbreviates a commit for display.
func ShortRev(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

// Change is a direct input whose revision moved.
type Change struct {
	Name, Repo, Ref, Old, New string
}

// Diff lists direct inputs whose revision moved, and how many other nodes moved.
func Diff(old, cur *Lock) ([]Change, int) {
	od, cd := old.Direct(), cur.Direct()
	isDirect := map[string]bool{}
	var out []Change
	for _, name := range cur.Names() {
		node := cur.Nodes[cd[name]]
		isDirect[cd[name]] = true
		if o := old.Nodes[od[name]]; o.Rev() != node.Rev() {
			c := Change{Name: name, Ref: node.Original.Ref, Old: o.Rev(), New: node.Rev(), Repo: node.Repo()}
			out = append(out, c)
		}
	}
	transitive := 0
	for key, node := range cur.Nodes {
		if key != cur.Root && !isDirect[key] && node.Rev() != old.Nodes[key].Rev() {
			transitive++
		}
	}
	return out, transitive
}
