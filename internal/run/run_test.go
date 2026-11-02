package run

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestExecCaptureSplitsStreams(t *testing.T) {
	out, errs, err := Exec{}.Capture("", "sh", "-c", "echo out; echo err >&2; exit 3")
	if err == nil {
		t.Fatal("exit 3 must be an error")
	}
	if strings.TrimSpace(out) != "out" || strings.TrimSpace(errs) != "err" {
		t.Errorf("out=%q err=%q", out, errs)
	}
}

func TestExecStreamAndDir(t *testing.T) {
	var buf bytes.Buffer
	dir := t.TempDir()
	if err := (Exec{Out: &buf}).Stream(dir, "sh", "-c", "pwd; echo oops >&2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "oops") || !strings.Contains(buf.String(), dir) {
		t.Errorf("stream must carry stdout and stderr from the given dir: %q", buf.String())
	}
}

func TestExecHas(t *testing.T) {
	if !(Exec{}).Has("sh") || (Exec{}).Has("definitely-not-a-command-nixup") {
		t.Error("Has must reflect PATH")
	}
}

func TestLastLine(t *testing.T) {
	if got := LastLine("a\nb\n\n"); got != "b" {
		t.Errorf("got %q", got)
	}
	if got := LastLine(""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestFakeRulesAndCalls(t *testing.T) {
	boom := errors.New("boom")
	var hook string
	f := (&Fake{Missing: map[string]bool{"flatpak": true}}).
		On(Rule{Prefix: "git ls-remote --tags", Out: "tags"}).
		On(Rule{Prefix: "git ls-remote", Out: "heads"}).
		On(Rule{Prefix: "nix build", Fail: boom, Err: "bad", Do: func(c Call) { hook = c.Dir }})

	if out, _, _ := f.Capture("", "git", "ls-remote", "--tags", "x"); out != "tags" {
		t.Errorf("first matching rule must win, got %q", out)
	}
	if out, _, _ := f.Capture("", "git", "ls-remote", "x"); out != "heads" {
		t.Errorf("got %q", out)
	}
	if err := f.Stream("/repo", "nix", "build", ".#x"); !errors.Is(err, boom) || hook != "/repo" {
		t.Errorf("Stream must apply Fail and Do: err=%v hook=%q", err, hook)
	}
	if out, errs, err := f.Capture("", "unscripted"); out != "" || errs != "" || err != nil {
		t.Error("unmatched commands succeed silently")
	}
	if f.Has("flatpak") || !f.Has("nix") {
		t.Error("Missing controls Has")
	}
	if f.Count("git ls-remote") != 2 || len(f.Calls()) != 4 {
		t.Errorf("calls: %v", f.Calls())
	}
}
