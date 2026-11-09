// Package update changes things on disk: flake.lock (Lock) and own packages (Pkg), both
// driven by Update. They show what changes upstream first, ask, work on a copy or
// restore on failure, and never activate a system generation.
package update

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

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

// Opts are what `nixup update` takes.
type Opts struct {
	Repo    string
	Names   []string // inputs and packages, mixed; empty: everything
	Sources []string // "input", "package"; empty: both
	Hosts   []string // for the inputs; empty: every nixosConfiguration
	NoBuild bool
	Yes     bool
	Report  string
}

// Update brings the flake's inputs (Lock: one scratch copy, every host built) and then its
// own packages (Pkg: one by one) up to date. Each name is looked up both as a direct input
// of flake.lock and as a package of the flake. Inputs go first, so packages are built
// against the new lock. The result is the worst code of the two steps.
func (u *Updater) Update(o Opts) int {
	doInputs := len(o.Sources) == 0 || slices.Contains(o.Sources, "input")
	doPkgs := len(o.Sources) == 0 || slices.Contains(o.Sources, "package")
	var inputs, pkgs []string
	if len(o.Names) > 0 {
		lock, err := flake.ReadLock(filepath.Join(o.Repo, "flake.lock"))
		if err != nil {
			return u.fail(err)
		}
		isPkg := map[string]bool{}
		if doPkgs {
			all, err := flake.Packages(u.R, o.Repo)
			if err != nil {
				return u.fail(err)
			}
			for _, p := range all {
				isPkg[p.Name] = true
			}
		}
		direct := lock.Direct()
		for _, n := range o.Names {
			_, isInput := direct[n]
			switch {
			case isInput && doInputs && isPkg[n]:
				return u.fail(fmt.Errorf("%s is both an input and a package: pick one with --source input or --source package", n))
			case isInput && doInputs:
				inputs = append(inputs, n)
			case isPkg[n]:
				pkgs = append(pkgs, n)
			case isInput: // left out by --source package
				return u.fail(fmt.Errorf("%s is an input, not a package (drop --source package)", n))
			case !doPkgs: // left out by --source input; whether it is a package was not looked up
				return u.fail(fmt.Errorf("no input called %s", n))
			default:
				return u.fail(fmt.Errorf("no input or package called %s", n))
			}
		}
		doInputs, doPkgs = len(inputs) > 0, len(pkgs) > 0
	}
	code := exit.OK
	if doInputs {
		code = u.Lock(LockOpts{Repo: o.Repo, Inputs: inputs, Hosts: o.Hosts, NoBuild: o.NoBuild, Yes: o.Yes, Report: o.Report})
	}
	if doPkgs {
		if doInputs {
			u.T.Println()
		}
		code = max(code, u.Pkg(PkgOpts{Repo: o.Repo, Names: pkgs, NoBuild: o.NoBuild, Yes: o.Yes, Report: o.Report}))
	}
	return code
}
