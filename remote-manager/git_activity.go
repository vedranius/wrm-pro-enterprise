// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── GIT ACTIVITY: pushes and commits per branch and author ───
//
// Read-only, from the provider API of the service's Git source (no SSH):
//   - GitLab: project events with action=pushed (every branch and tag) and the commits of
//     the listed branches;
//   - GitHub: the commits of the listed branches and, where the token allows it, the
//     repository activity (pushes, force pushes, branch creation and deletion, merges);
//   - Gitea: the commits of the listed branches.
//
// Listings follow the provider's next links up to gitActivityPages pages and stop early when
// the provider reports that few calls are left (rate limits); a cut listing is marked
// truncated. Results are cached per user, service, branch and date range for
// gitActivityTTL; the author filter and the summaries are computed from the cached listing.

const (
	gitActivityTTL       = 3 * time.Minute
	gitActivityPages     = 10 // pages per listing (100 items, 50 on Gitea)
	gitActivityBranches  = 6  // branches listed when no branch is chosen
	gitActivityMaxItems  = 5000
	gitActivityRateFloor = 5 // stop paging when the provider has this few calls left
	gitActivityDays      = 30
	gitActivityMaxDays   = 366
)

// ─── short-lived cache (activity, history and matrix cells) ───

type gitCacheEntry struct {
	at time.Time
	v  interface{}
}

var gitShortCache = struct {
	sync.Mutex
	m map[string]gitCacheEntry
}{m: map[string]gitCacheEntry{}}

func shortCacheGet(key string, ttl time.Duration) (interface{}, time.Time, bool) {
	gitShortCache.Lock()
	defer gitShortCache.Unlock()
	e, ok := gitShortCache.m[key]
	if !ok || time.Since(e.at) > ttl {
		return nil, time.Time{}, false
	}
	return e.v, e.at, true
}

func shortCachePut(key string, v interface{}) {
	gitShortCache.Lock()
	defer gitShortCache.Unlock()
	now := time.Now()
	if len(gitShortCache.m) > 500 {
		for k, e := range gitShortCache.m {
			if now.Sub(e.at) > 10*time.Minute {
				delete(gitShortCache.m, k)
			}
		}
	}
	gitShortCache.m[key] = gitCacheEntry{at: now, v: v}
}

// ─── web links ───────────────────────────────────────

// gitWeb builds links to the provider's web pages of a project.
type gitWeb struct {
	kind, base, project string
}

func newGitWeb(src gitSource, project string) gitWeb {
	base := strings.TrimRight(strings.TrimSpace(src.URL), "/")
	if i := strings.Index(asciiLower(base), "/api/"); i >= 0 {
		base = base[:i]
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return gitWeb{}
	}
	if src.Kind == "github" && strings.EqualFold(u.Hostname(), "api.github.com") {
		base = "https://github.com"
	}
	return gitWeb{kind: src.Kind, base: base, project: escapePathSegments(project)}
}

func nullSHA(s string) bool { return strings.Trim(s, "0") == "" }

func (w gitWeb) commit(sha string) string {
	if w.base == "" || sha == "" || nullSHA(sha) {
		return ""
	}
	if w.kind == "gitlab" {
		return w.base + "/" + w.project + "/-/commit/" + url.PathEscape(sha)
	}
	return w.base + "/" + w.project + "/commit/" + url.PathEscape(sha)
}

// compare links the commits from a (exclusive) to b.
func (w gitWeb) compare(a, b string) string {
	if w.base == "" || a == "" || b == "" || nullSHA(a) || nullSHA(b) {
		return ""
	}
	if w.kind == "gitlab" {
		return w.base + "/" + w.project + "/-/compare/" + url.PathEscape(a) + "..." + url.PathEscape(b)
	}
	return w.base + "/" + w.project + "/compare/" + url.PathEscape(a) + "..." + url.PathEscape(b)
}

