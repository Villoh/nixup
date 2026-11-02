package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type lockOpts struct {
	repo    string
	inputs  []string
	hosts   []string
	noBuild bool
	yes     bool
	report  string
}

// scratchCopy copies the working tree (dirty and untracked files included, ignored ones
// not) so that updating and building never touch the real checkout.
func scratchCopy(repo, dst string) error {
	out, errs, err := capture(repo, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return fmt.Errorf("git ls-files: %s", lastLine(errs))
	}
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		src := filepath.Join(repo, rel)
		st, err := os.Lstat(src)
		if err != nil || !st.Mode().IsRegular() {
			continue // deleted in the working tree, or a symlink
		}
		if err := copyFile(src, filepath.Join(dst, rel), st.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func dirty(repo string, paths ...string) bool {
	_, _, err := capture(repo, "git", append([]string{"diff", "--quiet", "--"}, paths...)...)
	return err != nil
}

func runLock(o lockOpts) int {
	lockPath := filepath.Join(o.repo, "flake.lock")
	if dirty(o.repo, "flake.lock") {
		fmt.Fprintln(os.Stderr, paint("flake.lock has uncommitted changes: commit or stash it, then re-run.", 33))
		return 1
	}
	oldData, err := os.ReadFile(lockPath)
	if err != nil {
		return fail(err)
	}
	old, err := parseLock(oldData)
	if err != nil {
		return fail(err)
	}
	tmp, err := os.MkdirTemp("", "nixup-")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(tmp)
	if err := scratchCopy(o.repo, tmp); err != nil {
		return fail(err)
	}

	what := "(all inputs)"
	if len(o.inputs) > 0 {
		what = strings.Join(o.inputs, " ")
	}
	fmt.Println(bold("== nix flake update " + what))
	if err := stream("", "nix", append([]string{"flake", "update", "--flake", tmp}, o.inputs...)...); err != nil {
		return fail(err)
	}
	newData, err := os.ReadFile(filepath.Join(tmp, "flake.lock"))
	if err != nil {
		return fail(err)
	}
	if string(newData) == string(oldData) {
		fmt.Println("flake.lock is already up to date.")
		return 0
	}
	cur, err := parseLock(newData)
	if err != nil {
		return fail(err)
	}

	changes, trans := diffLocks(old, cur)
	var md strings.Builder
	fmt.Fprintf(&md, "### flake.lock\n\n")
	reviews := parallel(len(changes), func(i int) *Review {
		c := changes[i]
		if c.Repo == "" {
			return nil
		}
		return buildReview(c.Repo, c.Old, c.New, false)
	})
	for i, c := range changes {
		fmt.Printf("\n%s  %s -> %s\n", bold(c.Name), short(c.Old), short(c.New))
		fmt.Fprintf(&md, "#### %s `%s` → `%s`\n\n", c.Name, short(c.Old), short(c.New))
		if r := reviews[i]; r != nil {
			fmt.Print(r.Text("  "))
			md.WriteString(r.Markdown() + "\n")
		}
	}
	fmt.Printf("\n(+ %d transitive inputs changed)\n", trans)
	fmt.Fprintf(&md, "%d inputs changed, %d transitive.\n", len(changes), trans)

	if !o.noBuild {
		hs := o.hosts
		if len(hs) == 0 {
			if hs, err = hosts(tmp); err != nil {
				return fail(err)
			}
		}
		for _, h := range hs {
			fmt.Println(bold("\n== nh os build -H " + h + " (scratch copy, nothing is activated)"))
			if err := stream("", "nh", "os", "build", tmp, "-H", h, "-o", filepath.Join(tmp, "result")); err != nil {
				fmt.Println(paint("build of "+h+" FAILED: flake.lock left untouched.", 31))
				fmt.Fprintf(&md, "\n**Build of `%s` failed.**\n", h)
				writeReport(o.report, md.String())
				return 1
			}
			fmt.Fprintf(&md, "\nBuild of `%s`: ok.\n", h)
		}
	}
	writeReport(o.report, md.String())

	if !o.yes && !confirm("\nWrite this flake.lock to the repo?") {
		fmt.Println("Discarded. Nothing changed.")
		return 0
	}
	if err := os.WriteFile(lockPath, newData, 0o644); err != nil {
		return fail(err)
	}
	fmt.Println(paint("flake.lock updated (not committed, nothing activated).", 32))
	fmt.Println("Next: git diff flake.lock && nix flake check && nh os test . -H <host>")
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, paint("nixup: "+err.Error(), 31))
	return 2
}

func writeReport(path, text string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nixup: report:", err)
		return
	}
	defer f.Close()
	fmt.Fprintln(f, text)
}
