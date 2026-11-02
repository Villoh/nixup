// Package testutil builds the fakes the tests share: a scripted command runner, a fake
// GitHub API and temporary flake trees. It is imported by tests only.
package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Villoh/nixup/internal/gh"
	"github.com/Villoh/nixup/internal/run"
	"github.com/Villoh/nixup/internal/ui"
)

// Env is a fake world: commands answer from Run, the GitHub API from routes.
type Env struct {
	T   *testing.T
	Run *run.Fake
	GH  *gh.Client

	mu       sync.Mutex
	routes   map[string]any
	requests []string
}

func New(t *testing.T) *Env {
	t.Helper()
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	e := &Env{T: t, Run: &run.Fake{Missing: map[string]bool{"gh": true}}, routes: map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.requests = append(e.requests, r.URL.RequestURI())
		v, ok := e.routes[strings.TrimPrefix(r.URL.Path, "/")]
		e.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(srv.Close)
	e.GH = gh.New(e.Run)
	e.GH.Base = srv.URL
	return e
}

// Requests are the API requests received so far, path and query.
func (e *Env) Requests() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.requests...)
}

// API answers GET <path> with v as JSON.
func (e *Env) API(path string, v any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.routes[path] = v
}

// Tags makes `git ls-remote --tags` of a repo list these tags.
func (e *Env) Tags(repo string, tags ...string) {
	var b strings.Builder
	for i, t := range tags {
		fmt.Fprintf(&b, "%040d\trefs/tags/%s\n", i+1, t)
	}
	e.Run.On(run.Rule{Prefix: "git ls-remote --tags --refs https://github.com/" + repo, Out: b.String()})
}

// Head makes `git ls-remote <repo> <ref>` return sha for the branch (ref "" is HEAD).
func (e *Env) Head(repo, ref, sha string) {
	pattern, line := ref, sha+"\trefs/heads/"+ref+"\n"
	if ref == "" {
		pattern, line = "HEAD", sha+"\tHEAD\n"
	}
	e.Run.On(run.Rule{Prefix: "git ls-remote https://github.com/" + repo + " " + pattern, Out: line})
}

// Compare answers the compare API: total commits, subjects oldest first.
func (e *Env) Compare(repo, from, to string, total int, subjects ...string) {
	var commits []map[string]any
	for _, s := range subjects {
		commits = append(commits, map[string]any{"commit": map[string]any{"message": s + "\n\nbody"}})
	}
	e.API(fmt.Sprintf("repos/%s/compare/%s...%s", repo, from, to),
		map[string]any{"total_commits": total, "commits": commits})
}

type Release struct {
	Tag, Body         string
	Prerelease, Draft bool
}

func (e *Env) Releases(repo string, rels ...Release) {
	var out []map[string]any
	for _, r := range rels {
		out = append(out, map[string]any{"tag_name": r.Tag, "body": r.Body, "prerelease": r.Prerelease, "draft": r.Draft})
	}
	e.API("repos/"+repo+"/releases", out)
}

// Tree writes files (relative path -> content) into a temp dir and returns it.
func Tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Term is a non-interactive terminal capturing output; answers feeds Confirm when set.
func Term(answers string) (*ui.Term, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	t := &ui.Term{Out: &out, Err: &errb}
	if answers != "" {
		t.In, t.Interactive = strings.NewReader(answers), true
	}
	return t, &out, &errb
}

// LockJSON builds a minimal flake.lock. inputs maps name -> "owner/repo@ref#rev" (ref may be empty).
func LockJSON(inputs map[string]string) string {
	nodes := map[string]any{}
	root := map[string]any{}
	for name, spec := range inputs {
		repo, rest, _ := strings.Cut(spec, "@")
		ref, rev, _ := strings.Cut(rest, "#")
		owner, rname, _ := strings.Cut(repo, "/")
		orig := map[string]any{"type": "github", "owner": owner, "repo": rname}
		if ref != "" {
			orig["ref"] = ref
		}
		nodes[name] = map[string]any{"locked": map[string]any{"rev": rev, "lastModified": 1700000000}, "original": orig}
		root[name] = name
	}
	nodes["root"] = map[string]any{"inputs": root}
	b, _ := json.Marshal(map[string]any{"root": "root", "version": 7, "nodes": nodes})
	return string(b)
}
