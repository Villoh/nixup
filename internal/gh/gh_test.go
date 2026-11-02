package gh_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/testutil"
)

func TestRepoFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/PabloStarOk/padpoll/releases/download/v1.1.0/padpoll-linux.tar.gz": "PabloStarOk/padpoll",
		"https://github.com/wealthfolio/wealthfolio/archive/v3.11.0.tar.gz":                    "wealthfolio/wealthfolio",
		"git@github.com:Villoh/nixup.git":                                                      "Villoh/nixup",
		"https://github.com/lgse/strata":                                                       "lgse/strata",
		"https://wealthfolio.app/":                                                             "",
		"":                                                                                     "",
	}
	for in, want := range cases {
		if got := gh.RepoFromURL(in); got != want {
			t.Errorf("RepoFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTagsParsesAndCaches(t *testing.T) {
	e := testutil.New(t)
	e.Tags("o/r", "v1.0.0", "v1.1.0")
	for range 3 {
		tags, err := e.GH.Tags("o/r")
		if err != nil || len(tags) != 2 || tags[1] != "v1.1.0" {
			t.Fatalf("tags=%v err=%v", tags, err)
		}
	}
	if n := e.Run.Count("git ls-remote"); n != 1 {
		t.Errorf("ls-remote ran %d times, want 1 (cached)", n)
	}
}

func TestTagsError(t *testing.T) {
	e := testutil.New(t)
	e.Run.On(run.Rule{Prefix: "git ls-remote", Fail: errors.New("exit 128"), Err: "fatal: repository not found\n"})
	_, err := e.GH.Tags("o/gone")
	if err == nil || !strings.Contains(err.Error(), "repository not found") {
		t.Errorf("err = %v", err)
	}
}

func TestLatestTag(t *testing.T) {
	e := testutil.New(t)
	e.Tags("o/r", "v1.0.0", "v1.10.0", "v1.9.0", "v2.0.0-rc.1", "dms/3.0.0", "nightly", "v1.1.1-rc.3")
	cases := []struct {
		prefix   string
		allowPre bool
		want     string
	}{
		{"", false, "v1.10.0"},    // numeric order, no prerelease, no other prefix
		{"", true, "v2.0.0-rc.1"}, // prereleases allowed
		{"dms/", false, "dms/3.0.0"},
	}
	for _, c := range cases {
		got, err := e.GH.LatestTag("o/r", c.prefix, c.allowPre)
		if err != nil || got != c.want {
			t.Errorf("LatestTag(%q, %v) = %q, %v; want %q", c.prefix, c.allowPre, got, err, c.want)
		}
	}
	if _, err := e.GH.LatestTag("o/r", "zzz/", false); err == nil {
		t.Error("no matching tag must be an error")
	}
}

func TestTagFor(t *testing.T) {
	e := testutil.New(t)
	e.Tags("o/r", "v1.1.12", "1.01", "dms/1.2.0")
	if got := e.GH.TagFor("o/r", "1.1.12"); got != "v1.1.12" {
		t.Errorf("got %q", got)
	}
	if got := e.GH.TagFor("o/r", "1.01"); got != "1.01" {
		t.Errorf("got %q", got)
	}
	if got := e.GH.TagFor("o/r", "9.9.9"); got != "v9.9.9" {
		t.Errorf("unknown versions fall back to v<version>, got %q", got)
	}
}

func TestHead(t *testing.T) {
	e := testutil.New(t)
	e.Head("o/r", "integration", "abc123")
	e.Head("o/r", "", "def456")
	if got, err := e.GH.Head("o/r", "integration"); err != nil || got != "abc123" {
		t.Errorf("branch head = %q, %v", got, err)
	}
	if got, err := e.GH.Head("o/r", ""); err != nil || got != "def456" {
		t.Errorf("default head = %q, %v", got, err)
	}
	if _, err := e.GH.Head("o/r", "missing"); err == nil {
		t.Error("an unknown branch must be an error")
	}
}

func TestGetAuthAndErrors(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/boom" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"n": 7}`))
	}))
	defer srv.Close()

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "secret")
	c := gh.New(&run.Fake{})
	c.Base = srv.URL
	var v struct{ N int }
	if err := c.Get("ok", &v); err != nil || v.N != 7 {
		t.Fatalf("v=%+v err=%v", v, err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if err := c.Get("boom", &v); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("non-200 must be an error naming the status: %v", err)
	}
}

