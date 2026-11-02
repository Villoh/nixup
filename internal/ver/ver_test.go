package ver

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v2.0.0", "v1.6.2", true},
		{"v1.6.2", "v2.0.0", false},
		{"1.11.6-dms.4", "1.11.6-dms.2", true},
		{"0.1.0", "0.1.0-alpha.7", true}, // a final release beats its prerelease
		{"0.1.0-alpha.7", "0.1.0", false},
		{"0.1.0-alpha.8", "0.1.0-alpha.7", true},
		{"v1.1.1-rc.3", "v1.1.1", false},
		{"v1.1.1-rc.3", "v1.1.1-rc.2", true},
		{"1.01", "1.01", false},
		{"v1.10.0", "v1.9.0", true}, // numeric, not lexical
		{"dms/1.2.0", "1.1.9", true},
		{"1.2.3.4", "1.2.3", true},
		{"1.2.3", "1.2.3.0", false}, // missing components count as zero
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestIsPre(t *testing.T) {
	pre := []string{"v1.1.1-rc.1", "0.1.0-alpha.7", "2.0.0-beta2", "1.0.1-nightly.20261015", "3.0-preview", "1.0-dev", "v1.0.0-snapshot"}
	final := []string{"v1.1.1", "1.01", "1.11.6-dms.4", "1.2.3-arch", "1.0.0-mercury", "v3.11.0",
		"1.0.0-src", "3.2-xdev", "2.0-bpre.1"} // the marker must start a segment, not end a word
	for _, s := range pre {
		if !IsPre(s) {
			t.Errorf("IsPre(%q) = false", s)
		}
	}
	for _, s := range final {
		if IsPre(s) {
			t.Errorf("IsPre(%q) = true", s)
		}
	}
}

func TestStripAndPrefix(t *testing.T) {
	cases := []struct{ tag, stripped, prefix string }{
		{"v1.6.2", "1.6.2", ""},
		{"1.01", "1.01", ""},
		{"dms/1.11.6-dms.4", "1.11.6-dms.4", "dms/"},
		{"release-2.0", "2.0", "release-"},
	}
	for _, c := range cases {
		if got := Strip(c.tag); got != c.stripped {
			t.Errorf("Strip(%q) = %q, want %q", c.tag, got, c.stripped)
		}
		if got := Prefix(c.tag); got != c.prefix {
			t.Errorf("Prefix(%q) = %q, want %q", c.tag, got, c.prefix)
		}
	}
}

func TestLooks(t *testing.T) {
	for _, ref := range []string{"v1.6.2", "1.01", "dms/1.2", "v2"} {
		if !Looks(ref) {
			t.Errorf("Looks(%q) = false", ref)
		}
	}
	// Branches must not be mistaken for release tags.
	for _, ref := range []string{"nixos-unstable", "release-25.11", "main", "integration", ""} {
		if Looks(ref) {
			t.Errorf("Looks(%q) = true", ref)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	cases := []struct {
		tag, prefix string
		want        bool
	}{
		{"v1.2", "", true},
		{"1.2", "", true},
		{"dms/1.2", "", false}, // a monorepo tag is not a tag of the root package
		{"dms/1.2", "dms/", true},
		{"dms/v1.2", "dms/", true},
		{"other/1.2", "dms/", false},
		{"latest", "", false},
	}
	for _, c := range cases {
		if got := HasPrefix(c.tag, c.prefix); got != c.want {
			t.Errorf("HasPrefix(%q, %q) = %v, want %v", c.tag, c.prefix, got, c.want)
		}
	}
}

func TestBump(t *testing.T) {
	cases := []struct {
		from, to string
		want     Kind
		warns    bool
	}{
		{"0.21.0", "1.1.1", Major, true},
		{"v1.6.2", "v2.0.0", Major, true},
		{"0.21.0", "0.22.0", ZeroMinor, true},
		{"0.21.0", "0.21.1", Patch, false},
		{"1.1.0", "1.2.0", Minor, false},
		{"1.1.12", "1.1.13", Patch, false},
		{"1.01", "1.02", Minor, false}, // two components: 1.01 is [1 1]
	}
	for _, c := range cases {
		got := Bump(c.from, c.to)
		if got != c.want {
			t.Errorf("Bump(%q, %q) = %s, want %s", c.from, c.to, got, c.want)
		}
		if (got.Warning() != "") != c.warns {
			t.Errorf("Bump(%q, %q).Warning() = %q, warns=%v", c.from, c.to, got.Warning(), c.warns)
		}
	}
}
