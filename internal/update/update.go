// Package update changes things on disk: flake.lock (Lock) and own packages (Pkg).
// Both show what changes upstream first, ask, work on a copy or restore on failure, and
// never activate a system generation.
package update

import (
	"fmt"
	"os"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
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
