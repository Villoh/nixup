// Package ui is the terminal: output streams, color and yes/no questions.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type Term struct {
	Out, Err io.Writer
	In       io.Reader
	Color    bool
	// Interactive is true when questions can be answered. Without it every Confirm is "no":
	// unattended runs must opt in with -y.
	Interactive bool

	rd *bufio.Reader
}

// Stdio is the process's real terminal.
func Stdio() *Term {
	return &Term{Out: os.Stdout, Err: os.Stderr, In: os.Stdin,
		Color: colorEnabled(isTTY(os.Stdout), os.Getenv), Interactive: isTTY(os.Stdin)}
}

// colorEnabled follows https://no-color.org: any non-empty NO_COLOR turns color off, as does
// TERM=dumb. Without a terminal there is no color either.
func colorEnabled(tty bool, getenv func(string) string) bool {
	return tty && getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func (t *Term) Paint(s string, code int) string {
	if !t.Color {
		return s
	}
	return fmt.Sprintf("\033[%dm%s\033[0m", code, s)
}

func (t *Term) Bold(s string) string { return t.Paint(s, 1) }

func (t *Term) Printf(format string, a ...any) { fmt.Fprintf(t.Out, format, a...) }
func (t *Term) Println(a ...any)               { fmt.Fprintln(t.Out, a...) }
func (t *Term) Errorln(a ...any)               { fmt.Fprintln(t.Err, a...) }

// Confirm asks a yes/no question; anything but "y" (or no terminal) is no.
func (t *Term) Confirm(prompt string) bool {
	if !t.Interactive || t.In == nil {
		return false
	}
	if t.rd == nil {
		t.rd = bufio.NewReader(t.In)
	}
	fmt.Fprintf(t.Out, "%s [y/N] ", prompt)
	line, _ := t.rd.ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y")
}
