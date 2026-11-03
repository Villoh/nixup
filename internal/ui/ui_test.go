package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestPaint(t *testing.T) {
	if got := (&Term{}).Paint("x", 31); got != "x" {
		t.Errorf("no color: %q", got)
	}
	if got := (&Term{Color: true}).Paint("x", 31); got != "\033[31mx\033[0m" {
		t.Errorf("color: %q", got)
	}
	if got := (&Term{Color: true}).Bold("x"); got != "\033[1mx\033[0m" {
		t.Errorf("bold: %q", got)
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		interactive bool
		want        bool
	}{
		{"yes", "y\n", true, true},
		{"uppercase yes", "Y\n", true, true},
		{"no", "n\n", true, false},
		{"empty means no", "\n", true, false},
		{"word is not y", "yes\n", true, false},
		{"eof", "", true, false},
		{"not interactive ignores a yes", "y\n", false, false},
	}
	for _, c := range cases {
		var out bytes.Buffer
		term := &Term{Out: &out, In: strings.NewReader(c.input), Interactive: c.interactive}
		if got := term.Confirm("ok?"); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if c.interactive && !strings.Contains(out.String(), "ok? [y/N]") {
			t.Errorf("%s: prompt not shown: %q", c.name, out.String())
		}
		if !c.interactive && out.Len() != 0 {
			t.Errorf("%s: must not prompt without a terminal", c.name)
		}
	}
}

func TestConfirmReadsOneLinePerQuestion(t *testing.T) {
	term := &Term{Out: &bytes.Buffer{}, In: strings.NewReader("y\nn\ny\n"), Interactive: true}
	got := []bool{term.Confirm("a"), term.Confirm("b"), term.Confirm("c")}
	if got[0] != true || got[1] != false || got[2] != true {
		t.Errorf("answers must not be lost between questions: %v", got)
	}
}

func TestColorEnabled(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	cases := []struct {
		name string
		tty  bool
		env  func(string) string
		want bool
	}{
		{"terminal", true, env("TERM", "xterm-256color"), true},
		{"pipe", false, env("TERM", "xterm"), false},
		{"NO_COLOR set", true, env("NO_COLOR", "1"), false},
		{"NO_COLOR any non-empty value", true, env("NO_COLOR", "false"), false},
		{"NO_COLOR empty is ignored", true, env("NO_COLOR", ""), true},
		{"dumb terminal", true, env("TERM", "dumb"), false},
	}
	for _, c := range cases {
		if got := colorEnabled(c.tty, c.env); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
