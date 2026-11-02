package main

import (
	"reflect"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v2.0.0", "v1.6.2", true},
		{"1.11.6-dms.4", "1.11.6-dms.2", true},
		{"0.1.0", "0.1.0-alpha.7", true}, // final beats its prerelease
		{"0.1.0-alpha.8", "0.1.0-alpha.7", true},
		{"1.01", "1.01", false},
		{"v1.1.1-rc.3", "v1.1.1", false},
		{"dms/1.2.0", "1.1.9", true},
	}
	for _, c := range cases {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestBump(t *testing.T) {
	cases := map[[2]string]string{
		{"0.21.0", "1.1.1"}:  "major",
		{"v1.6.2", "v2.0.0"}: "major",
		{"0.21.0", "0.22.0"}: "0.x-minor",
		{"1.1.0", "1.2.0"}:   "minor",
		{"1.1.12", "1.1.13"}: "patch",
	}
	for in, want := range cases {
		if got := bump(in[0], in[1]); got != want {
			t.Errorf("bump(%v)=%s want %s", in, got, want)
		}
	}
	if bumpWarning("major") == "" || bumpWarning("patch") != "" {
		t.Error("only major and 0.x jumps warn")
	}
}

func TestTagHelpers(t *testing.T) {
	if tagPrefix("v1.6.2") != "" || tagPrefix("dms/1.2") != "dms/" || tagPrefix("1.01") != "" {
		t.Error("tagPrefix")
	}
	if !tagLike.MatchString("v1.6.2") || !tagLike.MatchString("dms/1.2") || tagLike.MatchString("release-25.11") {
		t.Error("tagLike must accept versions and reject release branches")
	}
	if githubRepo("https://github.com/PabloStarOk/padpoll/releases/download/v1.1.0/x.tar.gz") != "PabloStarOk/padpoll" ||
		githubRepo("https://wealthfolio.app/") != "" {
		t.Error("githubRepo")
	}
}

const lockA = `{"root":"root","version":7,"nodes":{
 "root":{"inputs":{"nixpkgs":"nixpkgs","dms":"dms","hm":["nixpkgs"]}},
 "nixpkgs":{"locked":{"rev":"aaa1111"},"original":{"type":"github","owner":"NixOS","repo":"nixpkgs"}},
 "dms":{"locked":{"rev":"bbb2222"},"original":{"type":"github","owner":"x","repo":"dms","ref":"v1.6.2"}},
 "sub":{"locked":{"rev":"ccc3333"}}}}`

const lockB = `{"root":"root","version":7,"nodes":{
 "root":{"inputs":{"nixpkgs":"nixpkgs","dms":"dms","hm":["nixpkgs"]}},
 "nixpkgs":{"locked":{"rev":"aaa9999"},"original":{"type":"github","owner":"NixOS","repo":"nixpkgs"}},
 "dms":{"locked":{"rev":"bbb2222"},"original":{"type":"github","owner":"x","repo":"dms","ref":"v1.6.2"}},
 "sub":{"locked":{"rev":"ccc4444"}}}}`

func TestDiffLocks(t *testing.T) {
	a, err := parseLock([]byte(lockA))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := parseLock([]byte(lockB))
	if !reflect.DeepEqual(a.names(), []string{"dms", "nixpkgs"}) {
		t.Errorf("follows lists must be skipped: %v", a.names())
	}
	changes, trans := diffLocks(a, b)
	want := []change{{Name: "nixpkgs", Repo: "NixOS/nixpkgs", Old: "aaa1111", New: "aaa9999"}}
	if !reflect.DeepEqual(changes, want) || trans != 1 {
		t.Errorf("got %+v trans=%d", changes, trans)
	}
	if c, tr := diffLocks(a, a); len(c) != 0 || tr != 0 {
		t.Error("identical locks must produce no diff")
	}
}

func TestExtraArgs(t *testing.T) {
	p := ownPackage{Update: []string{"/nix/store/x/bin/nix-update", "--version=unstable", "--use-github-releases"}}
	args, pre := p.extraArgs()
	if !pre || !reflect.DeepEqual(args, []string{"--use-github-releases"}) {
		t.Errorf("args=%v pre=%v", args, pre)
	}
}

func TestSelectPackages(t *testing.T) {
	all := []ownPackage{{Name: "padpoll"}, {Name: "strata"}, {Name: "wealthfolio"}}
	got, err := selectPackages(all, []string{"padpoll"})
	if err != nil || len(got) != 1 || got[0].Name != "padpoll" {
		t.Fatalf("asking for one package must return only that one: %v %v", got, err)
	}
	if got, _ := selectPackages(all, nil); len(got) != 3 {
		t.Error("no names means all packages")
	}
	if _, err := selectPackages(all, []string{"nope"}); err == nil {
		t.Error("unknown package must be an error")
	}
}
