// Package run is the only place that starts processes. Everything else takes a Runner,
// so tests can script nix, git, nh and flatpak instead of running them.
package run

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Runner interface {
	// Capture runs a command and returns its stdout and stderr separately.
	Capture(dir, name string, args ...string) (stdout, stderr string, err error)
	// Stream runs a command attached to the user's terminal (builds, nix-update).
	Stream(dir, name string, args ...string) error
	// Has reports whether a command is on PATH.
	Has(cmd string) bool
}

// Exec runs real processes. Out receives the output of Stream.
type Exec struct{ Out io.Writer }

// command is name with nix's "Git tree is dirty" warning turned off: nixup works on
// uncommitted trees on purpose and says so itself.
func command(dir, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NIX_CONFIG="+strings.TrimSpace(os.Getenv("NIX_CONFIG")+"\nwarn-dirty = false"))
	return cmd
}

func (Exec) Capture(dir, name string, args ...string) (string, string, error) {
	cmd := command(dir, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func (e Exec) Stream(dir, name string, args ...string) error {
	cmd := command(dir, name, args...)
	cmd.Stdin = os.Stdin // interactive tools (nh --ask, a sudo that reads stdin) can ask
	cmd.Stdout, cmd.Stderr = e.Out, e.Out
	return cmd.Run()
}

func (Exec) Has(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// LastLine is the last non-empty line of a command's stderr, for short error messages.
func LastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
