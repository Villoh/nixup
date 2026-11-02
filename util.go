package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var useColor = isTTY(os.Stdout)

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func paint(s string, code int) string {
	if !useColor {
		return s
	}
	return fmt.Sprintf("\033[%dm%s\033[0m", code, s)
}

func bold(s string) string { return paint(s, 1) }

// confirm asks on the terminal. Without a terminal the answer is always no:
// unattended runs must opt in with --yes.
func confirm(prompt string) bool {
	if !isTTY(os.Stdin) {
		return false
	}
	fmt.Printf("%s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y")
}

func have(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// capture runs a command and returns stdout and stderr separately.
func capture(dir, name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

// stream runs a command attached to the terminal (builds, nix-update).
func stream(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}

func short(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
