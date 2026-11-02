package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxNotes = 3

var (
	tokenOnce sync.Once
	token     string
	httpc     = &http.Client{Timeout: 25 * time.Second}
	ghRepoRe  = regexp.MustCompile(`github\.com[/:]([^/]+)/([^/#?]+?)(?:\.git)?(?:/|$)`)
)

func ghToken() string {
	tokenOnce.Do(func() {
		token = firstNonEmpty(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))
		if token == "" && have("gh") {
			out, _, _ := capture("", "gh", "auth", "token")
			token = strings.TrimSpace(out)
		}
	})
	return token
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// ghGet fetches api.github.com/<path> and decodes the JSON into v.
func ghGet(path string, v any) error {
	req, _ := http.NewRequest("GET", "https://api.github.com/"+path, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nixup")
	if t := ghToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("github %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// githubRepo extracts "owner/name" from a URL mentioning github.com.
func githubRepo(url string) string {
	if m := ghRepoRe.FindStringSubmatch(url); m != nil {
		return m[1] + "/" + m[2]
	}
	return ""
}

// remoteTags lists tag names of a GitHub repo through git (no token, no rate limit).
func remoteTags(repo string) ([]string, error) {
	out, errs, err := capture("", "git", "ls-remote", "--tags", "--refs", "https://github.com/"+repo)
	if err != nil {
		return nil, fmt.Errorf("git ls-remote %s: %s", repo, lastLine(errs))
	}
	var tags []string
	for _, line := range strings.Split(out, "\n") {
		if _, tag, ok := strings.Cut(line, "refs/tags/"); ok {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// latestTag returns the highest version-like tag with the given prefix.
func latestTag(repo, prefix string, allowPre bool) (string, error) {
	tags, err := remoteTags(repo)
	if err != nil {
		return "", err
	}
	best := ""
	for _, t := range tags {
		if !strings.HasPrefix(t, prefix) || !tagLike.MatchString(t) || (!allowPre && isPre(t)) {
			continue
		}
		if best == "" || newer(t, best) {
			best = t
		}
	}
	if best == "" {
		return "", fmt.Errorf("no matching tag in %s", repo)
	}
	return best, nil
}

// remoteHead is the commit of a branch (or of HEAD when ref is empty).
func remoteHead(repo, ref string) (string, error) {
	pat := firstNonEmpty(ref, "HEAD")
	out, errs, err := capture("", "git", "ls-remote", "https://github.com/"+repo, pat)
	if err != nil || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("ref %s not found in %s %s", pat, repo, lastLine(errs))
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && (ref == "" || f[1] == "refs/heads/"+ref) {
			return f[0], nil
		}
	}
	return strings.Fields(out)[0], nil
}

// Review is what a human wants to read before accepting an update.
type Review struct {
	Repo       string   `json:"repo"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	CompareURL string   `json:"compare_url,omitempty"`
	Commits    int      `json:"commits"`
	Subjects   []string `json:"subjects,omitempty"`
	Releases   []Note   `json:"releases,omitempty"`
	SkippedPre int      `json:"skipped_prereleases,omitempty"`
	Err        string   `json:"error,omitempty"`
}

type Note struct {
	Tag  string `json:"tag"`
	Body string `json:"body"`
}

// shortRef shortens commit SHAs for display and URLs; tags are left alone.
func shortRef(ref string) string {
	if len(ref) == 40 && !strings.ContainsAny(ref, "./-_") {
		return ref[:12]
	}
	return ref
}

// buildReview compares two refs (tags or commits). With tags=true it also collects the
// release notes published between them.
func buildReview(repo, from, to string, tags bool) *Review {
	r := &Review{Repo: repo, From: from, To: to,
		CompareURL: fmt.Sprintf("https://github.com/%s/compare/%s...%s", repo, shortRef(from), shortRef(to))}
	var cmp struct {
		TotalCount int `json:"total_commits"`
		Commits    []struct {
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"commits"`
	}
	// The API lists oldest first, 100 per page: read the count, then the last page.
	base := fmt.Sprintf("repos/%s/compare/%s...%s", repo, from, to)
	if err := ghGet(base+"?per_page=1", &cmp); err != nil {
		r.Err = err.Error()
	} else {
		r.Commits = cmp.TotalCount
		last := (cmp.TotalCount + 99) / 100
		if err := ghGet(fmt.Sprintf("%s?per_page=100&page=%d", base, max(last, 1)), &cmp); err == nil {
			for i := len(cmp.Commits) - 1; i >= 0 && len(r.Subjects) < 8; i-- {
				subj, _, _ := strings.Cut(cmp.Commits[i].Commit.Message, "\n")
				r.Subjects = append(r.Subjects, subj)
			}
		}
	}
	if !tags {
		return r
	}
	var rels []struct {
		Tag        string `json:"tag_name"`
		Body       string `json:"body"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	if err := ghGet(fmt.Sprintf("repos/%s/releases?per_page=50", repo), &rels); err != nil {
		return r
	}
	sort.Slice(rels, func(i, j int) bool { return newer(rels[i].Tag, rels[j].Tag) })
	for _, x := range rels {
		inRange := newer(x.Tag, from) && !newer(x.Tag, to)
		switch {
		case !inRange || x.Draft:
		case x.Prerelease || isPre(x.Tag):
			r.SkippedPre++
		default:
			r.Releases = append(r.Releases, Note{x.Tag, x.Body})
		}
	}
	return r
}

// Text renders the review for a terminal, indented under its row.
func (r *Review) Text(indent string) string {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, indent+f+"\n", a...) }
	if r.Err != "" {
		p("review unavailable: %s", r.Err)
		return b.String()
	}
	p("source   https://github.com/%s", r.Repo)
	p("changes  %d commits  %s", r.Commits, r.CompareURL)
	for _, s := range r.Subjects {
		p("  · %s", s)
	}
	for i, n := range r.Releases {
		if i == maxNotes {
			p("(+%d more releases: see the compare link)", len(r.Releases)-maxNotes)
			break
		}
		p("notes    %s", n.Tag)
		for _, l := range strings.Split(firstLines(strings.TrimSpace(n.Body), 6), "\n") {
			if strings.TrimSpace(l) != "" {
				p("         %s", strings.TrimSpace(l))
			}
		}
	}
	if r.SkippedPre > 0 {
		p("(%d prereleases in between not shown)", r.SkippedPre)
	}
	return b.String()
}

// Markdown renders the review for a PR body or step summary.
func (r *Review) Markdown() string {
	if r.Err != "" {
		return fmt.Sprintf("_review unavailable: %s_\n", r.Err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[%s](https://github.com/%s) · [%d commits](%s)\n\n", r.Repo, r.Repo, r.Commits, r.CompareURL)
	for _, s := range r.Subjects {
		fmt.Fprintf(&b, "- %s\n", s)
	}
	for i, n := range r.Releases {
		if i == maxNotes {
			break
		}
		fmt.Fprintf(&b, "\n<details><summary>%s</summary>\n\n%s\n\n</details>\n", n.Tag, firstLines(n.Body, 25))
	}
	return b.String()
}
