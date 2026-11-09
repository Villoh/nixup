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
	only := fs.String("source", "", "comma list of: input, package, profile, flatpak (default: all)")
	all := fs.Bool("all", false, "also list up-to-date entries")
	asJSON := fs.Bool("json", false, "machine-readable output")
	md := fs.Bool("markdown", false, "Markdown output (PR body, CI summary)")
	review := fs.Bool("review", false, "include upstream changes for outdated entries")
	_, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}

	sources, err := parseSources(*only, []string{"input", "package", "profile", "flatpak"})
	if err != nil {
		return d.fail(err)
	}
	var sections []string // check names its sections in the plural
	for _, s := range sources {
		for _, sec := range check.Sections {
			if check.Source(sec) == s {
				sections = append(sections, sec)
			}
		}
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

// parseSources reads a --source list. Plurals are accepted too (inputs, packages).
func parseSources(list string, allowed []string) ([]string, error) {
	var out []string
	for _, s := range strings.Split(list, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !contains(allowed, s) && contains(allowed, strings.TrimSuffix(s, "s")) {
			s = strings.TrimSuffix(s, "s")
		}
		if !contains(allowed, s) {
			return nil, fmt.Errorf("unknown source %q (one of: %s)", s, strings.Join(allowed, ", "))
		}
		out = append(out, s)
	}
	return out, nil
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

func runUpdate(args []string, d Deps) int {
	fs, dir := d.flags("update")
	source := fs.String("source", "", "comma list of: "+strings.Join(update.Sources, ", ")+" (default: all)")
	var hosts hostList
	fs.Var(&hosts, "H", "host to build for inputs (repeatable; default: all nixosConfigurations)")
	noBuild := fs.Bool("no-build", false, "do not build: only update and show the changes")
	yes := fs.Bool("y", false, "apply without asking (CI)")
	reportFile := fs.String("report", "", "append a Markdown report to this file")
	names, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}
	sources, err := parseSources(*source, update.Sources)
	if err != nil {
		return d.fail(err)
	}
	return d.updater().Update(update.Opts{Repo: repo, Names: names, Sources: sources, Hosts: hosts,
		NoBuild: *noBuild, Yes: *yes, Report: *reportFile})
}

func printJSON(d Deps, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	d.T.Println(string(b))
}

func runList(args []string, d Deps) int {
	fs, dir := d.flags("list")
	asJSON := fs.Bool("json", false, "machine-readable output")
	source := fs.String("source", "", "comma list of: "+strings.Join(inventory.ListSources, ", ")+" (default: all)")
	_, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}
	sources, err := parseSources(*source, inventory.ListSources)
	if err != nil {
		return d.fail(err)
	}
	groups := inventory.List(d.R, repo, sources)
	if *asJSON {
		printJSON(d, groups)
	} else {
		report.List(d.T, groups)
	}
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
	source := fs.String("source", "", "comma list of: "+strings.Join(inventory.SearchSources, ", ")+" (default: all)")
	words, repo, code, ok := d.setup(fs, dir, args)
	if !ok {
		return code
	}
	if len(words) != 1 {
		return d.fail(fmt.Errorf("usage: nixup search <name>"))
	}
	sources, err := parseSources(*source, inventory.SearchSources)
	if err != nil {
		return d.fail(err)
	}
	groups, err := inventory.Search(d.R, repo, words[0], max(*limit, 1), sources)
	if err != nil {
		return d.fail(err)
	}
	if len(groups) == 0 && !*asJSON {
		d.T.Println("Nothing called " + strconv.Quote(words[0]) + " found.")
		return exit.Pending
	}
	if *asJSON {
		printJSON(d, groups)
	} else {
		report.Search(d.T, groups)
	}
	if len(groups) == 0 {
		return exit.Pending
	}
	return exit.OK
}