// branch links the commit list of a branch.
func (w gitWeb) branch(b string) string {
	if w.base == "" || b == "" {
		return ""
	}
	switch w.kind {
	case "gitlab":
		return w.base + "/" + w.project + "/-/commits/" + escapePathSegments(b)
	case "gitea":
		return w.base + "/" + w.project + "/commits/branch/" + escapePathSegments(b)
	}
	return w.base + "/" + w.project + "/commits/" + escapePathSegments(b)
}

// ─── paging with rate limits ─────────────────────────

var errStopWalk = errors.New("stop")

func rateRemaining(h http.Header) string {
	for _, k := range []string{"RateLimit-Remaining", "X-RateLimit-Remaining"} {
		if v := strings.TrimSpace(h.Get(k)); v != "" {
			return v
		}
	}
	return ""
}

// rateReset is when the provider accepts calls again (RFC 3339), from Retry-After or the
// reset time of the rate limit headers.
func rateReset(h http.Header) string {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Now().Add(time.Duration(n) * time.Second).UTC().Format(time.RFC3339)
		}
	}
	for _, k := range []string{"RateLimit-Reset", "X-RateLimit-Reset"} {
		if n, err := strconv.ParseInt(strings.TrimSpace(h.Get(k)), 10, 64); err == nil && n > 1e9 {
			return time.Unix(n, 0).UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func isRateLimited(err error) (*gitHTTPError, bool) {
	var he *gitHTTPError
	if errors.As(err, &he) && (he.Status == 429 || (he.Status == 403 && he.Remaining == "0")) {
		return he, true
	}
	return nil, false
}

// linkNext returns the rel="next" URL of a Link header.
func linkNext(h string) string {
	for _, part := range strings.Split(h, ",") {
		segs := strings.Split(part, ";")
		if len(segs) < 2 {
			continue
		}
		for _, s := range segs[1:] {
			if strings.TrimSpace(s) == `rel="next"` {
				return strings.Trim(strings.TrimSpace(segs[0]), "<>")
			}
		}
	}
	return ""
}

// apiWalk pages through list endpoints and keeps what the provider said about its limits.
type apiWalk struct {
	p         *gitProvider
	truncated bool
	limited   bool
	reset     string
	remaining int // -1 = not reported
}

func newAPIWalk(p *gitProvider) *apiWalk { return &apiWalk{p: p, remaining: -1} }

// nextRel is the request of the next page: the provider's next link when it points to the
// API, GitLab's X-Next-Page, or "" for the last page.
func (w *apiWalk) nextRel(h http.Header, rel string, page int) string {
	if l := linkNext(h.Get("Link")); l != "" {
		if strings.HasPrefix(l, w.p.api+"/") {
			return strings.TrimPrefix(l, w.p.api)
		}
		return rel + w.p.pageQuery(page)
	}
	if w.p.kind == "gitlab" {
		if n := h.Get("X-Next-Page"); n != "" && n != "0" {
			return rel + w.p.pageQuery(page)
		}
	}
	return ""
}

// list calls fn for every page of rel (which already has a query string). fn may return
// errStopWalk to end the listing early.
func (w *apiWalk) list(rel string, maxPages int, fn func(raw json.RawMessage) (int, error)) error {
	next := rel + w.p.pageQuery(1)
	for page := 1; ; page++ {
		var raw json.RawMessage
		h, err := w.p.getJSON(next, &raw)
		if err != nil {
			if he, ok := isRateLimited(err); ok {
				w.limited, w.truncated = true, true
				if he.Reset != "" {
					w.reset = he.Reset
				}
			}
			return err
		}
		if v := rateRemaining(h); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				w.remaining = n
				if r := rateReset(h); r != "" {
					w.reset = r
				}
			}
		}
		n, err := fn(raw)
		if errors.Is(err, errStopWalk) {
			return nil
		}
		if err != nil {
			return err
		}
		nx := w.nextRel(h, rel, page+1)
		if n == 0 || nx == "" {
			return nil
		}
		if page >= maxPages {
			w.truncated = true
			return nil
		}
		if w.remaining >= 0 && w.remaining <= gitActivityRateFloor {
			w.limited, w.truncated = true, true
			return nil
		}
		next = nx
	}
}

