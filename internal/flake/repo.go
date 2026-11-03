package flake

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Villoh/nixup/internal/run"
)

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// FindRepo walks upwards from dir until it finds flake.nix and flake.lock.
func FindRepo(dir string) (string, error) {
	p, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if exists(filepath.Join(p, "flake.nix")) && exists(filepath.Join(p, "flake.lock")) {
			return p, nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", fmt.Errorf("no flake.nix + flake.lock found from %s", dir)
		}
		p = parent
	}
}

// envVars are checked in order. NH_* are the ones nh itself reads, so a configured nh
// needs no extra setup here.
var envVars = []string{"NIXUP_FLAKE", "NH_OS_FLAKE", "NH_FLAKE"}

// Resolve finds the flake to work on. Precedence: the -C flag, then NIXUP_FLAKE,
// NH_OS_FLAKE and NH_FLAKE, then a search upwards from the current directory.
// source says where the answer came from: "-C", "$NIXUP_FLAKE", ... or "" for the search.
func Resolve(flagDir string, getenv func(string) string) (repo, source string, err error) {
	if flagDir != "" {
		repo, err = FindRepo(flagDir)
		return repo, "-C", err
	}
	for _, name := range envVars {
		v := getenv(name)
		if v == "" {
			continue
		}
		dir, err := localPath(v)
		if err != nil {
			return "", "", fmt.Errorf("$%s: %w", name, err)
		}
		// An explicitly configured path is used as is: never silently pick a parent directory.
		if !exists(filepath.Join(dir, "flake.nix")) || !exists(filepath.Join(dir, "flake.lock")) {
			return "", "", fmt.Errorf("$%s=%s: no flake.nix + flake.lock there", name, v)
		}
		return dir, "$" + name, nil
	}
	repo, err = FindRepo(".")
	if err != nil {
		err = fmt.Errorf("%w (run it inside the flake, use -C, or set NIXUP_FLAKE or NH_FLAKE)", err)
	}
	return repo, "", err
}

// localPath turns a value such as "~/nixos#desktop" or "path:/etc/nixos" into a directory.
// Remote references (github:, git+https:, ...) cannot be updated in place.
func localPath(v string) (string, error) {
	v, _, _ = strings.Cut(v, "#") // nh accepts "<flake>#<host>"
	v = strings.TrimPrefix(v, "path:")
	if m := schemeRe.FindString(v); m != "" {
		return "", fmt.Errorf("%q is not a local directory", v)
	}
	if v == "~" || strings.HasPrefix(v, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		v = filepath.Join(home, strings.TrimPrefix(v, "~"))
	}
	return filepath.Abs(v)
}

var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// System is the nix system string of this machine, e.g. "x86_64-linux".
func System() string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	return arch + "-" + runtime.GOOS
}

// Hosts lists the flake's nixosConfigurations.
func Hosts(r run.Runner, flakeDir string) ([]string, error) {
	out, errs, err := r.Capture("", "nix", "eval", "--json", flakeDir+"#nixosConfigurations",
		"--apply", "builtins.attrNames")
	if err != nil {
		return nil, fmt.Errorf("listing hosts: %s", run.LastLine(errs))
	}
	var hosts []string
	if err := json.Unmarshal([]byte(out), &hosts); err != nil {
		return nil, fmt.Errorf("listing hosts: %w", err)
	}
	return hosts, nil
}

// Dirty reports whether git sees uncommitted changes in the given paths of repo.
func Dirty(r run.Runner, repo string, paths ...string) bool {
	_, _, err := r.Capture(repo, "git", append([]string{"diff", "--quiet", "--"}, paths...)...)
	return err != nil
}

// Restore puts tracked files back to their committed state.
func Restore(r run.Runner, repo string, paths ...string) error {
	_, errs, err := r.Capture(repo, "git", append([]string{"checkout", "--"}, paths...)...)
	if err != nil {
		return fmt.Errorf("git checkout: %s", run.LastLine(errs))
	}
	return nil
}

// ScratchCopy copies the working tree (uncommitted and untracked files included, ignored
// ones not) into dst, so updating and building never touch the real checkout.
// Symlinks and files deleted from the working tree are skipped.
func ScratchCopy(r run.Runner, repo, dst string) error {
	out, errs, err := r.Capture(repo, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return fmt.Errorf("git ls-files: %s", run.LastLine(errs))
	}
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		src := filepath.Join(repo, rel)
		st, err := os.Lstat(src)
		if err != nil || !st.Mode().IsRegular() {
			continue
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
