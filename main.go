// nixup: manual update checker and safe updater for a flake-based NixOS system.
// It never activates anything: no switch, no boot, no sudo, no daemon.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

var version = "dev"

const usage = `nixup — update checker and safe updater for flake-based NixOS

  nixup [check]   what is outdated, read-only (inputs, packages, profile, flatpak)
  nixup lock      update flake.lock in a scratch copy, show changes, build, ask, apply
  nixup pkg       update own packages/ one by one after reviewing upstream changes
  nixup version

Common flags: -C <dir> flake directory (default: search upwards from cwd)
Run 'nixup <command> -h' for the flags of each command.`

var sections = []string{"inputs", "packages", "profile", "flatpak"}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	cmd := "check"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "check":
		return cmdCheck(args)
	case "lock":
		return cmdLock(args)
	case "pkg":
		return cmdPkg(args)
	case "version":
		fmt.Println("nixup", version)
		return 0
	case "help":
		fmt.Println(usage)
		return 0
	}
	fmt.Fprintln(os.Stderr, usage)
	return 2
}

func newFlags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	dir := fs.String("C", ".", "flake directory")
	return fs, dir
}

// parse accepts flags before, between and after positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func cmdCheck(args []string) int {
	fs, dir := newFlags("check")
	only := fs.String("only", "", "comma list of: "+strings.Join(sections, ", "))
	all := fs.Bool("all", false, "also list up-to-date entries")
	asJSON := fs.Bool("json", false, "machine-readable output")
	md := fs.Bool("markdown", false, "Markdown output (PR body, CI summary)")
	review := fs.Bool("review", false, "include upstream changes for outdated entries")
	if fs.Parse(args) != nil {
		return 2
	}
	repo, err := findRepo(*dir)
	if err != nil {
		return fail(err)
	}
	want := map[string]bool{}
	for _, s := range strings.Split(*only, ",") {
		if s != "" {
			want[s] = true
		}
	}
	for s := range want {
		if !contains(sections, s) {
			return fail(fmt.Errorf("unknown section %q", s))
		}
	}
	jobs := map[string]func() []Row{
		"inputs":   func() []Row { return checkInputs(repo, *review) },
		"packages": func() []Row { return checkPackages(repo, *review) },
		"profile":  func() []Row { return checkProfile(repo) },
		"flatpak":  checkFlatpak,
	}
	var names []string
	for _, s := range sections {
		if len(want) == 0 || want[s] {
			names = append(names, s)
		}
	}
	groups := parallel(len(names), func(i int) []Row { return jobs[names[i]]() })
	var rows []Row
	for _, g := range groups {
		rows = append(rows, g...)
	}

	switch {
	case *asJSON:
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(b))
	case *md:
		fmt.Print(renderMarkdown(rows))
	default:
		renderText(rows, *all, *review)
	}
	code := 0
	for _, r := range rows {
		if r.Status == stError {
			return 2
		}
		if r.Status == stOutdated {
			code = 1
		}
	}
	return code
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

var (
	colors = map[string]int{stOK: 32, stOutdated: 33, stInfo: 36, stError: 31}
	marks  = map[string]string{stOK: "ok", stOutdated: "UPDATE", stInfo: "info", stError: "ERROR"}
)

func renderText(rows []Row, all, review bool) {
	for _, sec := range sections {
		var group []Row
		for _, r := range rows {
			if r.Section == sec {
				group = append(group, r)
			}
		}
		if len(group) == 0 {
			continue
		}
		fmt.Println("\n" + bold(sec))
		hidden := 0
		for _, r := range group {
			if r.Status == stOK && !all {
				hidden++
				continue
			}
			ver := r.Current
			if r.Latest != "" {
				ver += " -> " + r.Latest
			}
			fmt.Printf("  %s %-26s %-30s %s\n", paint(fmt.Sprintf("%-6s", marks[r.Status]), colors[r.Status]), r.Name, ver, r.Note)
			if r.Warn != "" {
				fmt.Println("         " + paint("⚠ "+r.Warn, 33))
			}
			if review && r.Review != nil {
				fmt.Print(r.Review.Text("         "))
			}
		}
		if hidden > 0 {
			fmt.Printf("  %d up to date (use --all to list)\n", hidden)
		}
	}
}

func renderMarkdown(rows []Row) string {
	var b strings.Builder
	n := 0
	for _, r := range rows {
		if r.Status != stOutdated && r.Status != stError {
			continue
		}
		n++
		ver := r.Current
		if r.Latest != "" {
			ver += " → " + r.Latest
		}
		fmt.Fprintf(&b, "### %s / %s `%s`\n\n", r.Section, r.Name, ver)
		if r.Status == stError {
			fmt.Fprintf(&b, "Error: %s\n\n", r.Note)
		}
		if r.Warn != "" {
			fmt.Fprintf(&b, "> ⚠ %s\n\n", r.Warn)
		}
		if r.Review != nil {
			b.WriteString(r.Review.Markdown() + "\n")
		}
	}
	if n == 0 {
		return "Everything is up to date.\n"
	}
	return b.String()
}

func cmdLock(args []string) int {
	fs, dir := newFlags("lock")
	var hs multi
	fs.Var(&hs, "H", "host to build (repeatable; default: all nixosConfigurations)")
	noBuild := fs.Bool("no-build", false, "only update and show the changes")
	yes := fs.Bool("y", false, "apply without asking (CI)")
	report := fs.String("report", "", "append a Markdown report to this file")
	pos, perr := parse(fs, args)
	if perr != nil {
		return 2
	}
	repo, err := findRepo(*dir)
	if err != nil {
		return fail(err)
	}
	return runLock(lockOpts{repo: repo, inputs: pos, hosts: hs, noBuild: *noBuild, yes: *yes, report: *report})
}

func cmdPkg(args []string) int {
	fs, dir := newFlags("pkg")
	noBuild := fs.Bool("no-build", false, "do not build the package after updating")
	yes := fs.Bool("y", false, "apply without asking (CI)")
	report := fs.String("report", "", "append a Markdown report to this file")
	pos, perr := parse(fs, args)
	if perr != nil {
		return 2
	}
	repo, err := findRepo(*dir)
	if err != nil {
		return fail(err)
	}
	return runPkg(pkgOpts{repo: repo, names: pos, noBuild: *noBuild, yes: *yes, report: *report})
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }
