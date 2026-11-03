// Package cli parses the command line and wires the other packages together.
package cli

import (
	"errors"
	"flag"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ui"
	"github.com/Villoh/nixup/internal/update"
)

// Version is set at build time with -ldflags "-X .../internal/cli.Version=...".
var Version = "dev"

const usage = `nixup — update checker and safe updater for flake-based NixOS

  nixup [check]   what is outdated, read-only (inputs, packages, profile, flatpak)
  nixup lock      update flake.lock in a scratch copy, show changes, build, ask, apply
  nixup pkg       update own packages/ one by one after reviewing upstream changes
  nixup version

Common flags: -C <dir> flake directory (default: search upwards from cwd)
Run 'nixup <command> -h' for the flags of each command.`

// Deps are the things commands act through; tests replace them.
type Deps struct {
	T  *ui.Term
	R  run.Runner
	GH *gh.Client

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
	case "version":
		d.T.Println("nixup", Version)
		return exit.OK
	case "help":
		d.T.Println(usage)
		return exit.OK
	}
	d.T.Errorln(usage)
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
	return fs, fs.String("C", ".", "flake directory")
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
	repo, err = flake.FindRepo(*dir)
	if err != nil {
		return nil, "", d.fail(err), false
	}
	return pos, repo, exit.OK, true
}
