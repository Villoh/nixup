// Package update is `nixup update`: flake.lock (Lock) and own packages (Pkg), which only
// edit the repository, and Flatpak and nix profile, which install right away. Every step
// shows what changes first, asks, and says how to undo it; none activates a NixOS
// generation.
package update

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ui"
)

type Updater struct {
	R     run.Runner
	GH    *gh.Client
	Check *check.Checker
	T     *ui.Term

	edited bool // flake.lock or a package was written: there is something to apply with nh
}

func (u *Updater) fail(err error) int {
	u.T.Errorln(u.T.Paint("nixup: "+err.Error(), 31))
	return exit.Error
}

// appendReport adds Markdown to the report file; a missing path means no report.
func (u *Updater) appendReport(path, text string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		u.T.Errorln("nixup: report:", err)
		return
	}
	defer f.Close()
	fmt.Fprintln(f, text)
}

// Sources are what update works on, in the order the steps run: inputs first, so
// packages build against the new lock.
var Sources = []string{"input", "package", "flatpak", "profile"}

var nouns = map[string][2]string{ // with an article, bare
	"input":   {"an input", "input"},
	"package": {"a package", "package"},
	"flatpak": {"a Flatpak app", "Flatpak app"},
	"profile": {"a nix profile entry", "nix profile entry"},
}

// Opts are what `nixup update` takes.
type Opts struct {
	Repo       string
	Names      []string // inputs, packages, Flatpak app IDs, profile entries, mixed; empty: everything
	Sources    []string // some of Sources; empty: all
	Hosts      []string // for the inputs; empty: every nixosConfiguration
	NoBuild    bool
	Yes        bool
	AllowDirty bool
	Report     string
}

// Update runs one step per source: the inputs (Lock: one scratch copy, every host built),
// the own packages (Pkg: one by one), Flatpak (one question, then flatpak update) and
// nix profile (one question, then nix profile upgrade). Each name is looked up in every
// source and sent to its step; steps without names are skipped when names are given.
// The result is the worst code of the steps.
func (u *Updater) Update(o Opts) int {
	on := func(s string) bool { return len(o.Sources) == 0 || slices.Contains(o.Sources, s) }
	named := map[string][]string{}
	if len(o.Names) > 0 {
		known, err := u.known(o.Repo, on)
		if err != nil {
			return u.fail(err)
		}
		for _, n := range o.Names {
			var in, out []string
			for _, s := range Sources {
				switch {
				case known[s][n] && on(s):
					in = append(in, s)
				case known[s][n]:
					out = append(out, s)
				}
			}
			switch {
			case len(in) == 1:
				named[in[0]] = append(named[in[0]], n)
			case len(in) > 1:
				var flags []string
				for _, s := range in {
					flags = append(flags, "--source "+s)
				}
				return u.fail(fmt.Errorf("%s is %s%s: pick one with %s", n, both(in), list(in, 0, "and"), list(flags, -1, "or")))
			case len(out) > 0:
				return u.fail(fmt.Errorf("%s is %s, which --source leaves out", n, list(out, 0, "and")))
			default:
				var enabled []string
				for _, s := range Sources {
					if on(s) {
						enabled = append(enabled, s)
					}
				}
				return u.fail(fmt.Errorf("no %s called %s", list(enabled, 1, "or"), n))
			}
		}
	}
	code, ran := exit.OK, false
	for _, s := range Sources {
		if !on(s) || (len(o.Names) > 0 && len(named[s]) == 0) ||
			(s == "flatpak" && !u.R.Has("flatpak")) || (s == "profile" && !u.R.Has("nix")) {
			continue
		}
		if ran {
			u.T.Println()
		}
		ran = true
		switch s {
		case "input":
			code = max(code, u.Lock(LockOpts{Repo: o.Repo, Inputs: named[s], Hosts: o.Hosts, NoBuild: o.NoBuild, Yes: o.Yes, AllowDirty: o.AllowDirty, Report: o.Report}))
		case "package":
			code = max(code, u.Pkg(PkgOpts{Repo: o.Repo, Names: named[s], NoBuild: o.NoBuild, Yes: o.Yes, AllowDirty: o.AllowDirty, Report: o.Report}))
		case "flatpak":
			code = max(code, u.Flatpak(FlatpakOpts{IDs: named[s], Yes: o.Yes, Report: o.Report}))
		case "profile":
			code = max(code, u.Profile(ProfileOpts{Names: named[s], Yes: o.Yes, Report: o.Report}))
		}
	}
	if u.edited {
		u.T.Println("\nNext: review with git diff, then nh os test " + u.target(o) + " (nothing is committed or activated)")
	}
	return code
}

// target is how nh os test reaches the configuration just edited: "." from inside the
// repository, else its path, and the host when there is only one.
func (u *Updater) target(o Opts) string {
	dir := o.Repo
	if wd, err := os.Getwd(); err == nil && wd == o.Repo {
		dir = "."
	} else if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(dir, home+"/") {
		dir = "~" + strings.TrimPrefix(dir, home)
	}
	hosts := o.Hosts
	if len(hosts) == 0 {
		hosts, _ = flake.Hosts(u.R, o.Repo)
	}
	if len(hosts) == 1 {
		return dir + " -H " + hosts[0]
	}
	return dir + " -H <host>"
}

// known is every name each source has. Packages are only evaluated when their step is
// on; the other sources are cheap local reads.
func (u *Updater) known(repo string, on func(string) bool) (map[string]map[string]bool, error) {
	known := map[string]map[string]bool{"input": {}, "package": {}, "flatpak": u.Check.FlatpakInstalled(), "profile": {}}
	lock, err := flake.ReadLock(filepath.Join(repo, "flake.lock"))
	if err != nil {
		return nil, err
	}
	for n := range lock.Direct() {
		known["input"][n] = true
	}
	if on("package") {
		pkgs, err := flake.Packages(u.R, repo)
		if err != nil {
			return nil, err
		}
		for _, p := range pkgs {
			known["package"][p.Name] = true
		}
	}
	if entries, err := u.Check.ProfileEntries(); err == nil {
		for _, e := range entries {
			known["profile"][e.Name] = true
		}
	}
	return known, nil
}

func both(s []string) string {
	if len(s) == 2 {
		return "both "
	}
	return ""
}

// list joins sources (form 0: with an article, 1: bare) or plain strings (form -1) as
// "a, b and c".
func list(items []string, form int, conj string) string {
	words := make([]string, len(items))
	for i, it := range items {
		words[i] = it
		if form >= 0 {
			words[i] = nouns[it][form]
		}
	}
	if len(words) == 1 {
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " " + conj + " " + words[len(words)-1]
}
