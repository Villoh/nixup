package cli_test

import (
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/run"
)

// completions runs `nixup __complete words...` and returns its lines.
func (f *fixture) completions(t *testing.T, words ...string) []string {
	t.Helper()
	f.out.Reset()
	if code := f.run(append([]string{"__complete"}, words...)...); code != exit.OK {
		t.Fatalf("code %d for %q", code, words)
	}
	if f.errb.Len() != 0 {
		t.Errorf("a completion never writes errors: %q", f.errb)
	}
	return strings.Split(strings.TrimRight(f.out.String(), "\n"), "\n")
}

func completionFixture(t *testing.T) *fixture {
	f := newFixture(t, map[string]string{"nixpkgs": "NixOS/nixpkgs@#aaa", "home-manager": "nix-community/home-manager@#bbb"})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `["ripgrep","nixpkgs"]`})
	f.e.Run.On(run.Rule{Prefix: "nix eval --json " + f.repo + "#nixosConfigurations", Out: `["desktop","laptop"]`})
	f.e.Run.On(run.Rule{Prefix: "flatpak list --columns=application", Out: "co.hyprlab.Hylki\norg.gnome.Platform\n"})
	f.e.Run.On(run.Rule{Prefix: "nix profile list --json", Out: `{"elements":{"hello":{"originalUrl":"flake:nixpkgs"}}}`})
	return f
}

func TestCompleteCommands(t *testing.T) {
	f := completionFixture(t)
	got := f.completions(t, "")
	if len(got) < 6 || got[0] != "check\twhat is outdated, read-only" || !strings.HasPrefix(got[1], "update\t") {
		t.Errorf("the first word is a command, with what it does: %q", got)
	}
	if got := f.completions(t, "completion", ""); strings.Join(got, " ") != "bash zsh fish" {
		t.Errorf("completion takes a shell: %q", got)
	}
	if got := f.completions(t, "bogus", ""); len(got) != 1 || got[0] != "" {
		t.Errorf("an unknown command offers nothing: %q", got)
	}
}

func TestCompleteFlags(t *testing.T) {
	f := completionFixture(t)
	got := "\n" + strings.Join(f.completions(t, "update", "--"), "\n")
	for _, want := range []string{"\n-C\tflake directory", "\n-H\thost to build", "\n-y\tapply without asking", "\n--allow-dirty\t", "\n--source\t", "\n--build\t"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if got := strings.Join(f.completions(t, "-"), "\n"); !strings.Contains(got, "--review\t") {
		t.Errorf("a flag as the first word is a flag of check, the default command:\n%s", got)
	}
}

func TestCompleteFlagValues(t *testing.T) {
	f := completionFixture(t)
	for _, c := range []struct {
		words []string
		want  string
	}{
		{[]string{"update", "--source", ""}, "input package flatpak profile"},
		{[]string{"check", "--source", "in"}, "input package flatpak profile"},
		{[]string{"list", "--source", "input,f"}, "input,package input,flatpak input,profile"},
		{[]string{"search", "--source", ""}, "nixpkgs package input flatpak"},
		{[]string{"update", "--source=pa"}, "--source=input --source=package --source=flatpak --source=profile"},
		{[]string{"update", "--source", "=", "pa"}, "input package flatpak profile"}, // bash split the word
		{[]string{"update", "--source", "="}, "input package flatpak profile"},
		{[]string{"update", "-C", f.repo, "-H", ""}, "desktop\thost laptop\thost"},
		{[]string{"update", "-C", f.repo, "-H=l"}, "-H=desktop\thost -H=laptop\thost"},
		{[]string{"update", "-C", f.repo, "--source", "input", "--build", ""}, "home-manager\tinput nixpkgs\tinput"}, // a bool flag takes no value
		{[]string{"update", "-C", ""}, ":dirs"},
		{[]string{"update", "--report", ""}, ":files"},
	} {
		if got := strings.Join(f.completions(t, c.words...), " "); got != c.want {
			t.Errorf("%q: got %q, want %q", c.words, got, c.want)
		}
	}
}

func TestCompleteUpdateNames(t *testing.T) {
	f := completionFixture(t)
	got := f.completions(t, "update", "-C", f.repo, "")
	want := []string{"co.hyprlab.Hylki\tflatpak", "hello\tprofile", "home-manager\tinput", "nixpkgs\tinput",
		"org.gnome.Platform\tflatpak", "ripgrep\tpackage"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("every source, sorted, a name once (nixpkgs is an input first):\n%q\nwant\n%q", got, want)
	}
	if got := f.completions(t, "update", "-C", f.repo, "nixpkgs", "--build", ""); strings.Contains(strings.Join(got, ","), "nixpkgs") {
		t.Errorf("a name already typed is not offered again: %q", got)
	}
	if got := f.completions(t, "update", "-C", f.repo, "--source", "package,profile", ""); strings.Join(got, ",") != "hello\tprofile,nixpkgs\tpackage,ripgrep\tpackage" {
		t.Errorf("--source limits the names: %q", got)
	}
	if got := f.completions(t, "check", "-C", f.repo, ""); len(got) != 1 || got[0] != "" {
		t.Errorf("only update takes names: %q", got)
	}
	g := completionFixture(t)
	if got := g.completions(t, "update", "-C", "/nonexistent", ""); len(got) != 1 || got[0] != "" {
		t.Errorf("no flake: nothing, and no error: %q", got)
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		f := newFixture(t, nil)
		if code := f.run("completion", shell); code != exit.OK || !strings.Contains(f.out.String(), "nixup __complete") {
			t.Errorf("%s: code %d\n%s", shell, code, f.out)
		}
	}
	f := newFixture(t, nil)
	if code := f.run("completion", "tcsh"); code != exit.Error || !strings.Contains(f.errb.String(), "usage: nixup completion bash|zsh|fish") {
		t.Errorf("code %d err %q", code, f.errb)
	}
}
