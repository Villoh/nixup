package main

import (
	"fmt"
	"os"
	"strings"
)

type pkgOpts struct {
	repo    string
	names   []string
	noBuild bool
	yes     bool
	report  string
}

func nixUpdateCmd() []string {
	if have("nix-update") {
		return []string{"nix-update"}
	}
	return []string{"nix", "run", "nixpkgs#nix-update", "--"}
}

// selectPackages returns the requested packages (all when names is empty) or an error
// naming the first one that does not exist.
func selectPackages(pkgs []ownPackage, names []string) ([]ownPackage, error) {
	if len(names) == 0 {
		return pkgs, nil
	}
	byName := map[string]ownPackage{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	var out []ownPackage
	for _, n := range names {
		p, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("no such package: %s", n)
		}
		out = append(out, p)
	}
	return out, nil
}

// runPkg shows what changes upstream for each outdated own package, asks, then runs
// nix-update pinned to the reviewed version and builds the result. A failed build puts
// the package files back.
func runPkg(o pkgOpts) int {
	pkgs, err := ownPackages(o.repo)
	if err != nil {
		return fail(err)
	}
	todo, err := selectPackages(pkgs, o.names)
	if err != nil {
		return fail(err)
	}

	code := 0
	for _, p := range todo {
		row := checkPackage(p, true)
		switch row.Status {
		case stError:
			fmt.Printf("%s %s: %s\n", paint("ERROR", 31), p.Name, row.Note)
			code = 2
			continue
		case stOK, stInfo:
			if len(o.names) > 0 || row.Status == stInfo {
				fmt.Printf("%s %s %s %s\n", paint(strings.ToUpper(row.Status), 32), p.Name, row.Current, row.Note)
			}
			continue
		}
		if c := updateOne(o, p, row); c > code {
			code = c
		}
	}
	return code
}

func updateOne(o pkgOpts, p ownPackage, row Row) int {
	fmt.Printf("\n%s  %s -> %s\n", bold(p.Name), row.Current, row.Latest)
	if row.Warn != "" {
		fmt.Println("  " + paint("⚠ "+row.Warn, 33))
	}
	md := fmt.Sprintf("### %s `%s` → `%s`\n\n", p.Name, row.Current, row.Latest)
	if row.Warn != "" {
		md += "> ⚠ " + row.Warn + "\n\n"
	}
	if row.Review != nil {
		fmt.Print(row.Review.Text("  "))
		md += row.Review.Markdown()
	}
	if !o.yes && !confirm(fmt.Sprintf("Update %s to %s?", p.Name, row.Latest)) {
		fmt.Println("Skipped.")
		return 0
	}
	dir := "packages/" + p.Name
	if dirty(o.repo, dir) {
		fmt.Println(paint(dir+" has uncommitted changes: commit or stash them first.", 33))
		return 1
	}
	extra, _ := p.extraArgs()
	args := append(nixUpdateCmd(), "--flake", p.Name, "--version="+row.Latest)
	args = append(args, extra...)
	if err := stream(o.repo, args[0], args[1:]...); err != nil {
		restore(o.repo, dir)
		fmt.Println(paint("nix-update failed; "+dir+" restored.", 31))
		writeReport(o.report, md+"\n**nix-update failed.**\n")
		return 2
	}
	if !o.noBuild {
		fmt.Println(bold("== nix build .#" + p.Name))
		if err := stream(o.repo, "nix", "build", "--no-link", ".#"+p.Name); err != nil {
			restore(o.repo, dir)
			fmt.Println(paint("build FAILED; "+dir+" restored.", 31))
			writeReport(o.report, md+"\n**Build failed; not applied.**\n")
			return 2
		}
	}
	diff, _, _ := capture(o.repo, "git", "diff", "--", dir)
	fmt.Println(diff)
	fmt.Println(paint(p.Name+" updated (not committed).", 32))
	writeReport(o.report, md+"\nBuild: ok.\n")
	return 0
}

func restore(repo, dir string) {
	if _, errs, err := capture(repo, "git", "checkout", "--", dir); err != nil {
		fmt.Fprintln(os.Stderr, "nixup: restore failed:", lastLine(errs))
	}
}
