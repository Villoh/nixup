package flake

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// # nixup: follows nixpkgs#<attribute>
	followsRe = regexp.MustCompile(`^\s*#\s*nixup:\s*follows\s+nixpkgs#([A-Za-z_][A-Za-z0-9_'-]*)\s*$`)
	// `name = {`, `name.url =`, `inputs.name.url =`
	definitionRe = regexp.MustCompile(`^\s*(?:inputs\.)?([A-Za-z_][A-Za-z0-9_'-]*)(?:\.[A-Za-z.]+)?\s*=`)
)

// Follows reads the `# nixup: follows nixpkgs#<attribute>` comments of flake.nix: input
// name -> nixpkgs attribute. Such an input provides something else (a module) for a
// package that comes from nixpkgs, so its tag follows that package's version. A comment
// applies to the first definition after it, past other comments and blank lines.
func Follows(repo string) (map[string]string, error) {
	f, err := os.Open(filepath.Join(repo, "flake.nix"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	pending := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if m := followsRe.FindStringSubmatch(line); m != nil {
			pending = m[1]
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := definitionRe.FindStringSubmatch(line); m != nil && pending != "" {
			out[m[1]] = pending
		}
		pending = ""
	}
	return out, sc.Err()
}
