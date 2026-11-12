package cli

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Villoh/nixup/internal/check"
	"github.com/Villoh/nixup/internal/exit"
	"github.com/Villoh/nixup/internal/flake"
	"github.com/Villoh/nixup/internal/inventory"
	"github.com/Villoh/nixup/internal/update"
)

// commands are what `nixup <TAB>` offers.
var commands = [][2]string{
	{"check", "what is outdated, changes nothing"},
	{"update", "review and apply updates"},
	{"list", "everything it watches, with current versions"},
	{"search", "look a package name up"},
	{"version", "print the version"},
	{"help", "print the usage"},
	{"completion", "print a shell completion script (bash, zsh, fish)"},
}

// sourcesOf are the --source values each command takes.
var sourcesOf = map[string][]string{
	"check":  check.Sources(),
	"update": update.Sources,
	"list":   inventory.ListSources,
	"search": inventory.SearchSources,
}

// complete answers `nixup __complete <words...>`: the words typed after "nixup", the last
// one being the word under the cursor (maybe empty). It prints one candidate per line,
// "value<TAB>description", or ":files" / ":dirs" to let the shell complete paths. It never
// prints errors: a completion that fails offers nothing.
func complete(words []string, d Deps) int {
	// bash hands "--source=inp" over as "--source", "=", "inp" and matches what it gets
	// back against "inp" alone: answer it without the "--source=".
	bashSplit := len(words) >= 2 && (words[len(words)-1] == "=" || words[len(words)-2] == "=")
	words = joinEquals(words)
	if len(words) == 0 {
		words = []string{""}
	}
	cur, prev := words[len(words)-1], words[:len(words)-1]
	cmd := "check"
	if len(prev) > 0 && !strings.HasPrefix(prev[0], "-") {
		cmd, prev = prev[0], prev[1:]
	} else if len(prev) == 0 && !strings.HasPrefix(cur, "-") {
		for _, c := range commands {
			d.T.Println(c[0] + "\t" + c[1])
		}
		return exit.OK
	}
	if cmd == "completion" {
		if len(prev) == 0 {
			d.T.Println("bash\nzsh\nfish")
		}
		return exit.OK
	}
	fs := d.flagSet(cmd)
	if fs == nil {
		return exit.OK
	}
	pos, _, _ := parse(fs, prev) // reads -C and --source typed so far; errors are expected mid-line

	// A value: "--flag=<cur>" in one word, or the word after a flag that takes one.
	prefix, name, value := "", "", cur
	if f, v, ok := strings.Cut(cur, "="); ok && strings.HasPrefix(f, "-") {
		prefix, name, value = f+"=", strings.TrimLeft(f, "-"), v
		if bashSplit {
			prefix = ""
		}
	} else if len(prev) > 0 && takesValue(fs, prev[len(prev)-1]) {
		name = strings.TrimLeft(prev[len(prev)-1], "-")
	}
	if name != "" {
		d.completeValue(cmd, fs, name, prefix, value)
		return exit.OK
	}
	if strings.HasPrefix(cur, "-") {
		fs.VisitAll(func(f *flag.Flag) {
			dash := "--"
			if len(f.Name) == 1 {
				dash = "-" // -C, -H, -y, as the help shows them
			}
			d.T.Println(dash + f.Name + "\t" + f.Usage)
		})
		return exit.OK
	}
	if cmd == "update" {
		d.completeNames(fs, pos)
	}
	return exit.OK
}

// joinEquals undoes bash's split of "--source=input" into "--source", "=", "input".
func joinEquals(words []string) []string {
	var out []string
	for i := 0; i < len(words); i++ {
		if words[i] == "=" && len(out) > 0 && strings.HasPrefix(out[len(out)-1], "-") {
			out[len(out)-1] += "="
			if i+1 < len(words) {
				out[len(out)-1] += words[i+1]
				i++
			}
			continue
		}
		out = append(out, words[i])
	}
	return out
}

