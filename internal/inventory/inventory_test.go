package inventory_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/inventory"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/testutil"
)

var hash = strings.Repeat("a", 32)

func repo(t *testing.T) string {
	return testutil.Tree(t, map[string]string{"flake.nix": "{}", "flake.lock": testutil.LockJSON(map[string]string{
		"dms":     "o/dms@v1.6.2#" + strings.Repeat("1", 40),
		"nixpkgs": "NixOS/nixpkgs@nixos-unstable#" + strings.Repeat("e", 40),
	})})
}

func bySource(groups []inventory.Group) map[string]inventory.Group {
	m := map[string]inventory.Group{}
	for _, g := range groups {
		m[g.Source] = g
	}
	return m
}

func TestList(t *testing.T) {
	f := &run.Fake{}
	f.On(run.Rule{Prefix: "nix eval --json .#packages.", Out: `{"tool":{"version":"1.2.0","urls":["https://github.com/o/tool/x"],"homepage":"","update":[]}}`})
	f.On(run.Rule{Prefix: "flatpak list --app --columns=application,version,origin", Out: "org.app.One\t3.0\tflathub\n\n"})
	f.On(run.Rule{Prefix: "nix profile list --json", Out: `{"elements":{
		"zz":{"originalUrl":"flake:nixpkgs"},
		"qt":{"originalUrl":"github:NixOS/nixpkgs/` + strings.Repeat("a", 40) + `"}}}`})

	groups := inventory.List(f, repo(t), nil)
	var order []string
	for _, g := range groups {
		order = append(order, g.Source)
	}
	if strings.Join(order, ",") != "input,package,flatpak,profile" {
		t.Errorf("sources in order, empty ones left out: %v", order)
	}
	g := bySource(groups)
	in := g["input"].Items
	if len(in) != 2 || in[0].Name != "dms" || in[0].Version != "v1.6.2" || in[0].Note != "github:o/dms/v1.6.2" {
		t.Errorf("a pinned tag shows the tag: %+v", in)
	}
	if !strings.HasPrefix(in[1].Version, "eeeeeee · ") || in[1].Note != "github:NixOS/nixpkgs/nixos-unstable" {
		t.Errorf("a branch shows the locked commit and its age: %+v", in[1])
	}
	if p := g["package"].Items; len(p) != 1 || p[0].Version != "1.2.0" || p[0].Note != "github:o/tool" {
		t.Errorf("packages, origin in flake syntax: %+v", p)
	}
	pr := g["profile"].Items
	if len(pr) != 2 || pr[0].Name != "qt" || pr[0].Version != "aaaaaaa" || pr[0].Note != "github:NixOS/nixpkgs" ||
		pr[1].Version != "" || pr[1].Note != "flake:nixpkgs" {
		t.Errorf("profile: the pinned revision is the version, the rest the origin: %+v", pr)
	}
	if fp := g["flatpak"].Items; len(fp) != 1 || fp[0].Name != "org.app.One" || fp[0].Version != "3.0" || fp[0].Note != "flathub" {
		t.Errorf("flatpak, origin is the remote: %+v", fp)
	}

	only := inventory.List(f, repo(t), []string{"profile", "flatpak"})
	if len(only) != 2 || only[0].Source != "flatpak" || only[1].Source != "profile" {
		t.Errorf("--source picks sources, in display order: %+v", only)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c.String(), "ls-remote") || strings.Contains(c.String(), "remote-ls") {
			t.Errorf("list must not reach the network: %s", c)
		}
	}
}

func TestListReportsBrokenSources(t *testing.T) {
	f := &run.Fake{Missing: map[string]bool{"flatpak": true}}
	f.On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"), Err: "error: boom\n"})
	f.On(run.Rule{Prefix: "flatpak", Fail: errors.New("x"), Err: "flatpak: not found\n"}) // what running it would do
	g := bySource(inventory.List(f, t.TempDir(), nil))
	if g["input"].Err == "" || !strings.Contains(g["package"].Err, "boom") {
		t.Errorf("errors are kept per source: %+v", g)
	}
	if _, ok := g["flatpak"]; ok {
		t.Errorf("no flatpak installed: no section")
	}
}

const evalOut = `{
 "rev": "e7439b6",
 "nixpkgs": {"total": 14, "items": [
   {"name":"ripgrep","pname":"ripgrep","version":"15.2.0","description":"Fast\n  grep"},
   {"name":"ripgrep-all","pname":"ripgrep-all","version":"0.10.10","description":"rg in PDFs"}]},
 "own": {"total": 0, "items": []},
 "inputs": {
   "zeta":  {"total": 1, "items": [{"name":"rg-zeta","pname":"","version":"1","description":""}]},
   "alpha": {"total": 1, "items": [{"name":"rg-alpha","pname":"","version":"2","description":""}]},
   "empty": {"total": 0, "items": []}}
}`

