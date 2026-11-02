package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	stOK       = "ok"
	stOutdated = "outdated"
	stInfo     = "info"
	stError    = "error"
)

type Row struct {
	Section string  `json:"section"`
	Name    string  `json:"name"`
	Current string  `json:"current"`
	Latest  string  `json:"latest,omitempty"`
	Status  string  `json:"status"`
	Note    string  `json:"note,omitempty"`
	Warn    string  `json:"warning,omitempty"`
	Review  *Review `json:"review,omitempty"`
}

// tagCache caches ls-remote per repo: several packages and inputs may share one.
var tagCache sync.Map

func cachedTags(repo string) ([]string, error) {
	if v, ok := tagCache.Load(repo); ok {
		return v.([]string), nil
	}
	t, err := remoteTags(repo)
	if err == nil {
		tagCache.Store(repo, t)
	}
	return t, err
}

// tagFor finds the upstream tag that carries `version` ("1.1.12" -> "v1.1.12").
func tagFor(repo, version string) string {
	tags, _ := cachedTags(repo)
	for _, t := range tags {
		if stripTag(t) == version {
			return t
		}
	}
	return "v" + version
}

// parallel runs fn over n items with a small worker pool, keeping result order.
func parallel[T any](n int, fn func(i int) T) []T {
	out := make([]T, n)
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = fn(i)
		}()
	}
	wg.Wait()
	return out
}

func errRow(section, name string, err error) Row {
	return Row{Section: section, Name: name, Status: stError, Note: err.Error()}
}

// checkInputs: tag-pinned inputs against the newest tag, branch inputs against the remote head.
func checkInputs(repo string, review bool) []Row {
	l, err := readLock(repo + "/flake.lock")
	if err != nil {
		return []Row{errRow("inputs", "flake.lock", err)}
	}
	direct := l.direct()
	var names []string
	for _, n := range l.names() {
		if l.Nodes[direct[n]].Original.Type == "github" {
			names = append(names, n)
		}
	}
	return parallel(len(names), func(i int) Row {
		name := names[i]
		node := l.Nodes[direct[name]]
		id := node.Original.Owner + "/" + node.Original.Repo
		ref, rev := node.Original.Ref, node.Locked.Rev
		age := int(time.Since(time.Unix(node.Locked.LastModified, 0)).Hours() / 24)
		if ref != "" && tagLike.MatchString(ref) { // release tag: `flake update` never moves it
			top, err := latestTag(id, tagPrefix(ref), isPre(ref))
			if err != nil {
				return errRow("inputs", name, err)
			}
			if !newer(top, ref) {
				return Row{Section: "inputs", Name: name, Current: ref, Status: stOK, Note: "pinned tag"}
			}
			row := Row{Section: "inputs", Name: name, Current: ref, Latest: top, Status: stOutdated,
				Note: "pinned tag: edit flake.nix", Warn: bumpWarning(bump(ref, top))}
			if review {
				row.Review = buildReview(id, ref, top, true)
			}
			return row
		}
		head, err := remoteHead(id, ref)
		if err != nil {
			return errRow("inputs", name, err)
		}
		if head == rev {
			return Row{Section: "inputs", Name: name, Current: short(rev), Status: stOK}
		}
		row := Row{Section: "inputs", Name: name, Current: fmt.Sprintf("%s (%dd)", short(rev), age),
			Latest: short(head), Status: stOutdated, Note: "nixup lock " + name}
		if review {
			row.Review = buildReview(id, rev, head, false)
		}
		return row
	})
}

// checkPackages: packages defined in the flake's own `packages` output.
func checkPackages(repo string, review bool) []Row {
	pkgs, err := ownPackages(repo)
	if err != nil {
		return []Row{errRow("packages", "packages", err)}
	}
	return parallel(len(pkgs), func(i int) Row { return checkPackage(pkgs[i], review) })
}

func checkPackage(p ownPackage, review bool) Row {
	row := Row{Section: "packages", Name: p.Name, Current: p.Version}
	id := p.upstream()
	if id == "" {
		row.Status, row.Note = stInfo, "no GitHub upstream: check by hand"
		return row
	}
	if len(p.Update) == 0 {
		row.Status, row.Note = stInfo, "no passthru.updateScript"
		return row
	}
	_, allowPre := p.extraArgs()
	top, err := latestTag(id, "", allowPre || isPre(p.Version))
	if err != nil {
		return errRow("packages", p.Name, err)
	}
	if !newer(top, p.Version) {
		row.Status = stOK
		return row
	}
	row.Latest, row.Status, row.Warn = stripTag(top), stOutdated, bumpWarning(bump(p.Version, top))
	row.Note = "nixup pkg " + p.Name
	if review {
		row.Review = buildReview(id, tagFor(id, p.Version), top, true)
	}
	return row
}

var pinnedNixpkgs = regexp.MustCompile(`/([0-9a-f]{40})`)

// checkProfile: `nix profile` entries pinned to a nixpkgs other than the system one.
func checkProfile(repo string) []Row {
	if !have("nix") {
		return nil
	}
	out, errs, err := capture("", "nix", "profile", "list", "--json")
	if err != nil {
		return []Row{{Section: "profile", Name: "nix profile", Status: stError, Note: lastLine(errs)}}
	}
	var data struct {
		Elements map[string]struct {
			OriginalURL string `json:"originalUrl"`
		} `json:"elements"`
	}
	if json.Unmarshal([]byte(out), &data) != nil {
		return nil
	}
	system := ""
	if l, err := readLock(repo + "/flake.lock"); err == nil {
		system = l.Nodes[l.direct()["nixpkgs"]].Locked.Rev
	}
	var names []string
	for n := range data.Elements {
		names = append(names, n)
	}
	sort.Strings(names)
	var rows []Row
	for _, n := range names {
		m := pinnedNixpkgs.FindStringSubmatch(data.Elements[n].OriginalURL)
		switch {
		case m == nil:
			rows = append(rows, Row{Section: "profile", Name: n, Current: "tracks flake ref", Status: stOK,
				Note: "nix profile upgrade " + n})
		case system != "" && m[1] != system:
			rows = append(rows, Row{Section: "profile", Name: n, Current: short(m[1]), Latest: short(system),
				Status: stOutdated, Note: "pinned to an old nixpkgs"})
		default:
			rows = append(rows, Row{Section: "profile", Name: n, Current: short(m[1]), Status: stOK})
		}
	}
	return rows
}

func checkFlatpak() []Row {
	if !have("flatpak") {
		return nil
	}
	out, errs, err := capture("", "flatpak", "remote-ls", "--updates", "--columns=application")
	if err != nil {
		return []Row{{Section: "flatpak", Name: "flatpak", Status: stError, Note: lastLine(errs)}}
	}
	var rows []Row
	seen := map[string]bool{} // one app can list several branches/arches
	for _, l := range strings.Split(out, "\n") {
		if a := strings.TrimSpace(l); a != "" && !seen[a] {
			seen[a] = true
			rows = append(rows, Row{Section: "flatpak", Name: a, Latest: "update", Status: stOutdated,
				Note: "flatpak update"})
		}
	}
	return rows
}
