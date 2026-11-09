package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/inventory"
	"github.com/Villoh/nixup/internal/report"
	"github.com/Villoh/nixup/internal/update"
)

func runCheck(args []string, d Deps) int {
	fs, dir := d.flags("check")
	only := fs.String("only", "", "comma list of: "+strings.Join(check.Sections, ", "))
	all := fs.Bool("all", false, "also list up-to-date entries")
	asJSON := fs.Bool("json", false, "machine-readable output")
	md := fs.Bool("markdown", false, "Markdown output (PR body, CI summary)")
	review := fs.Bool("review", false, "include upstream changes for outdated entries")
	_, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}

	var sections []string
	for _, s := range strings.Split(*only, ",") {
		if s == "" {
			continue
		}
		if !contains(check.Sections, s) {
			return d.fail(fmt.Errorf("unknown section %q", s))
		}
		sections = append(sections, s)
	}

	rows := (&check.Checker{R: d.R, GH: d.GH}).All(repo, sections, *review)
	switch {
	case *asJSON:
		b, _ := json.MarshalIndent(rows, "", "  ")
		d.T.Println(string(b))
	case *md:
		d.T.Printf("%s", report.Markdown(rows))
	default:
		report.Text(d.T, rows, *all, *review)
	}
	return check.ExitCode(rows)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// hostList is a repeatable -H flag.
type hostList []string

func (h *hostList) String() string     { return strings.Join(*h, ",") }
func (h *hostList) Set(v string) error { *h = append(*h, v); return nil }

func runLock(args []string, d Deps) int {
	fs, dir := d.flags("lock")
	var hosts hostList
	fs.Var(&hosts, "H", "host to build (repeatable; default: all nixosConfigurations)")
	noBuild := fs.Bool("no-build", false, "only update and show the changes")
	yes := fs.Bool("y", false, "apply without asking (CI)")
	reportFile := fs.String("report", "", "append a Markdown report to this file")
	inputs, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}
	return d.updater().Lock(update.LockOpts{Repo: repo, Inputs: inputs, Hosts: hosts,
		NoBuild: *noBuild, Yes: *yes, Report: *reportFile})
}

func runPkg(args []string, d Deps) int {
	fs, dir := d.flags("pkg")
	noBuild := fs.Bool("no-build", false, "do not build the package after updating")
	yes := fs.Bool("y", false, "apply without asking (CI)")
	reportFile := fs.String("report", "", "append a Markdown report to this file")
	names, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}
	return d.updater().Pkg(update.PkgOpts{Repo: repo, Names: names,
		NoBuild: *noBuild, Yes: *yes, Report: *reportFile})
}

func printGroups(d Deps, groups []inventory.Group, asJSON bool) {
	if asJSON {
		b, _ := json.MarshalIndent(groups, "", "  ")
		d.T.Println(string(b))
		return
	}
	report.Inventory(d.T, groups)
}

func runList(args []string, d Deps) int {
	fs, dir := d.flags("list")
	asJSON := fs.Bool("json", false, "machine-readable output")
	_, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}
	groups := inventory.List(d.R, repo)
	printGroups(d, groups, *asJSON)
	for _, g := range groups {
		if g.Err != "" {
			return exit.Error
		}
	}
	return exit.OK
}

func runSearch(args []string, d Deps) int {
	fs, dir := d.flags("search")
	asJSON := fs.Bool("json", false, "machine-readable output")
	limit := fs.Int("limit", 10, "results per source")
	words, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}
	if len(words) != 1 {
		return d.fail(fmt.Errorf("usage: nixup search <name>"))
	}
	groups, err := inventory.Search(d.R, repo, words[0], max(*limit, 1))
	if err != nil {
		return d.fail(err)
	}
	if len(groups) == 0 && !*asJSON {
		d.T.Println("Nothing called " + strconv.Quote(words[0]) + " in nixpkgs, your packages, your inputs or Flatpak.")
		return exit.Pending
	}
	printGroups(d, groups, *asJSON)
	if len(groups) == 0 {
		return exit.Pending
	}
	return exit.OK
}