// ─── activity ────────────────────────────────────────

type gitActivityItem struct {
	Kind     string   `json:"kind"` // commit | push
	At       string   `json:"at"`
	Author   string   `json:"author"`
	Login    string   `json:"login,omitempty"`
	Branch   string   `json:"branch,omitempty"`
	Branches []string `json:"branches,omitempty"` // a commit on several listed branches
	RefType  string   `json:"ref_type,omitempty"` // push: branch | tag
	Ref      string   `json:"ref,omitempty"`      // push: the branch or tag name
	SHA      string   `json:"sha,omitempty"`
	Title    string   `json:"title,omitempty"`
	Action   string   `json:"action,omitempty"` // push: pushed | created | removed | push | force_push | branch_creation | …
	Count    int      `json:"count,omitempty"`  // push: number of commits (GitLab)
	Before   string   `json:"before,omitempty"`
	After    string   `json:"after,omitempty"`
	URL      string   `json:"url,omitempty"`
	email    string
	at       time.Time
}

type gitActivitySummary struct {
	Key      string   `json:"key"` // author or branch
	Commits  int      `json:"commits"`
	Pushes   int      `json:"pushes"`
	Authors  int      `json:"authors,omitempty"`
	Branches []string `json:"branches,omitempty"`
	Last     string   `json:"last"`
	URL      string   `json:"url,omitempty"`
}

// gitActivityRaw is a cached listing (before the author filter).
type gitActivityRaw struct {
	App         string            `json:"app"`
	Project     string            `json:"project"`
	Provider    string            `json:"provider"`
	Since       string            `json:"since"`
	Until       string            `json:"until"`
	Branch      string            `json:"branch,omitempty"`
	AllBranches []string          `json:"branches"`
	Tracked     []string          `json:"tracked"`
	Items       []gitActivityItem `json:"-"`
	Truncated   bool              `json:"truncated"`
	RateLimited bool              `json:"rate_limited"`
	RateReset   string            `json:"rate_reset,omitempty"`
	Notes       []string          `json:"notes"`
	FetchedAt   string            `json:"fetched_at"`
	Calls       int               `json:"calls"`
	web         gitWeb
}

type gitActivityResult struct {
	gitActivityRaw
	Author   string               `json:"author,omitempty"`
	Cached   bool                 `json:"cached"`
	Items    []gitActivityItem    `json:"items"`
	Authors  []gitActivitySummary `json:"authors"`
	ByBranch []gitActivitySummary `json:"by_branch"`
}

type gitActivityQuery struct {
	App, Branch, Author string
	Since, Until        time.Time // dates (UTC); until is inclusive
	Refresh             bool
}

func parseDay(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}

// parseRange reads since / until (YYYY-MM-DD, until inclusive); empty values default to
// the last gitActivityDays days.
func parseRange(q url.Values, defDays int) (time.Time, time.Time, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	until, since := today, time.Time{}
	if v := q.Get("until"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			return since, until, fmt.Errorf("invalid end date %q (YYYY-MM-DD)", v)
		}
		until = t
	}
	if v := q.Get("since"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			return since, until, fmt.Errorf("invalid start date %q (YYYY-MM-DD)", v)
		}
		since = t
	} else if defDays > 0 {
		since = until.AddDate(0, 0, -defDays)
	}
	if !since.IsZero() && since.After(until) {
		return since, until, fmt.Errorf("the start date is after the end date")
	}
	return since, until, nil
}

func parseActivityQuery(q url.Values) (gitActivityQuery, error) {
	aq := gitActivityQuery{App: q.Get("app"), Branch: strings.TrimSpace(q.Get("branch")), Author: strings.TrimSpace(q.Get("author")), Refresh: q.Get("refresh") == "1"}
	var err error
	if aq.Since, aq.Until, err = parseRange(q, gitActivityDays); err != nil {
		return aq, err
	}
	if aq.Until.Sub(aq.Since) > gitActivityMaxDays*24*time.Hour {
		return aq, fmt.Errorf("choose at most %d days", gitActivityMaxDays)
	}
	if len(aq.Branch) > 250 || len(aq.Author) > 200 {
		return aq, fmt.Errorf("filter too long")
	}
	return aq, nil
}