func TestTokenFromGhCLI(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	f := (&run.Fake{}).On(run.Rule{Prefix: "gh auth token", Out: "from-gh\n"})
	c := gh.New(f)
	c.Base = srv.URL
	c.Get("x", &struct{}{})
	c.Get("y", &struct{}{})
	if gotAuth != "Bearer from-gh" || f.Count("gh auth token") != 1 {
		t.Errorf("auth=%q, gh calls=%d (want the token read once)", gotAuth, f.Count("gh auth token"))
	}
}

func TestReviewReadsLastPageOfCommits(t *testing.T) {
	e := testutil.New(t)
	e.Compare("o/r", "v1.0.0", "v2.0.0", 250, "first", "second", "third")
	r := e.GH.Review("o/r", "v1.0.0", "v2.0.0", false)
	if r.Err != "" || r.Commits != 250 {
		t.Fatalf("review: %+v", r)
	}
	if strings.Join(r.Subjects, ",") != "third,second,first" { // newest first
		t.Errorf("subjects = %v", r.Subjects)
	}
	reqs := strings.Join(e.Requests(), " ")
	if !strings.Contains(reqs, "per_page=1") || !strings.Contains(reqs, "page=3") {
		t.Errorf("250 commits live on page 3 of 100; requests: %s", reqs)
	}
	if r.CompareURL != "https://github.com/o/r/compare/v1.0.0...v2.0.0" {
		t.Errorf("compare url = %s", r.CompareURL)
	}
}

func TestReviewCapsSubjects(t *testing.T) {
	e := testutil.New(t)
	var subjects []string
	for i := range 30 {
		subjects = append(subjects, "commit "+string(rune('a'+i%26)))
	}
	e.Compare("o/r", "a", "b", 30, subjects...)
	if r := e.GH.Review("o/r", "a", "b", false); len(r.Subjects) != 8 {
		t.Errorf("%d subjects, want 8", len(r.Subjects))
	}
}

func TestReviewShortensShaInLink(t *testing.T) {
	e := testutil.New(t)
	from, to := strings.Repeat("a", 40), strings.Repeat("b", 40)
	e.Compare("o/r", from, to, 2, "x")
	r := e.GH.Review("o/r", from, to, false)
	if want := "https://github.com/o/r/compare/aaaaaaaaaaaa...bbbbbbbbbbbb"; r.CompareURL != want {
		t.Errorf("link = %s, want %s", r.CompareURL, want)
	}
}

func TestReviewReleases(t *testing.T) {
	e := testutil.New(t)
	e.Compare("o/r", "v1.0.0", "v1.3.0", 9, "x")
	e.Releases("o/r",
		testutil.Release{Tag: "v1.4.0", Body: "too new"},
		testutil.Release{Tag: "v1.3.0", Body: "three"},
		testutil.Release{Tag: "v1.2.0-rc.1", Body: "rc", Prerelease: true},
		testutil.Release{Tag: "v1.2.0", Body: "two"},
		testutil.Release{Tag: "v1.1.0", Body: "draft", Draft: true},
		testutil.Release{Tag: "v1.0.0", Body: "already installed"},
		testutil.Release{Tag: "v0.9.0", Body: "older"},
	)
	r := e.GH.Review("o/r", "v1.0.0", "v1.3.0", true)
	var tags []string
	for _, n := range r.Releases {
		tags = append(tags, n.Tag)
	}
	if strings.Join(tags, ",") != "v1.3.0,v1.2.0" {
		t.Errorf("releases = %v; want only final releases in (from, to], newest first", tags)
	}
	if r.SkippedPre != 1 {
		t.Errorf("skipped prereleases = %d, want 1", r.SkippedPre)
	}
}

func TestReviewKeepsReleasesWhenCompareFails(t *testing.T) {
	e := testutil.New(t)
	e.Releases("o/r", testutil.Release{Tag: "v1.1.0", Body: "notes"})
	r := e.GH.Review("o/r", "v1.0.0", "v1.1.0", true) // no compare route: 404
	if r.Err == "" || !strings.Contains(r.Err, "404") {
		t.Errorf("err = %q", r.Err)
	}
	if len(r.Releases) != 1 {
		t.Error("release notes are still useful when the compare call fails")
	}
}