// takesValue reports whether word is a flag that needs a value after it.
func takesValue(fs *flag.FlagSet, word string) bool {
	if !strings.HasPrefix(word, "-") || strings.Contains(word, "=") {
		return false
	}
	f := fs.Lookup(strings.TrimLeft(word, "-"))
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !b.IsBoolFlag()
}

func (d Deps) completeValue(cmd string, fs *flag.FlagSet, name, prefix, value string) {
	switch name {
	case "C":
		d.T.Println(":dirs")
	case "report":
		d.T.Println(":files")
	case "source":
		done, _ := cutLast(value, ",")
		if done != "" {
			done += ","
		}
		for _, s := range sourcesOf[cmd] {
			if !contains(strings.Split(done, ","), s) {
				d.T.Println(prefix + done + s)
			}
		}
	case "H":
		if repo, ok := d.completionRepo(fs); ok {
			hosts, _ := flake.Hosts(d.R, repo)
			for _, h := range hosts {
				d.T.Println(prefix + h + "\thost")
			}
		}
	}
}

// completeNames offers what `nixup update` takes by name, from the sources --source leaves
// on, without the names already typed.
func (d Deps) completeNames(fs *flag.FlagSet, typed []string) {
	repo, ok := d.completionRepo(fs)
	if !ok {
		return
	}
	on, _ := parseSources(fs.Lookup("source").Value.String(), update.Sources)
	enabled := func(s string) bool { return len(on) == 0 || contains(on, s) }
	names := map[string]string{} // name -> where it comes from
	add := func(source string, list []string) {
		for _, n := range list {
			if _, seen := names[n]; !seen && !contains(typed, n) {
				names[n] = source
			}
		}
	}
	if enabled("input") {
		if lock, err := flake.ReadLock(filepath.Join(repo, "flake.lock")); err == nil {
			var inputs []string
			for n := range lock.Direct() {
				inputs = append(inputs, n)
			}
			add("input", inputs)
		}
	}
	if enabled("package") {
		add("package", d.packageNames(repo))
	}
	c := &check.Checker{R: d.R, GH: d.GH}
	if enabled("flatpak") {
		var ids []string
		for id := range c.FlatpakInstalled() {
			ids = append(ids, id)
		}
		add("flatpak", ids)
	}
	if enabled("profile") {
		entries, _ := c.ProfileEntries()
		var profile []string
		for _, e := range entries {
			profile = append(profile, e.Name)
		}
		add("profile", profile)
	}
	keys := make([]string, 0, len(names))
	for n := range names {
		keys = append(keys, n)
	}
	sort.Strings(keys)
	for _, n := range keys {
		d.T.Println(n + "\t" + names[n])
	}
}

// packageNames lists the flake's packages without evaluating them, which nix answers from
// its evaluation cache in a few milliseconds.
func (d Deps) packageNames(repo string) []string {
	out, _, err := d.R.Capture(repo, "nix", "eval", "--json", ".#packages."+flake.System(), "--apply", "builtins.attrNames")
	if err != nil {
		return nil
	}
	var names []string
	json.Unmarshal([]byte(out), &names)
	return names
}

// completionRepo is the flake the line works on: -C if typed, else the usual lookup.
func (d Deps) completionRepo(fs *flag.FlagSet) (string, bool) {
	getenv := d.Env
	if getenv == nil {
		getenv = os.Getenv
	}
	repo, _, err := flake.Resolve(fs.Lookup("C").Value.String(), getenv)
	return repo, err == nil
}

// flagSet is the flag set a command defines, captured before it would run.
func (d Deps) flagSet(cmd string) *flag.FlagSet {
	var fs *flag.FlagSet
	d.inspect = func(f *flag.FlagSet) { fs = f }
	quiet := *d.T
	quiet.Err = io.Discard
	d.T = &quiet
	switch cmd {
	case "check":
		runCheck(nil, d)
	case "update":
		runUpdate(nil, d)
	case "list":
		runList(nil, d)
	case "search":
		runSearch(nil, d)
	}
	return fs
}

func cutLast(s, sep string) (before, after string) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):]
	}
	return "", s
}
