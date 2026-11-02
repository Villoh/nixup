// nixup: update checker and safe updater for flake-based NixOS.
// It never activates anything: no switch, no boot, no sudo, no daemon.
package main

import (
	"os"

	"github.com/Villoh/nixup/internal/cli"
	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ui"
)

func main() {
	r := run.Exec{Out: os.Stderr} // builds and nix-update stream to stderr
	os.Exit(cli.Run(os.Args[1:], cli.Deps{T: ui.Stdio(), R: r, GH: gh.New(r)}))
}
