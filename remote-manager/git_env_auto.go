// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"
	"strings"
)

// ─── AUTOMATIC (DERIVED) ENVIRONMENTS ────────────────
//
// The checker already knows where every service is installed (git_installs: connection,
// path and an environment label taken from the path). Simple mode turns that into ready
// environment cards without anyone defining environments by hand: for every catalog service,
// the detected installations are grouped by their label into derived environments
// (destinations = the servers and paths found). Nothing is stored for them:
//
//   - a manual environment always wins (same name, or a destination it already lists, is
//     skipped), and Advanced mode lists only what is stored;
//   - a derived environment is resolved by the same functions as a manual one (envCatalog),
//     so plan, deploy, rollback, locks, backups, permissions and audit are identical, and a
//     deploy still needs a reviewed plan and a click;
//   - POST /api/git/env/adopt stores derived environments in the catalog (audited), after
//     which they are ordinary manual environments.

const gitAutoEnvDefault = "default"

// deriveEnvs returns the derived environments of the catalog's services (by service name).
func deriveEnvs(userID int, cat *gitCatalog) map[string][]gitEnvironment {
	installs := loadGitInstalls(userID, false, "i.state!='gone'")
	covered := map[string]bool{}
	manual := map[string]map[string]bool{}
	for _, a := range cat.Apps {
		manual[a.Name] = map[string]bool{}
		for _, e := range a.Environments {
			manual[a.Name][strings.ToLower(e.Name)] = true
			for _, d := range e.Destinations {
				covered[strings.ToLower(d.Server)+"\x00"+d.Path] = true
			}
		}
	}
	// only a connection whose name is unique among the user's SSH connections can be a destination
	usable := map[string]bool{}
	usableConn := func(name string) bool {
		ok, seen := usable[name]
		if !seen {
			_, err := envConn(userID, name)
			ok = err == nil
			usable[name] = ok
		}
		return ok
	}
	type key struct{ app, env string }
	groups := map[key][]gitEnvDest{}
	var order []key
	for _, in := range installs {
		if _, ok := cat.app(in.App); !ok || !usableConn(in.ConnName) {
			continue
		}
		p, err := validInstallDir(in.Path)
		if err != nil || covered[strings.ToLower(in.ConnName)+"\x00"+p] {
			continue
		}
		name := strings.TrimSpace(in.Env)
		if !gitNameRe.MatchString(name) {
			name = gitAutoEnvDefault
		}
		if manual[in.App][strings.ToLower(name)] {
			continue
		}
		k := key{in.App, strings.ToLower(name)}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], gitEnvDest{Server: in.ConnName, Path: p})
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].app != order[j].app {
			return order[i].app < order[j].app
		}
		return order[i].env < order[j].env
	})
	// accepted so far, to reject a derived environment whose paths clash with another one
	trial := make([]gitCatalogApp, len(cat.Apps))
	copy(trial, cat.Apps)
	out := map[string][]gitEnvironment{}
	for _, k := range order {
		dests := groups[k]
		if len(dests) > gitEnvMaxDests || len(out[k.app]) >= 20-len(manual[k.app]) {
			continue
		}
		env := gitEnvironment{Name: k.env, Destinations: dests, Auto: true}
		if env.normalize(k.app) != nil {
			continue
		}
		cand := append([]gitCatalogApp(nil), trial...)
		for i := range cand {
			if cand[i].Name == k.app {
				cand[i].Environments = append(append([]gitEnvironment(nil), cand[i].Environments...), env)
			}
		}
		if checkEnvPaths(cand) != nil {
			continue
		}
		trial = cand
		out[k.app] = append(out[k.app], env)
	}
	return out
}

// envCatalog is the catalog with the derived environments added to the services (never
// saved). A stored environment of that name wins and nothing is derived.
func envCatalog(userID int, app, env string) *gitCatalog {
	cat := loadGitCatalog(userID)
	if _, e, err := findEnv(cat, app, env); err == nil && e != nil {
		return cat
	}
	for name, envs := range deriveEnvs(userID, cat) {
		if a, ok := cat.app(name); ok && name == app {
			a.Environments = append(a.Environments, envs...)
		}
	}
	return cat
}

// adoptDerivedEnvs stores the derived environments (of one service when app is set) in the
// catalog as ordinary ones. It returns how many environments were stored, per service.
func adoptDerivedEnvs(userID int, app string) (map[string]int, error) {
	cat := loadGitCatalog(userID)
	added := map[string]int{}
	for name, envs := range deriveEnvs(userID, cat) {
		if app != "" && name != app {
			continue
		}
		a, _ := cat.app(name)
		for _, e := range envs {
			e.Auto = false
			a.Environments = append(a.Environments, e)
			added[name]++
		}
	}
	if len(added) == 0 {
		return added, nil
	}
	if err := cat.normalize(); err != nil {
		return nil, err
	}
	saveGitWorkspace(userID, cat, nil)
	return added, nil
}