func TestSearch(t *testing.T) {
	f := &run.Fake{}
	f.On(run.Rule{Prefix: "nix eval --json --impure --expr", Out: evalOut})
	f.On(run.Rule{Prefix: "nix-store --query --requisites /run/current-system",
		Out: "/nix/store/" + hash + "-ripgrep-15.2.0\n/nix/store/" + hash + "-ripgrep-all-0.9.0\n"})
	f.On(run.Rule{Prefix: "flatpak search", Out: "org.gtk.Gtk3theme.Rg\t1\tRg theme\tA theme\n" +
		"de.x.Clapgrep\t26.09\tClapgrep\tSearch\n" +
		"io.ripgrep.Ripgrep\t2\tRipgrep\tThe app\n" +
		"io.ripgrep.Ripgrep\t2\tRipgrep\tThe app\n"})
	f.On(run.Rule{Prefix: "flatpak list --app --columns=application", Out: "io.ripgrep.Ripgrep\n"})

	groups, err := inventory.Search(f, "/repo", "RipGrep", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, g := range groups {
		order = append(order, g.Source)
	}
	if strings.Join(order, ",") != "nixpkgs,input:alpha,input:zeta,flatpak" {
		t.Errorf("nixpkgs, yours, inputs by name, flatpak; empty sources left out: %v", order)
	}
	g := bySource(groups)
	np := g["nixpkgs"]
	if np.Rev != "e7439b6" || np.More != 12 || len(np.Items) != 2 {
		t.Errorf("total minus shown is 'more': %+v", np)
	}
	if !np.Items[0].InSystem || np.Items[1].InSystem {
		t.Errorf("in the system means the same name and version in its closure: %+v", np.Items)
	}
	if np.Items[0].Note != "Fast grep" {
		t.Errorf("descriptions on one line: %q", np.Items[0].Note)
	}
	fp := g["flatpak"]
	if len(fp.Items) != 2 || fp.Items[0].Name != "io.ripgrep.Ripgrep" || !fp.Items[0].InSystem || fp.More != 1 {
		t.Errorf("flatpak: the app named that first, duplicates once, installed marked, limit: %+v", fp)
	}

	var expr string
	for _, c := range f.Calls() {
		if c.Name == "nix" {
			expr = c.Args[len(c.Args)-1]
		}
	}
	if !strings.Contains(expr, `q = "[rR][iI][pP][gG][rR][eE][pP]"`) || !strings.Contains(expr, "limit = 2;") {
		t.Errorf("case-insensitive pattern and limit reach nix: %s", expr)
	}
}

func TestSearchPattern(t *testing.T) {
	f := &run.Fake{Missing: map[string]bool{"flatpak": true}}
	f.On(run.Rule{Prefix: "nix eval", Out: `{"rev":"","nixpkgs":null,"own":{"total":0,"items":[]},"inputs":{}}`})
	if _, err := inventory.Search(f, "/repo", "gtk+3.x_1-a", 5, nil); err != nil {
		t.Fatal(err)
	}
	expr := f.Calls()[0].Args[len(f.Calls()[0].Args)-1]
	if !strings.Contains(expr, `q = "[gG][tT][kK]\\+3\\.[xX]_1-[aA]"`) {
		t.Errorf(". and + are literal: %s", expr)
	}
}

func TestSearchErrors(t *testing.T) {
	f := &run.Fake{}
	for _, q := range []string{"", "a b", `x"y`, "${x}", "a;b"} {
		if _, err := inventory.Search(f, "/repo", q, 5, nil); err == nil {
			t.Errorf("%q must be rejected before reaching nix", q)
		}
	}
	if len(f.Calls()) != 0 {
		t.Errorf("nothing ran: %v", f.Calls())
	}
	f.On(run.Rule{Prefix: "nix eval", Fail: errors.New("x"), Err: "error: no flake\n"})
	if _, err := inventory.Search(f, "/repo", "x", 5, nil); err == nil || !strings.Contains(err.Error(), "no flake") {
		t.Errorf("eval failure: %v", err)
	}
}

func TestSearchSources(t *testing.T) {
	f := &run.Fake{}
	f.On(run.Rule{Prefix: "nix eval --json --impure --expr", Out: evalOut})
	f.On(run.Rule{Prefix: "flatpak search", Out: "io.x.Rg\t1\tRg\tThe app\n"})

	groups, _ := inventory.Search(f, "/repo", "rg", 5, []string{"input"})
	if len(groups) != 2 || groups[0].Source != "input:alpha" || groups[1].Source != "input:zeta" {
		t.Errorf("input matches every input: %+v", groups)
	}
	if f.Count("flatpak search") != 0 {
		t.Error("flatpak is not searched when not asked for")
	}

	f2 := &run.Fake{}
	f2.On(run.Rule{Prefix: "flatpak search", Out: "io.x.Rg\t1\tRg\tThe app\n"})
	groups, _ = inventory.Search(f2, "/repo", "rg", 5, []string{"flatpak"})
	if len(groups) != 1 || groups[0].Source != "flatpak" || f2.Count("nix") != 0 {
		t.Errorf("flatpak only: no evaluation at all: %+v, nix calls %d", groups, f2.Count("nix"))
	}
}
