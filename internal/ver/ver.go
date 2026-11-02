// Package ver compares the loose version strings found in git tags and nix packages:
// "v1.2.3", "dms/1.11.6-dms.4", "0.1.0-alpha.7", "1.01".
package ver

import (
	"regexp"
	"strconv"
)

var (
	// A prerelease marker must start a segment ("-rc.1", ".beta2") so that "1.2.3-arch" is not one.
	preRe    = regexp.MustCompile(`(?i)(?:^|[-._])(alpha|beta|rc|preview|pre|dev|nightly|snapshot)(?:[-._\d]|$)`)
	numRe    = regexp.MustCompile(`\d+`)
	leadRe   = regexp.MustCompile(`^\D*`)
	looksRe  = regexp.MustCompile(`^(?:[A-Za-z]+/)?v?\d`)
	prefixRe = regexp.MustCompile(`^(.*?)v?\d`)
)

// Version is a parsed version; a prerelease sorts below its final release.
type Version struct {
	nums    []int
	pre     bool
	preNums []int
}

func ints(s string) []int {
	var out []int
	for _, m := range numRe.FindAllString(s, -1) {
		n, _ := strconv.Atoi(m)
		out = append(out, n)
	}
	return out
}

// Strip removes whatever precedes the first digit: "v1.2" -> "1.2", "dms/1.2" -> "1.2".
func Strip(tag string) string { return leadRe.ReplaceAllString(tag, "") }

func Parse(s string) Version {
	s = Strip(s)
	if loc := preRe.FindStringIndex(s); loc != nil {
		return Version{nums: ints(s[:loc[0]]), pre: true, preNums: ints(s[loc[0]:])}
	}
	return Version{nums: ints(s)}
}

func cmpInts(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		switch {
		case x > y:
			return 1
		case x < y:
			return -1
		}
	}
	return 0
}

// Compare returns 1, 0 or -1.
func (v Version) Compare(o Version) int {
	if c := cmpInts(v.nums, o.nums); c != 0 {
		return c
	}
	switch {
	case v.pre && !o.pre:
		return -1
	case !v.pre && o.pre:
		return 1
	}
	return cmpInts(v.preNums, o.preNums)
}

// Newer reports whether a is a higher version than b.
func Newer(a, b string) bool { return Parse(a).Compare(Parse(b)) > 0 }

func IsPre(s string) bool { return Parse(s).pre }

// Looks reports whether a git ref reads as a release tag rather than a branch name.
// "v1.6.2" and "dms/1.2" do; "nixos-unstable" and "release-25.11" do not.
func Looks(ref string) bool { return looksRe.MatchString(ref) }

// Prefix is the text before the version: "v1.2" -> "", "dms/1.2" -> "dms/".
func Prefix(tag string) string {
	m := prefixRe.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return m[1]
}

// HasPrefix reports whether tag is `prefix`, an optional "v", then a digit.
// With an empty prefix, "v1.2" matches but "dms/1.2" does not.
func HasPrefix(tag, prefix string) bool {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `v?\d`).MatchString(tag)
}

// Kind classifies a version jump.
type Kind string

const (
	Major     Kind = "major"
	ZeroMinor Kind = "0.x-minor"
	Minor     Kind = "minor"
	Patch     Kind = "patch"
)

// Bump classifies from -> to. A minor jump while still at 0.x counts as breaking, by convention.
func Bump(from, to string) Kind {
	a, b := Parse(from).nums, Parse(to).nums
	at := func(n []int, i int) int {
		if i < len(n) {
			return n[i]
		}
		return 0
	}
	switch {
	case at(a, 0) != at(b, 0):
		return Major
	case at(a, 0) == 0 && at(a, 1) != at(b, 1):
		return ZeroMinor
	case at(a, 1) != at(b, 1):
		return Minor
	}
	return Patch
}

// Warning is the text shown for jumps that are probably breaking, or "".
func (k Kind) Warning() string {
	switch k {
	case Major:
		return "MAJOR version jump: read the changelog, expect breaking changes"
	case ZeroMinor:
		return "0.x minor jump: may be breaking"
	}
	return ""
}
