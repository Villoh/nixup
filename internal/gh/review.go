package gh

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Villoh/nixup/internal/ver"
)

// Review is what a human wants to read before accepting an update.
type Review struct {
	Repo       string   `json:"repo"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	CompareURL string   `json:"compare_url,omitempty"`
	Commits    int      `json:"commits"`
	Files      int      `json:"files,omitempty"` // changed files; GitHub lists at most 300
	Additions  int      `json:"additions,omitempty"`
	Deletions  int      `json:"deletions,omitempty"`
	Subjects   []string `json:"subjects,omitempty"`
	Releases   []Note   `json:"releases,omitempty"`
	SkippedPre int      `json:"skipped_prereleases,omitempty"`
	Err        string   `json:"error,omitempty"`
}

type Note struct {
	Tag  string `json:"tag"`
	Body string `json:"body"`
}

const maxSubjects = 8

// MaxFiles is how many changed files the compare API lists, on its first page only.
const MaxFiles = 300

type compareResp struct {
	Total int `json:"total_commits"`
	Files []struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
	} `json:"files"`
	Commits []struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	} `json:"commits"`
}

type releaseResp struct {
	Tag        string `json:"tag_name"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// shortRef shortens commit SHAs for display and links; tags are left alone.
func shortRef(ref string) string {
	if len(ref) == 40 && !strings.ContainsAny(ref, "./-_") {
		return ref[:12]
	}
	return ref
}

// Review compares two refs (tags or commits). With releases=true it also collects the
// release notes published between them (both refs must then be tags).
func (c *Client) Review(repo, from, to string, releases bool) *Review {
	r := &Review{Repo: repo, From: from, To: to,
		CompareURL: fmt.Sprintf("https://github.com/%s/compare/%s...%s", repo, shortRef(from), shortRef(to))}

	// The compare API lists commits oldest first, 100 per page: read the total, then the last page.
	base := fmt.Sprintf("repos/%s/compare/%s...%s", repo, from, to)
	var cmp compareResp
	if err := c.Get(base+"?per_page=1", &cmp); err != nil {
		r.Err = err.Error()
	} else {
		r.Commits = cmp.Total
		r.Files = len(cmp.Files)
		for _, f := range cmp.Files {
			r.Additions += f.Additions
			r.Deletions += f.Deletions
		}
		page := max((cmp.Total+99)/100, 1)
		var last compareResp
		if c.Get(fmt.Sprintf("%s?per_page=100&page=%d", base, page), &last) == nil {
			for i := len(last.Commits) - 1; i >= 0 && len(r.Subjects) < maxSubjects; i-- {
				subject, _, _ := strings.Cut(last.Commits[i].Commit.Message, "\n")
				r.Subjects = append(r.Subjects, subject)
			}
		}
	}
	if releases {
		c.addReleases(r)
	}
	return r
}

func (c *Client) addReleases(r *Review) {
	var rels []releaseResp
	if err := c.Get(fmt.Sprintf("repos/%s/releases?per_page=50", r.Repo), &rels); err != nil {
		return
	}
	sort.Slice(rels, func(i, j int) bool { return ver.Newer(rels[i].Tag, rels[j].Tag) })
	for _, x := range rels {
		inRange := ver.Newer(x.Tag, r.From) && !ver.Newer(x.Tag, r.To)
		switch {
		case !inRange || x.Draft:
		case x.Prerelease || ver.IsPre(x.Tag):
			r.SkippedPre++
		default:
			r.Releases = append(r.Releases, Note{x.Tag, x.Body})
		}
	}
}
