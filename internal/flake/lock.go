// Package flake reads what nixup needs from a flake: flake.lock, the repo layout,
// the hosts, and the packages defined in the flake's own `packages` output.
package flake

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type Node struct {
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
		if o := old.Nodes[od[name]]; o.Locked.Rev != node.Locked.Rev {
			c := Change{Name: name, Ref: node.Original.Ref, Old: o.Locked.Rev, New: node.Locked.Rev}
			if node.Original.Type == "github" {
				c.Repo = node.Original.Owner + "/" + node.Original.Repo
			}
			out = append(out, c)
		}
	}
	transitive := 0
	for key, node := range cur.Nodes {
		if key != cur.Root && !isDirect[key] && node.Locked.Rev != old.Nodes[key].Locked.Rev {
			transitive++
		}
	}
	return out, transitive
}
