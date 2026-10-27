// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── fake provider for activity (GitLab events and commits, GitHub commits and activity,
// Gitea commits), each with its own paging: GitLab X-Next-Page, GitHub Link headers with
// absolute URLs (a cursor for the activity), Gitea Link headers with limit ─────────

type actCommit struct {
	sha, title, author, email, login string
	at                               time.Time
}

type actPush struct {
	at                            time.Time
	author, login, ref, refType   string
	before, after, title, actType string
	count                         int
}

type fakeActivity struct {
	kind     string
	srv      *httptest.Server
	token    string
	mu       sync.Mutex
	branches map[string][]actCommit // newest first
	pushes   []actPush              // newest first
	reqs     atomic.Int64
	// the X-RateLimit-Remaining sent, counting down (-1 = no header)
	remaining int
	// paths that answer 429 with Retry-After
	limited map[string]bool
	// Gitea ignores since (older servers): the client must stop by itself
	ignoreSince bool
}

var actBase = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// newFakeActivity serves demo/demo-api: main has 120 commits, one every 6 hours from
// 2026-09-01 (Ana writes two of three, Bo the third); release has two own commits by Bo on
// top of the five oldest commits of main. Pushes: Ana to main (twice), Bo to release, a tag.
func newFakeActivity(t *testing.T, kind string) *fakeActivity {
	f := &fakeActivity{kind: kind, token: "act-" + kind, branches: map[string][]actCommit{}, remaining: -1, limited: map[string]bool{}}
	var main []actCommit
	for i := 0; i < 120; i++ {
		c := actCommit{sha: commitSHA(fmt.Sprintf("m%d", i)), title: fmt.Sprintf("change %d", i), author: "Ana Dev", email: "ana@example.com", login: "ana",
			at: actBase.Add(time.Duration(i) * 6 * time.Hour)}
		if i%3 == 0 {
			c.author, c.email, c.login = "Bo Dev", "bo@example.com", "bo"
		}
		main = append([]actCommit{c}, main...)
	}
	f.branches["main"] = main
	rel := []actCommit{
		{sha: commitSHA("r2"), title: "fix, \"quoted\"\nsecond line", author: "Bo Dev", email: "bo@example.com", login: "bo", at: actBase.AddDate(0, 0, 25)},
		{sha: commitSHA("r1"), title: "=1+1", author: "Bo Dev", email: "bo@example.com", login: "bo", at: actBase.AddDate(0, 0, 24)},
	}
	f.branches["release"] = append(rel, main[len(main)-5:]...)
	f.pushes = []actPush{
		{at: actBase.AddDate(0, 0, 28), author: "Ana Dev", login: "ana", ref: "main", refType: "branch", before: main[1].sha, after: main[0].sha, title: main[0].title, count: 1, actType: "push"},
		{at: actBase.AddDate(0, 0, 25), author: "Bo Dev", login: "bo", ref: "release", refType: "branch", before: rel[1].sha, after: rel[0].sha, title: rel[0].title, count: 2, actType: "force_push"},
		{at: actBase.AddDate(0, 0, 20), author: "Ana Dev", login: "ana", ref: "v2.0", refType: "tag", before: strings.Repeat("0", 40), after: main[30].sha, count: 0, actType: "push"},
		{at: actBase.AddDate(0, 0, 10), author: "Ana Dev", login: "ana", ref: "main", refType: "branch", before: main[81].sha, after: main[80].sha, title: main[80].title, count: 3, actType: "push"},
		{at: actBase.AddDate(0, 0, -5), author: "Ana Dev", login: "ana", ref: "main", refType: "branch", before: main[119].sha, after: main[119].sha, count: 1, actType: "push"}, // before the range
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeActivity) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs.Add(1)
	auth := r.Header.Get("PRIVATE-TOKEN")
	switch f.kind {
	case "github":
		auth = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	case "gitea":
		auth = strings.TrimPrefix(r.Header.Get("Authorization"), "token ")
	}
	if auth != f.token {
		w.WriteHeader(401)
		return
	}
	p, q := r.URL.EscapedPath(), r.URL.Query()
	for prefix := range f.limited {
		if strings.Contains(p, prefix) {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(429)
			w.Write([]byte(`{"message":"Too Many Requests"}`))
			return
		}
	}
	if f.remaining >= 0 {
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(f.remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(actBase.AddDate(1, 0, 0).Unix(), 10))
		if f.remaining > 0 {
			f.remaining--
		}
	}
	var rest string
	switch f.kind {
	case "gitlab":
		rest = strings.TrimPrefix(p, "/api/v4/projects/demo%2Fdemo-api")
		rest = strings.TrimPrefix(rest, "/repository")
	default:
		rest = strings.TrimPrefix(strings.TrimPrefix(p, "/api/v3"), "/api/v1")
		rest = strings.TrimPrefix(rest, "/repos/demo/demo-api")
	}
	out := func(v interface{}) { json.NewEncoder(w).Encode(v) }
	size := 100
	if f.kind == "gitea" {
		size, _ = strconv.Atoi(q.Get("limit"))
	} else if n, _ := strconv.Atoi(q.Get("per_page")); n > 0 {
		size = n
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	// paginate n items: the slice bounds and the next-page headers
	paginate := func(n int) (int, int) {
		from, to := (page-1)*size, page*size
		if from > n {
			from = n
		}
		if to > n {
			to = n
		}
		if to < n {
			switch f.kind {
			case "gitlab":
				w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
			default:
				nq := r.URL.Query()
				nq.Set("page", strconv.Itoa(page+1))
				w.Header().Set("Link", fmt.Sprintf(`<%s%s?%s>; rel="next", <%s/x?page=99>; rel="last"`, f.srv.URL, p, nq.Encode(), f.srv.URL))
			}
		}
		return from, to
	}
	switch {
	case rest == "" || rest == "/":
		out(map[string]string{"default_branch": "main"})
	case rest == "/branches":
		out([]map[string]string{{"name": "main"}, {"name": "release"}, {"name": "old-feature"}})
	case rest == "/commits":
		b := q.Get("ref_name")
		if f.kind != "gitlab" {
			b = q.Get("sha")
		}
		since, _ := time.Parse(time.RFC3339, q.Get("since"))
		until, _ := time.Parse(time.RFC3339, q.Get("until"))
		var list []actCommit
		for _, c := range f.branches[b] {
			if (f.ignoreSince || since.IsZero() || !c.at.Before(since)) && (until.IsZero() || !c.at.After(until)) {
				list = append(list, c)
			}
		}
		from, to := paginate(len(list))
		var l []map[string]interface{}
		for _, c := range list[from:to] {
			if f.kind == "gitlab" {
				l = append(l, map[string]interface{}{"id": c.sha, "title": c.title, "author_name": c.author, "author_email": c.email, "committed_date": c.at.Format(time.RFC3339)})
			} else {
				l = append(l, map[string]interface{}{"sha": c.sha, "author": map[string]string{"login": c.login},
					"commit": map[string]interface{}{"message": c.title + "\n\nbody", "author": map[string]string{"name": c.author, "email": c.email, "date": c.at.Format(time.RFC3339)},
						"committer": map[string]string{"date": c.at.Format(time.RFC3339)}}})
			}
		}
		if l == nil {
			l = []map[string]interface{}{}
		}
		out(l)
	case rest == "/events" && f.kind == "gitlab":
		after, _ := time.Parse("2006-01-02", q.Get("after"))
		before, _ := time.Parse("2006-01-02", q.Get("before"))
		var list []actPush
		for _, e := range f.pushes {
			if e.at.After(after.AddDate(0, 0, 1)) && e.at.Before(before) {
				list = append(list, e)
			}
		}
		from, to := paginate(len(list))
		var l []map[string]interface{}
		for _, e := range list[from:to] {
			l = append(l, map[string]interface{}{"created_at": e.at.Format("2006-01-02T15:04:05.000Z"), "action_name": "pushed to", "author": map[string]string{"name": e.author, "username": e.login},
				"push_data": map[string]interface{}{"commit_count": e.count, "action": "pushed", "ref_type": e.refType, "ref": e.ref, "commit_from": e.before, "commit_to": e.after, "commit_title": e.title}})
		}
		out(l)
	case rest == "/activity" && f.kind == "github":
		// cursor paging: two items per page, the cursor is the index
		start, _ := strconv.Atoi(q.Get("after"))
		var list []actPush
		for _, e := range f.pushes {
			ref := "refs/heads/" + e.ref
			if e.refType == "tag" {
				ref = "refs/tags/" + e.ref
			}
			if q.Get("ref") == "" || q.Get("ref") == ref {
				list = append(list, e)
			}
		}
		end := start + 2
		if end < len(list) {
			nq := r.URL.Query()
			nq.Set("after", strconv.Itoa(end))
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?%s>; rel="next"`, f.srv.URL, p, nq.Encode()))
		} else {
			end = len(list)
		}
		var l []map[string]interface{}
		for _, e := range list[start:end] {
			ref := "refs/heads/" + e.ref
			if e.refType == "tag" {
				ref = "refs/tags/" + e.ref
			}
			l = append(l, map[string]interface{}{"before": e.before, "after": e.after, "ref": ref, "timestamp": e.at.Format(time.RFC3339), "activity_type": e.actType, "actor": map[string]string{"login": e.login}})
		}
		out(l)
	default:
		w.WriteHeader(404)
	}
}

func setupActivityUser(t *testing.T, srv *httptest.Server, name string, f *fakeActivity) *testClient {
	u := newTestUser(t, srv, name, false)
	cat := demoCatalog(f.srv.URL)
	cat["apps"].([]map[string]interface{})[0]["environments"] = []map[string]interface{}{{"name": "test", "destinations": []map[string]string{{"server": "app-01", "path": "/opt/demo-api"}},
		"allowed_refs": []string{"refs/heads/release", "refs/tags/v*"}}}
	if code := u.jsonDo("PUT", "/api/git/catalog", cat, nil); code != 200 {
		t.Fatalf("catalog: %d", code)
	}
	if code := u.jsonDo("POST", "/api/git/sources", map[string]interface{}{"kind": f.kind, "name": "demo " + f.kind, "url": f.srv.URL, "token": f.token}, nil); code != 200 {
		t.Fatalf("source: %d", code)
	}
	return u
}

func activityURL(v url.Values) string {
	v.Set("app", "demo-api")
	return "/api/git/activity?" + v.Encode()
}

func summaryOf(list []gitActivitySummary, key string) gitActivitySummary {
	for _, s := range list {
		if s.Key == key {
			return s
		}
	}
	return gitActivitySummary{}
}

func TestGitActivity(t *testing.T) {
	srv := newTestServer(t)
	for _, kind := range []string{"gitlab", "github", "gitea"} {
		t.Run(kind, func(t *testing.T) {
			f := newFakeActivity(t, kind)
			u := setupActivityUser(t, srv, "act-"+kind, f)
			rng := url.Values{"since": {"2026-09-01"}, "until": {"2026-09-30"}}
			var res gitActivityResult
			if code := u.jsonDo("GET", activityURL(rng), nil, &res); code != 200 {
				t.Fatalf("activity: %d", code)
			}
			// the service's branch and the environment's allowed branch are listed
			if strings.Join(res.Tracked, ",") != "main,release" || len(res.AllBranches) != 3 || res.Provider != kind {
				t.Fatalf("branches: %+v %+v", res.Tracked, res.AllBranches)
			}
			commits, pushes := 0, 0
			var shared *gitActivityItem
			for i, it := range res.Items {
				if it.Kind == "commit" {
					commits++
					if it.SHA == f.branches["main"][119].sha {
						shared = &res.Items[i]
					}
				} else {
					pushes++
				}
				if i > 0 && it.At > res.Items[i-1].At {
					t.Fatalf("not newest first at %d", i)
				}
			}
			// 120 commits of main over several pages (100 / 50 per page) + 2 own commits of release
			if commits != 122 || res.Truncated {
				t.Fatalf("commits: %d truncated %v notes %v", commits, res.Truncated, res.Notes)
			}
			// a commit on both branches is listed once
			if shared == nil || strings.Join(shared.Branches, ",") != "main,release" {
				t.Fatalf("shared commit: %+v", shared)
			}
			wantPushes := map[string]int{"gitlab": 4, "github": 4, "gitea": 0}[kind]
			if pushes != wantPushes {
				t.Fatalf("pushes: %d, want %d (notes %v)", pushes, wantPushes, res.Notes)
			}
			// summaries: Bo wrote every third commit of main and both of release
			ana, bo := summaryOf(res.Authors, "Ana Dev"), summaryOf(res.Authors, "Bo Dev")
			if ana.Commits != 80 || bo.Commits != 42 || ana.Pushes != wantPushes*3/4 || bo.Pushes != wantPushes/4 {
				t.Fatalf("authors: %+v", res.Authors)
			}
			if strings.Join(bo.Branches, ",") != "main,release" || ana.Last == "" {
				t.Fatalf("author branches: %+v", bo)
			}
			main, rel := summaryOf(res.ByBranch, "main"), summaryOf(res.ByBranch, "release")
			if main.Commits != 120 || rel.Commits != 7 || main.Authors != 2 || rel.Authors != 2 || main.Last != f.branches["main"][0].at.Format(time.RFC3339) {
				t.Fatalf("branches: %+v", res.ByBranch)
			}
			if main.URL == "" || !strings.HasPrefix(main.URL, f.srv.URL+"/demo/demo-api/") {
				t.Fatalf("branch link: %q", main.URL)
			}
			// links to the commit and the compare page
			c := res.Items[0]
			for _, it := range res.Items {
				if it.Kind == "commit" {
					c = it
					break
				}
			}
			wantLink := f.srv.URL + "/demo/demo-api/commit/" + c.SHA
			if kind == "gitlab" {
				wantLink = f.srv.URL + "/demo/demo-api/-/commit/" + c.SHA
			}
			if c.URL != wantLink {
				t.Fatalf("commit link %q, want %q", c.URL, wantLink)
			}
			for _, it := range res.Items {
				if it.Kind == "push" && it.RefType == "branch" && !strings.Contains(it.URL, "compare/"+it.Before+"..."+it.After) {
					t.Fatalf("push link: %+v", it)
				}
				if it.Kind == "push" && it.RefType == "tag" && !strings.HasSuffix(it.URL, "/commit/"+it.After) {
					t.Fatalf("tag push link: %+v", it)
				}
			}
			// the cache: the same query does not call the provider again; refresh does
			n := f.reqs.Load()
			u.jsonDo("GET", activityURL(rng), nil, &res)
			if f.reqs.Load() != n || !res.Cached {
				t.Fatal("the cached listing was not used")
			}
			rf := url.Values{"since": {"2026-09-01"}, "until": {"2026-09-30"}, "refresh": {"1"}}
			u.jsonDo("GET", activityURL(rf), nil, &res)
			if f.reqs.Load() == n || res.Cached {
				t.Fatal("refresh did not call the provider")
			}
			// filters: author (from the cached listing), branch, dates
			var fl gitActivityResult
			u.jsonDo("GET", activityURL(url.Values{"since": {"2026-09-01"}, "until": {"2026-09-30"}, "author": {"BO"}}), nil, &fl)
			for _, it := range fl.Items {
				if it.Author != "Bo Dev" {
					t.Fatalf("author filter: %+v", it)
				}
			}
			if len(fl.Authors) != 1 || fl.Authors[0].Commits != 42 {
				t.Fatalf("author filter summary: %+v", fl.Authors)
			}
			u.jsonDo("GET", activityURL(url.Values{"since": {"2026-09-01"}, "until": {"2026-09-30"}, "branch": {"release"}}), nil, &fl)
			for _, it := range fl.Items {
				if it.Branch != "release" {
					t.Fatalf("branch filter: %+v", it)
				}
			}
			if strings.Join(fl.Tracked, ",") != "release" || len(fl.ByBranch) != 1 || fl.ByBranch[0].Commits != 7 {
				t.Fatalf("branch filter summary: %+v %+v", fl.Tracked, fl.ByBranch)
			}
			if kind == "gitea" {
				f.ignoreSince = true // the client stops at the first older commit
			}
			u.jsonDo("GET", activityURL(url.Values{"since": {"2026-09-21"}, "until": {"2026-09-30"}}), nil, &fl)
			nc := 0
			for _, it := range fl.Items {
				if it.At < "2026-09-21" || it.At > "2026-10-01" {
					t.Fatalf("date filter: %+v", it)
				}
				if it.Kind == "commit" {
					nc++
				}
			}
			// main: 2026-09-21 00:00 … 2026-09-30 18:00, every 6 hours = 40; release: r2 and r1 (25th, 26th)
			if nc != 42 {
				t.Fatalf("date filter commits: %d", nc)
			}
			f.ignoreSince = false
			// invalid filters
			for _, bad := range []url.Values{{"since": {"2026-13-01"}}, {"since": {"2026-09-30"}, "until": {"2026-09-01"}}, {"since": {"2024-01-01"}, "until": {"2026-09-01"}}} {
				if code := u.jsonDo("GET", activityURL(bad), nil, nil); code != 400 {
					t.Fatalf("bad filter %v: %d", bad, code)
				}
			}
			if code := u.jsonDo("GET", "/api/git/activity?app=nope", nil, nil); code != 404 {
				t.Fatalf("unknown service: %d", code)
			}
			// CSV: the filtered items with escaping and neutralised formulas
			csvQ := url.Values{"app": {"demo-api"}, "since": {"2026-09-01"}, "until": {"2026-09-30"}, "branch": {"release"}}
			u.jsonDo("GET", "/api/git/activity?"+csvQ.Encode(), nil, &fl)
			resp := u.do("GET", "/api/git/activity/export.csv?"+csvQ.Encode(), nil, "")
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") || !strings.Contains(resp.Header.Get("Content-Disposition"), "wrm-git-activity-demo-api-release-") {
				t.Fatalf("csv: %d %v", resp.StatusCode, resp.Header)
			}
			if !strings.HasPrefix(string(body), "\ufeff") {
				t.Fatal("csv: no byte order mark")
			}
			recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll()
			if err == nil && len(recs) != len(fl.Items)+1 {
				t.Fatalf("csv rows: %d, items %d", len(recs), len(fl.Items))
			}
			if err != nil || strings.Join(recs[0], ",") != strings.Join(activityCSVHeader, ",") {
				t.Fatalf("csv header: %v %v", err, recs)
			}
			titles := map[string]bool{}
			for _, r := range recs[1:] {
				titles[r[11]] = true
				if r[2] != "demo-api" {
					t.Fatalf("csv service: %v", r)
				}
			}
			if !titles["fix, \"quoted\""] || !titles["'=1+1"] {
				t.Fatalf("csv escaping: %v", titles)
			}
		})
	}
}

func TestGitActivityRateLimits(t *testing.T) {
	srv := newTestServer(t)
	// GitHub: few calls left → paging stops, the listing is marked
	f := newFakeActivity(t, "github")
	u := setupActivityUser(t, srv, "act-rate", f)
	f.remaining = gitActivityRateFloor + 3
	var res gitActivityResult
	if code := u.jsonDo("GET", activityURL(url.Values{"since": {"2026-09-01"}, "until": {"2026-09-30"}}), nil, &res); code != 200 {
		t.Fatalf("activity: %d", code)
	}
	if !res.RateLimited || !res.Truncated || res.RateReset == "" {
		t.Fatalf("rate limit not reported: %+v", res.gitActivityRaw)
	}
	commits := 0
	for _, it := range res.Items {
		if it.Kind == "commit" {
			commits++
		}
	}
	if commits >= 122 {
		t.Fatalf("paging did not stop: %d commits", commits)
	}
	// GitLab: 429 on the events → a note with the time, the listing is still answered
	g := newFakeActivity(t, "gitlab")
	u2 := setupActivityUser(t, srv, "act-429", g)
	g.limited["/events"] = true
	if code := u2.jsonDo("GET", activityURL(url.Values{"since": {"2026-09-01"}, "until": {"2026-09-30"}}), nil, &res); code != 200 {
		t.Fatalf("activity 429: %d", code)
	}
	if !res.RateLimited || len(res.Notes) == 0 || !strings.Contains(res.Notes[0], "429") || res.RateReset == "" {
		t.Fatalf("429: %+v", res.gitActivityRaw)
	}
	// the branch list itself limited → an error that says so
	g.limited["/branches"] = true
	if code := u2.jsonDo("GET", activityURL(url.Values{"since": {"2026-09-02"}, "until": {"2026-09-30"}, "refresh": {"1"}}), nil, nil); code != 502 {
		t.Fatalf("limited branches: %d", code)
	}
}

func TestGitActivityHelpers(t *testing.T) {
	if linkNext(`<https://example.com/api/v4/x?page=2>; rel="next", <https://example.com/api/v4/x?page=9>; rel="last"`) != "https://example.com/api/v4/x?page=2" ||
		linkNext(`<https://example.com/x?page=1>; rel="prev"`) != "" {
		t.Fatal("linkNext")
	}
	for in, want := range map[string]string{"=SUM(A1)": "'=SUM(A1)", "+1": "'+1", "-x": "'-x", "@a": "'@a", "plain": "plain", "": ""} {
		if csvCell(in) != want {
			t.Errorf("csvCell(%q) = %q", in, csvCell(in))
		}
	}
	gl := newGitWeb(gitSource{Kind: "gitlab", URL: "https://git.example.com/"}, "group/sub/demo-api")
	if gl.commit("abc") != "https://git.example.com/group/sub/demo-api/-/commit/abc" || gl.compare("a", "b") != "https://git.example.com/group/sub/demo-api/-/compare/a...b" ||
		gl.branch("feature/x") != "https://git.example.com/group/sub/demo-api/-/commits/feature/x" || gl.compare(strings.Repeat("0", 40), "b") != "" {
		t.Fatal("gitlab links")
	}
	gh := newGitWeb(gitSource{Kind: "github", URL: "https://api.github.com"}, "demo/demo-api")
	if gh.commit("abc") != "https://github.com/demo/demo-api/commit/abc" {
		t.Fatalf("github link: %s", gh.commit("abc"))
	}
	ge := newGitWeb(gitSource{Kind: "gitea", URL: "https://code.example.com/api/v1"}, "demo/demo-api")
	if ge.branch("main") != "https://code.example.com/demo/demo-api/commits/branch/main" || ge.compare("a", "b") != "https://code.example.com/demo/demo-api/compare/a...b" {
		t.Fatal("gitea links")
	}
	if (newGitWeb(gitSource{Kind: "gitlab", URL: "javascript:alert(1)"}, "x")).commit("a") != "" {
		t.Fatal("a non-http URL gave a link")
	}
	if b, _ := gitAPIBase("gitea", "https://code.example.com"); b != "https://code.example.com/api/v1" {
		t.Fatalf("gitea API base: %s", b)
	}
	if csvName("git-activity", "demo api", "../x") != "wrm-git-activity-demo_api-.._x-"+time.Now().Format("20060102-1504")+".csv" {
		t.Fatalf("csvName: %s", csvName("git-activity", "demo api", "../x"))
	}
}
