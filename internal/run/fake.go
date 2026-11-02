package run

import (
	"strings"
	"sync"
)

// Call is one recorded command.
type Call struct {
	Dir, Name string
	Args      []string
}

// String is "name arg1 arg2", the form Rule.Prefix is matched against.
func (c Call) String() string { return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " ")) }

// Rule answers the first command whose String() starts with Prefix.
// Do runs before the answer (to simulate side effects such as nix-update editing a file).
type Rule struct {
	Prefix string
	Out    string
	Err    string // stderr
	Fail   error
	Do     func(Call)
}

// Fake is a scripted Runner for tests. Unmatched commands succeed with no output.
type Fake struct {
	mu      sync.Mutex
	Rules   []Rule
	Missing map[string]bool // commands Has reports as absent
	calls   []Call
}

func (f *Fake) On(r Rule) *Fake { f.Rules = append(f.Rules, r); return f }

func (f *Fake) respond(c Call) (string, string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	for _, r := range f.Rules {
		if strings.HasPrefix(c.String(), r.Prefix) {
			if r.Do != nil {
				r.Do(c)
			}
			return r.Out, r.Err, r.Fail
		}
	}
	return "", "", nil
}

func (f *Fake) Capture(dir, name string, args ...string) (string, string, error) {
	return f.respond(Call{dir, name, args})
}

func (f *Fake) Stream(dir, name string, args ...string) error {
	_, _, err := f.respond(Call{dir, name, args})
	return err
}

func (f *Fake) Has(cmd string) bool { return !f.Missing[cmd] }

// Calls returns every command run so far, in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Count is how many recorded commands start with prefix.
func (f *Fake) Count(prefix string) int {
	n := 0
	for _, c := range f.Calls() {
		if strings.HasPrefix(c.String(), prefix) {
			n++
		}
	}
	return n
}