// serviceSource returns the service and the API source used for it: the source of its
// target, or the workspace's API source.
func serviceSource(userID int, app string) (*gitCatalog, *gitCatalogApp, gitSource, error) {
	cat, st := loadGitWorkspace(userID)
	a, ok := cat.app(app)
	if !ok {
		return cat, nil, gitSource{}, fmt.Errorf("service %q not found", app)
	}
	if t, ok := loadGitTarget(userID, app); ok && t.SourceID > 0 {
		if src, found := loadGitSource(userID, t.SourceID); found && gitAPIKind(src.Kind) {
			return cat, a, src, nil
		}
	}
	api, _ := pickSources(userID, st)
	if api == nil || !gitAPIKind(api.Kind) {
		return cat, a, gitSource{}, fmt.Errorf("no Git server source (GitLab, GitHub or Gitea) is configured")
	}
	return cat, a, *api, nil
}

// trackedBranches picks the branches whose commits are listed when no branch is chosen:
// the service's branch, the target's, the default branch, the fallback branches and the
// branches of the environments' allowed refs, as far as they exist.
func trackedBranches(cat *gitCatalog, a *gitCatalogApp, t gitTarget, def string, all []string) []string {
	exists := map[string]bool{}
	for _, b := range all {
		exists[b] = true
	}
	cand := []string{a.Branch, t.Branch, def}
	cand = append(cand, cat.FallbackBranches...)
	for _, e := range a.Environments {
		for _, r := range e.AllowedRefs {
			if b := strings.TrimPrefix(r, "refs/heads/"); b != r && !strings.ContainsAny(b, "*?[") {
				cand = append(cand, b)
			}
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, b := range cand {
		b = strings.TrimSpace(b)
		if b == "" || seen[b] || (len(all) > 0 && !exists[b]) {
			continue
		}
		seen[b] = true
		out = append(out, b)
		if len(out) >= gitActivityBranches {
			break
		}
	}
	return out
}

func parseAPITime(s string) time.Time {
	for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000-07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(l)
}

// branchCommitsIn lists the commits of a branch between since and until (newest first).
func branchCommitsIn(w *apiWalk, project, branch string, since, end time.Time, web gitWeb) ([]gitActivityItem, error) {
	p := w.p
	iso := func(t time.Time) string { return url.QueryEscape(t.UTC().Format(time.RFC3339)) }
	rel := p.gl(project) + "/repository/commits?ref_name=" + url.QueryEscape(branch)
	if p.hub() {
		rel = p.gh(project) + "/commits?sha=" + url.QueryEscape(branch)
	}
	if p.kind == "gitea" {
		rel += "&stat=false&verification=false&files=false"
	}
	rel += "&since=" + iso(since) + "&until=" + iso(end)
	var out []gitActivityItem
	err := w.list(rel, gitActivityPages, func(raw json.RawMessage) (int, error) {
		var list []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			AuthorName    string `json:"author_name"`
			AuthorEmail   string `json:"author_email"`
			CommittedDate string `json:"committed_date"`
			SHA           string `json:"sha"`
			Commit        struct {
				Message string `json:"message"`
				Author  struct {
					Name  string `json:"name"`
					Email string `json:"email"`
					Date  string `json:"date"`
				} `json:"author"`
				Committer struct {
					Date string `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, fmt.Errorf("unexpected answer from the Git server: %v", err)
		}
		older := false
		for _, c := range list {
			it := gitActivityItem{Kind: "commit", Branch: branch}
			if c.ID != "" {
				it.SHA, it.Title, it.Author, it.email = c.ID, firstLine(c.Title), c.AuthorName, c.AuthorEmail
				it.at = parseAPITime(c.CommittedDate)
			} else {
				it.SHA, it.Title, it.Author, it.email = c.SHA, firstLine(c.Commit.Message), c.Commit.Author.Name, c.Commit.Author.Email
				d := c.Commit.Committer.Date
				if d == "" {
					d = c.Commit.Author.Date
				}
				it.at = parseAPITime(d)
				if c.Author != nil {
					it.Login = c.Author.Login
				}
			}
			if it.SHA == "" {
				continue
			}
			if !it.at.IsZero() && it.at.Before(since) {
				older = true // a server that ignores since: the rest is older
				continue
			}
			if !it.at.IsZero() && !it.at.Before(end) {
				continue
			}
			it.URL = web.commit(it.SHA)
			out = append(out, it)
		}
		if older {
			return len(list), errStopWalk
		}
		return len(list), nil
	})
	return out, err
}

// gitlabPushes lists the push events of a GitLab project between since and end.
func gitlabPushes(w *apiWalk, project string, since, end time.Time, web gitWeb) ([]gitActivityItem, error) {
	// after / before are exclusive dates
	rel := w.p.gl(project) + "/events?action=pushed&after=" + since.AddDate(0, 0, -1).Format("2006-01-02") + "&before=" + end.Format("2006-01-02") + "&sort=desc"
	var out []gitActivityItem
	err := w.list(rel, gitActivityPages, func(raw json.RawMessage) (int, error) {
		var list []struct {
			CreatedAt string `json:"created_at"`
			Author    struct {
				Name     string `json:"name"`
				Username string `json:"username"`
			} `json:"author"`
			AuthorUsername string `json:"author_username"`
			PushData       *struct {
				CommitCount int    `json:"commit_count"`
				Action      string `json:"action"`
				RefType     string `json:"ref_type"`
				CommitFrom  string `json:"commit_from"`
				CommitTo    string `json:"commit_to"`
				Ref         string `json:"ref"`
				CommitTitle string `json:"commit_title"`
			} `json:"push_data"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, fmt.Errorf("unexpected answer from the Git server: %v", err)
		}
		for _, e := range list {
			if e.PushData == nil {
				continue
			}
			pd := e.PushData
			it := gitActivityItem{Kind: "push", Author: e.Author.Name, Login: e.Author.Username, Action: pd.Action, Count: pd.CommitCount,
				RefType: pd.RefType, Ref: pd.Ref, Before: pd.CommitFrom, After: pd.CommitTo, SHA: pd.CommitTo, Title: firstLine(pd.CommitTitle)}
			if it.Login == "" {
				it.Login = e.AuthorUsername
			}
			if it.Author == "" {
				it.Author = it.Login
			}
			if pd.RefType == "branch" {
				it.Branch = pd.Ref
			}
			it.at = parseAPITime(e.CreatedAt)
			if !it.at.IsZero() && (it.at.Before(since) || !it.at.Before(end)) {
				continue
			}
			it.URL = web.compare(it.Before, it.After)
			if it.URL == "" {
				it.URL = web.commit(it.After)
			}
			out = append(out, it)
		}
		return len(list), nil
	})
	return out, err
}

// githubPushes lists the repository activity of GitHub (pushes, force pushes, branch
// creation and deletion, merges) between since and end.
func githubPushes(w *apiWalk, project, branch string, since, end time.Time, web gitWeb) ([]gitActivityItem, error) {
	period := "year"
	switch days := time.Since(since).Hours() / 24; {
	case days <= 1:
		period = "day"
	case days <= 7:
		period = "week"
	case days <= 31:
		period = "month"
	case days <= 92:
		period = "quarter"
	}
	rel := w.p.gh(project) + "/activity?time_period=" + period + "&direction=desc"
	if branch != "" {
		rel += "&ref=" + url.QueryEscape("refs/heads/"+branch)
	}
	var out []gitActivityItem
	err := w.list(rel, gitActivityPages, func(raw json.RawMessage) (int, error) {
		var list []struct {
			Before       string `json:"before"`
			After        string `json:"after"`
			Ref          string `json:"ref"`
			Timestamp    string `json:"timestamp"`
			ActivityType string `json:"activity_type"`
			Actor        *struct {
				Login string `json:"login"`
			} `json:"actor"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return 0, fmt.Errorf("unexpected answer from the Git server: %v", err)
		}
		older := false
		for _, a := range list {
			it := gitActivityItem{Kind: "push", Action: a.ActivityType, Before: a.Before, After: a.After, SHA: a.After}
			if a.Actor != nil {
				it.Login, it.Author = a.Actor.Login, a.Actor.Login
			}
			switch {
			case strings.HasPrefix(a.Ref, "refs/heads/"):
				it.RefType, it.Ref, it.Branch = "branch", strings.TrimPrefix(a.Ref, "refs/heads/"), strings.TrimPrefix(a.Ref, "refs/heads/")
			case strings.HasPrefix(a.Ref, "refs/tags/"):
				it.RefType, it.Ref = "tag", strings.TrimPrefix(a.Ref, "refs/tags/")
			default:
				it.Ref = a.Ref
			}
			it.at = parseAPITime(a.Timestamp)
			if !it.at.IsZero() && it.at.Before(since) {
				older = true
				continue
			}
			if !it.at.IsZero() && !it.at.Before(end) {
				continue
			}
			it.URL = web.compare(it.Before, it.After)
			if it.URL == "" {
				it.URL = web.commit(it.After)
			}
			out = append(out, it)
		}
		if older {
			return len(list), errStopWalk
		}
		return len(list), nil
	})
	return out, err
}

// fetchActivity lists the activity of a service (cached).
func fetchActivity(userID int, aq gitActivityQuery) (*gitActivityRaw, bool, error) {
	cat, a, src, err := serviceSource(userID, aq.App)
	if err != nil {
		return nil, false, err
	}
	key := fmt.Sprintf("act|%d|%d|%s|%s|%s|%s", userID, src.ID, a.Name, aq.Branch, aq.Since.Format("2006-01-02"), aq.Until.Format("2006-01-02"))
	if !aq.Refresh {
		if v, _, ok := shortCacheGet(key, gitActivityTTL); ok {
			return v.(*gitActivityRaw), true, nil
		}
	}
	p, err := newGitProvider(src)
	if err != nil {
		return nil, false, err
	}
	res := &gitActivityRaw{App: a.Name, Project: a.Project, Provider: src.Kind, Since: aq.Since.Format("2006-01-02"), Until: aq.Until.Format("2006-01-02"),
		Branch: aq.Branch, Notes: []string{}, FetchedAt: nowStamp(), web: newGitWeb(src, a.Project)}
	w := newAPIWalk(p)
	end := aq.Until.AddDate(0, 0, 1)
	// branches (for the filter); a cached list is reused
	bkey := fmt.Sprintf("branches|%d|%s", src.ID, a.Project)
	if v, _, ok := shortCacheGet(bkey, gitActivityTTL); ok && !aq.Refresh {
		res.AllBranches = v.([]string)
	} else {
		all, err := p.branches(a.Project)
		if err != nil {
			if _, limited := isRateLimited(err); limited {
				return nil, false, fmt.Errorf("%v; try again later", err)
			}
			return nil, false, err
		}
		sort.Strings(all)
		shortCachePut(bkey, all)
		res.AllBranches = all
	}
	if len(res.AllBranches) > 1000 {
		res.AllBranches = res.AllBranches[:1000]
	}
	if aq.Branch != "" {
		res.Tracked = []string{aq.Branch}
	} else {
		t, _ := loadGitTarget(userID, a.Name)
		def := ""
		if a.Branch == "" && t.Branch == "" {
			def, _ = p.defaultBranch(a.Project)
		}
		res.Tracked = trackedBranches(cat, a, t, def, res.AllBranches)
	}
	var items []gitActivityItem
	fail := func(what string, err error) { res.Notes = append(res.Notes, what+": "+err.Error()) }
	// pushes
	switch p.kind {
	case "gitlab":
		list, err := gitlabPushes(w, a.Project, aq.Since, end, res.web)
		if err != nil {
			fail("push events", err)
		}
		for _, it := range list {
			if aq.Branch == "" || it.Branch == aq.Branch {
				items = append(items, it)
			}
		}
	case "github":
		list, err := githubPushes(w, a.Project, aq.Branch, aq.Since, end, res.web)
		if err != nil {
			if _, limited := isRateLimited(err); limited {
				fail("repository activity", err)
			} else {
				res.Notes = append(res.Notes, "repository activity is not available with this token: "+err.Error())
			}
		}
		items = append(items, list...)
	}
	// commits per branch (one item per commit; the branches it was seen on)
	bySHA := map[string]int{}
	for _, b := range res.Tracked {
		if w.limited {
			break
		}
		list, err := branchCommitsIn(w, a.Project, b, aq.Since, end, res.web)
		if err != nil {
			fail("commits of "+b, err)
		}
		for _, it := range list {
			if i, ok := bySHA[it.SHA]; ok {
				items[i].Branches = append(items[i].Branches, b)
				continue
			}
			it.Branches = []string{b}
			bySHA[it.SHA] = len(items)
			items = append(items, it)
		}
	}
	// a login seen on a commit names the person of a push
	names := map[string]string{}
	for _, it := range items {
		if it.Kind == "commit" && it.Login != "" && it.Author != "" {
			names[strings.ToLower(it.Login)] = it.Author
		}
	}
	for i := range items {
		if it := &items[i]; it.Kind == "push" && it.Author == it.Login {
			if n := names[strings.ToLower(it.Login)]; n != "" {
				it.Author = n
			}
		}
		items[i].At = items[i].at.Format(time.RFC3339)
		if items[i].at.IsZero() {
			items[i].At = ""
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })
	if len(items) > gitActivityMaxItems {
		items = items[:gitActivityMaxItems]
		w.truncated = true
	}
	res.Items = items
	res.Truncated, res.RateLimited, res.RateReset = w.truncated, w.limited, w.reset
	p.mu.Lock()
	res.Calls = p.calls
	p.mu.Unlock()
	shortCachePut(key, res)
	return res, false, nil
}

func matchAuthor(it gitActivityItem, q string) bool {
	if q == "" {
		return true
	}
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(it.Author), q) || strings.Contains(strings.ToLower(it.Login), q) || strings.Contains(strings.ToLower(it.email), q)
}

// summarize counts the commits and pushes per author and per branch with the last activity.
func summarize(items []gitActivityItem, web gitWeb) ([]gitActivitySummary, []gitActivitySummary) {
	type acc struct {
		s        gitActivitySummary
		last     time.Time
		branches map[string]bool
		authors  map[string]bool
	}
	authors, branches := map[string]*acc{}, map[string]*acc{}
	get := func(m map[string]*acc, k string) *acc {
		a := m[k]
		if a == nil {
			a = &acc{s: gitActivitySummary{Key: k}, branches: map[string]bool{}, authors: map[string]bool{}}
			m[k] = a
		}
		return a
	}
	touch := func(a *acc, it gitActivityItem) {
		if it.Kind == "commit" {
			a.s.Commits++
		} else {
			a.s.Pushes++
		}
		if it.at.After(a.last) {
			a.last = it.at
		}
	}
	for _, it := range items {
		who := it.Author
		if who == "" {
			who = it.Login
		}
		au := get(authors, who)
		touch(au, it)
		bl := it.Branches
		if it.Kind == "push" {
			bl = nil
			if it.Branch != "" {
				bl = []string{it.Branch}
			}
		}
		for _, b := range bl {
			au.branches[b] = true
			br := get(branches, b)
			touch(br, it)
			br.authors[strings.ToLower(who)] = true
		}
	}
	out := func(m map[string]*acc, branch bool) []gitActivitySummary {
		list := []gitActivitySummary{}
		for k, a := range m {
			s := a.s
			if !a.last.IsZero() {
				s.Last = a.last.Format(time.RFC3339)
			}
			if branch {
				s.Authors = len(a.authors)
				s.URL = web.branch(k)
			} else {
				for b := range a.branches {
					s.Branches = append(s.Branches, b)
				}
				sort.Strings(s.Branches)
			}
			list = append(list, s)
		}
		sort.Slice(list, func(i, j int) bool {
			ti, tj := list[i].Commits+list[i].Pushes, list[j].Commits+list[j].Pushes
			if ti != tj {
				return ti > tj
			}
			return strings.ToLower(list[i].Key) < strings.ToLower(list[j].Key)
		})
		return list
	}
	return out(authors, false), out(branches, true)
}

// gitActivity lists and filters the activity of a service.
func gitActivity(userID int, aq gitActivityQuery) (*gitActivityResult, error) {
	raw, cached, err := fetchActivity(userID, aq)
	if err != nil {
		return nil, err
	}
	res := &gitActivityResult{gitActivityRaw: *raw, Author: aq.Author, Cached: cached, Items: []gitActivityItem{}}
	for _, it := range raw.Items {
		if matchAuthor(it, aq.Author) {
			res.Items = append(res.Items, it)
		}
	}
	res.Authors, res.ByBranch = summarize(res.Items, raw.web)
	return res, nil
}

// ─── CSV ─────────────────────────────────────────────

// csvCell neutralises values a spreadsheet would run as a formula.
func csvCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// writeCSV sends a CSV download (UTF-8 with a byte order mark for spreadsheets).
func writeCSV(w http.ResponseWriter, name string, header []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte("\ufeff"))
	cw := csv.NewWriter(w)
	cw.Write(header)
	for _, r := range rows {
		for i := range r {
			r[i] = csvCell(r[i])
		}
		cw.Write(r)
	}
	cw.Flush()
}

// csvName is a download name: wrm-<what>[-<part>]-<date>.csv with safe characters only.
func csvName(what string, parts ...string) string {
	name := "wrm-" + what
	for _, p := range parts {
		if p = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
				return r
			}
			return '_'
		}, p); p != "" {
			name += "-" + p
		}
	}
	return name + "-" + time.Now().Format("20060102-1504") + ".csv"
}

func activityCSVRows(res *gitActivityResult) [][]string {
	rows := [][]string{}
	for _, it := range res.Items {
		count := ""
		if it.Count > 0 {
			count = strconv.Itoa(it.Count)
		}
		branch := it.Branch
		if it.Kind == "commit" && len(it.Branches) > 0 {
			branch = strings.Join(it.Branches, " ")
		}
		rows = append(rows, []string{it.At, it.Kind, res.App, branch, it.Author, it.Login, it.Action, it.RefType, it.Ref, count, it.SHA, it.Title, it.URL})
	}
	return rows
}

var activityCSVHeader = []string{"time", "kind", "service", "branch", "author", "login", "action", "ref_type", "ref", "commits", "sha", "title", "url"}

// ─── API ─────────────────────────────────────────────

// apiGitActivity handles the activity routes of /api/git; it returns false for other routes.
//
//	GET /api/git/activity?app=&branch=&author=&since=&until=&refresh=1   activity with summaries
//	GET /api/git/activity/export.csv?…                                    the same as CSV
func apiGitActivity(w http.ResponseWriter, r *http.Request, userID int, rest string) bool {
	if rest != "activity" && rest != "activity/export.csv" {
		return false
	}
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", 405)
		return true
	}
	aq, err := parseActivityQuery(r.URL.Query())
	if err != nil {
		jsonError(w, err.Error(), 400)
		return true
	}
	res, err := gitActivity(userID, aq)
	if err != nil {
		code := 502
		if strings.Contains(err.Error(), "not found") || strings.HasPrefix(err.Error(), "no Git server source") {
			code = 404
		}
		jsonError(w, err.Error(), code)
		return true
	}
	if rest == "activity/export.csv" {
		writeCSV(w, csvName("git-activity", res.App, aq.Branch), activityCSVHeader, activityCSVRows(res))
		return true
	}
	jsonOK(w, res)
	return true
}
