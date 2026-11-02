package main

import (
	"regexp"
	"strconv"
)

var (
	preRe   = regexp.MustCompile(`(?i)[-._]?(alpha|beta|rc|pre|nightly|snapshot)`)
	numRe   = regexp.MustCompile(`\d+`)
	leadRe  = regexp.MustCompile(`^\D*`)
	tagLike = regexp.MustCompile(`^(?:[A-Za-z]+/)?v?\d`)
)

// Version is a loosely parsed version: "v1.2.3", "dms/1.11.6-dms.4", "0.1.0-alpha.7".
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

func stripTag(tag string) string { return leadRe.ReplaceAllString(tag, "") }

func ParseVersion(s string) Version {
	s = stripTag(s)
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
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// Compare returns 1, 0 or -1. A prerelease sorts below its final release.
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

func newer(a, b string) bool { return ParseVersion(a).Compare(ParseVersion(b)) > 0 }

func isPre(s string) bool { return ParseVersion(s).pre }

// tagPrefix is the text before the version: "v1.2" -> "", "dms/1.2" -> "dms/".
func tagPrefix(tag string) string {
	m := regexp.MustCompile(`^(.*?)v?\d`).FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return m[1]
}

// bump classifies the jump from -> to. 0.x minor jumps count as breaking by convention.
func bump(from, to string) string {
	a, b := ParseVersion(from).nums, ParseVersion(to).nums
	at := func(n []int, i int) int {
		if i < len(n) {
			return n[i]
		}
		return 0
	}
	switch {
	case at(a, 0) != at(b, 0):
		return "major"
	case at(a, 0) == 0 && at(a, 1) != at(b, 1):
		return "0.x-minor"
	case at(a, 1) != at(b, 1):
		return "minor"
	}
	return "patch"
}

func bumpWarning(kind string) string {
	switch kind {
	case "major":
		return "MAJOR version jump: read the changelog, expect breaking changes"
	case "0.x-minor":
		return "0.x minor jump: may be breaking"
	}
	return ""
}
