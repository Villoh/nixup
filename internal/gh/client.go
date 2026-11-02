// Package gh talks to GitHub: tags and branch heads through git (no token, no rate limit),
// commits and release notes through the REST API.
package gh

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ver"
)

type Client struct {
	Base string // API root; tests point it at an httptest server
	HTTP *http.Client
	Run  run.Runner

	tokenOnce sync.Once
	token     string
	tags      sync.Map // repo -> []string; several packages and inputs may share a repo
}

func New(r run.Runner) *Client {
	return &Client{Base: "https://api.github.com", HTTP: &http.Client{Timeout: 25 * time.Second}, Run: r}
}

// authToken is GH_TOKEN, GITHUB_TOKEN or `gh auth token`; empty means unauthenticated.
func (c *Client) authToken() string {
	c.tokenOnce.Do(func() {
		for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if c.token = os.Getenv(k); c.token != "" {
				return
			}
		}
		if c.Run.Has("gh") {
			out, _, _ := c.Run.Capture("", "gh", "auth", "token")
			c.token = strings.TrimSpace(out)
		}
	})
	return c.token
}

// Get fetches <Base>/<path> and decodes the JSON body into v.
func (c *Client) Get(path string, v any) error {
	req, err := http.NewRequest("GET", c.Base+"/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nixup")
	if t := c.authToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

var repoRe = regexp.MustCompile(`github\.com[/:]([^/]+)/([^/#?]+?)(?:\.git)?(?:/|$)`)

// RepoFromURL extracts "owner/name" from any URL that mentions github.com, or "".
func RepoFromURL(url string) string {
	if m := repoRe.FindStringSubmatch(url); m != nil {
		return m[1] + "/" + m[2]
	}
	return ""
}

// Tags lists the tag names of a repo.
func (c *Client) Tags(repo string) ([]string, error) {
	if v, ok := c.tags.Load(repo); ok {
		return v.([]string), nil
	}
	out, errs, err := c.Run.Capture("", "git", "ls-remote", "--tags", "--refs", "https://github.com/"+repo)
	if err != nil {
		return nil, fmt.Errorf("git ls-remote %s: %s", repo, run.LastLine(errs))
	}
	var tags []string
	for _, line := range strings.Split(out, "\n") {
		if _, tag, ok := strings.Cut(line, "refs/tags/"); ok {
			tags = append(tags, tag)
		}
	}
	c.tags.Store(repo, tags)
	return tags, nil
}

// LatestTag is the highest version-like tag that starts with prefix.
func (c *Client) LatestTag(repo, prefix string, allowPre bool) (string, error) {
	tags, err := c.Tags(repo)
	if err != nil {
		return "", err
	}
	best := ""
	for _, t := range tags {
		if !ver.HasPrefix(t, prefix) || (!allowPre && ver.IsPre(t)) {
			continue
		}
		if best == "" || ver.Newer(t, best) {
			best = t
		}
	}
	if best == "" {
		return "", fmt.Errorf("no matching tag in %s", repo)
	}
	return best, nil
}

// TagFor finds the upstream tag that carries a version ("1.1.12" -> "v1.1.12").
func (c *Client) TagFor(repo, version string) string {
	tags, _ := c.Tags(repo)
	for _, t := range tags {
		if ver.Strip(t) == version {
			return t
		}
	}
	return "v" + version
}

// Head is the commit a branch points to, or of the default branch when ref is empty.
func (c *Client) Head(repo, ref string) (string, error) {
	pattern := ref
	if pattern == "" {
		pattern = "HEAD"
	}
	out, errs, err := c.Run.Capture("", "git", "ls-remote", "https://github.com/"+repo, pattern)
	if err != nil || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("ref %s not found in %s %s", pattern, repo, run.LastLine(errs))
	}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) == 2 && (ref == "" || f[1] == "refs/heads/"+ref) {
			return f[0], nil
		}
	}
	return strings.Fields(out)[0], nil
}
