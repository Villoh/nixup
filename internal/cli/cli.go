// Package cli parses the command line and wires the other packages together.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ui"
	"github.com/Villoh/nixup/internal/update"
)

// Version and Commit are set at build time with -ldflags "-X .../internal/cli.Version=...".
var (
	Version = "dev"
	Commit  = ""
)

func versionString() string {
	if Commit == "" {
		return "nixup " + Version
	}
	return "nixup " + Version + " (" + Commit + ")"
}

const usage = `nixup — update checker and safe updater for flake-based NixOS

  nixup [check]   what is outdated, read-only (inputs, packages, profile, flatpak)
  nixup lock      update flake.lock in a scratch copy, show changes, build, ask, apply
  nixup pkg       update own packages/ one by one after reviewing upstream changes
  nixup list      everything it watches, with current versions, without network
  nixup search    look a package name up in nixpkgs, your packages, your inputs and Flatpak
  nixup version

Common flags: -C <dir> flake directory. Without it: $NIXUP_FLAKE, $NH_OS_FLAKE, $NH_FLAKE,
              then a search upwards from the current directory.
Run 'nixup <command> -h' for the flags of each command.`

// Deps are the things commands act through; tests replace them.
type Deps struct {
	T  *ui.Term
	R  run.Runner
	GH *gh.Client

	// Env reads environment variables; nil means the process environment.
	Env func(string) string

	implicit bool // the subcommand was not typed: `check` was assumed
}

// Run executes one nixup invocation and returns its exit code.
func Run(args []string, d Deps) int {
	cmd, origArgs := "check", args
	if len(args) > 0 {
		switch args[0] {
		case "-version", "--version", "-v":
			args[0] = "version"
		case "-h", "-help", "--help":
			args[0] = "help"
		}
		if !strings.HasPrefix(args[0], "-") {
			cmd, args = args[0], args[1:]
		}
	}
	d.implicit = len(args) == len(origArgs)
	switch cmd {
	case "check":
		return runCheck(args, d)
	case "lock":
		return runLock(args, d)
	case "pkg":
		return runPkg(args, d)
	case "list":
		return runList(args, d)
	case "search":
		return runSearch(args, d)
	case "version":
		d.T.Println(versionString())
		return exit.OK
	case "help":
		d.T.Println(usage)
		return exit.OK
	}
	d.T.Errorln(d.T.Paint("nixup: unknown command "+strconv.Quote(cmd), 31) + "\n\n" + usage)
	return exit.Error
}

func (d Deps) fail(err error) int {
	d.T.Errorln(d.T.Paint("nixup: "+err.Error(), 31))
	return exit.Error
}

func (d Deps) updater() *update.Updater {
	return &update.Updater{R: d.R, GH: d.GH, Check: &check.Checker{R: d.R, GH: d.GH}, T: d.T}
}

// flags creates a flag set with the shared -C flag.
func (d Deps) flags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(d.T.Err)
	return fs, fs.String("C", "", "flake directory (default: $NIXUP_FLAKE, $NH_OS_FLAKE, $NH_FLAKE, else search upwards from here)")
}

// parse accepts flags before, between and after positional arguments (Go's flag package
// stops at the first positional one). The second result is -h: the caller exits 0.
func parse(fs *flag.FlagSet, args []string) (pos []string, help bool, err error) {
	for {
		if err := fs.Parse(args); err != nil {
			return nil, errors.Is(err, flag.ErrHelp), err
		}
		if fs.NArg() == 0 {
			return pos, false, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// setup parses flags and locates the flake. ok is false when the command should exit with code.
func (d Deps) setup(fs *flag.FlagSet, dir *string, args []string) (pos []string, repo string, code int, ok bool) {
	pos, help, err := parse(fs, args)
	switch {
	case help:
		return nil, "", exit.OK, false
	case err != nil:
		if d.implicit {
			d.T.Errorln("nixup: no command given, 'check' was assumed (see 'nixup help' for the others)")
		}
		return nil, "", exit.Error, false
	}
	getenv := d.Env
	if getenv == nil {
		getenv = os.Getenv
	}
	repo, source, err := flake.Resolve(*dir, getenv)
	if err != nil {
		return nil, "", d.fail(err), false
	}
	if strings.HasPrefix(source, "$") { // the user did not point here in this command
		if here, ok := otherFlake(repo); ok {
			d.T.Errorln(fmt.Sprintf("nixup: using %s (%s), not the flake in %s; pass -C . for that one", repo, source, here))
		} else if fs.Name() == "lock" || fs.Name() == "pkg" {
			d.T.Errorln("nixup: using", repo, "("+source+")") // about to write there
		}
	}
	return pos, repo, exit.OK, true
}

// otherFlake reports the flake around the current directory when it is not repo: the
// case where an environment variable silently wins over the directory you are in.
func otherFlake(repo string) (string, bool) {
	here, err := flake.FindRepo(".")
	if err != nil {
		return "", false
	}
	real := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	return here, real(here) != real(repo)
}
