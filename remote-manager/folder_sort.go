// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// ─── FOLDER SORTING (#134) ───────────────────────────
//
// Folders keep the order the user gives them (drag and drop, up / down). A user with many
// folders can sort them by name once, at one level or at every level below a folder, and can
// keep them sorted: a folder marked "keep sorted" (folders.auto_sort) sorts its subfolders at
// every level below it whenever a folder is added, renamed or moved; "keep all folders sorted"
// (user_ui key folders_sort = "auto") does the same for the whole tree. Connections inside a
// folder are always listed by name. Names compare without case and with numbers in their
// natural order (srv2 before srv10).

// POST /api/folders/sort {"folder_id": 0 | id, "recursive": true, "keep": true | false}
//
//	folder_id 0 is the top level; keep (optional) switches automatic sorting for that folder
//	(for 0: every folder of the user).
//
// GET  /api/folders/sort → {"keep_all": bool}
func apiFolderSortHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		jsonOK(w, map[string]interface{}{"keep_all": folderSortAll(uid)})
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var in struct {
		FolderID  int   `json:"folder_id"`
		Recursive bool  `json:"recursive"`
		Keep      *bool `json:"keep"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.FolderID < 0 {
		jsonError(w, "Invalid request", 400)
		return
	}
	folderHierarchyMu.Lock()
	defer folderHierarchyMu.Unlock()
	name := ""
	if in.FolderID > 0 {
		if !userOwnsFolder(in.FolderID, uid) {
			jsonError(w, "Not found", 404)
			return
		}
		db.QueryRow(`SELECT name FROM folders WHERE id=? AND user_id=?`, in.FolderID, uid).Scan(&name)
	}
	if in.Keep != nil {
		if in.FolderID == 0 {
			if *in.Keep {
				db.Exec(`INSERT INTO user_ui (user_id, key, value, updated_at) VALUES (?, 'folders_sort', 'auto', ?)
					ON CONFLICT(user_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, uid, nowStamp())
			} else {
				db.Exec(`DELETE FROM user_ui WHERE user_id=? AND key='folders_sort'`, uid)
			}
		} else {
			db.Exec(`UPDATE folders SET auto_sort=? WHERE id=? AND user_id=?`, map[bool]int{true: 1, false: 0}[*in.Keep], in.FolderID, uid)
		}
	}
	var parent *int
	if in.FolderID > 0 {
		parent = &in.FolderID
	}
	n := sortFolderLevel(uid, parent, in.Recursive || in.Keep != nil && *in.Keep)
	autoSortFolders(uid)
	d := map[string]interface{}{"folder_id": in.FolderID, "recursive": in.Recursive, "moved": n}
	if in.Keep != nil {
		d["keep_sorted"] = *in.Keep
	}
	auditLog(r, uid, "folder.sorted", name, d)
	jsonOK(w, map[string]interface{}{"folders": loadFolders(uid), "keep_all": folderSortAll(uid), "moved": n})
}

func folderSortAll(uid int) bool {
	var v string
	db.QueryRow(`SELECT value FROM user_ui WHERE user_id=? AND key='folders_sort'`, uid).Scan(&v)
	return v == "auto"
}

// sortFolderLevel sorts the subfolders of parent (nil = top level) by name, and every level
// below them when recursive is set. It returns how many folders changed their place.
func sortFolderLevel(uid int, parent *int, recursive bool) int {
	list := loadFolders(uid)
	want := func(p *int) bool {
		if intPtrEq(p, parent) {
			return true
		}
		if !recursive {
			return false
		}
		if parent == nil {
			return true
		}
		return p != nil && folderUnder(list, *p, *parent)
	}
	return sortFolderGroups(uid, list, want)
}

// autoSortFolders keeps the levels sorted that the user asked to keep sorted.
func autoSortFolders(uid int) {
	list := loadFolders(uid)
	all := folderSortAll(uid)
	auto := map[int]bool{}
	for _, f := range list {
		if f.AutoSort {
			auto[f.ID] = true
		}
	}
	if !all && len(auto) == 0 {
		return
	}
	sortFolderGroups(uid, list, func(p *int) bool {
		if all {
			return true
		}
		if p == nil {
			return false
		}
		for id := range auto {
			if *p == id || folderUnder(list, *p, id) {
				return true
			}
		}
		return false
	})
}

// folderUnder tells whether folder id lies below ancestor.
func folderUnder(list []Folder, id, ancestor int) bool {
	byID := map[int]Folder{}
	for _, f := range list {
		byID[f.ID] = f
	}
	for n := 0; n < folderMaxDepth; n++ {
		f, ok := byID[id]
		if !ok || f.ParentID == nil {
			return false
		}
		if *f.ParentID == ancestor {
			return true
		}
		id = *f.ParentID
	}
	return false
}

// sortFolderGroups sorts every group of sibling folders whose parent want accepts and writes
// the new sort_order where it changed.
func sortFolderGroups(uid int, list []Folder, want func(parent *int) bool) int {
	groups := map[int][]Folder{} // parent id (0 = top level) → children in their current order
	parents := map[int]*int{}
	for _, f := range list {
		k := 0
		if f.ParentID != nil {
			k = *f.ParentID
		}
		groups[k] = append(groups[k], f)
		parents[k] = f.ParentID
	}
	moved := 0
	for k, kids := range groups {
		if !want(parents[k]) {
			continue
		}
		sorted := append([]Folder(nil), kids...)
		sort.SliceStable(sorted, func(i, j int) bool {
			if c := naturalCompare(sorted[i].Name, sorted[j].Name); c != 0 {
				return c < 0
			}
			return sorted[i].ID < sorted[j].ID
		})
		for i, f := range sorted {
			if f.SortOrder != i+1 {
				db.Exec(`UPDATE folders SET sort_order=? WHERE id=? AND user_id=?`, i+1, f.ID, uid)
			}
			if kids[i].ID != f.ID {
				moved++
			}
		}
	}
	return moved
}

// naturalCompare compares names without case, with runs of digits compared as numbers.
func naturalCompare(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			continue
		}
		if ra[i] != rb[j] {
			if ra[i] < rb[j] {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case len(ra)-i < len(rb)-j:
		return -1
	case len(ra)-i > len(rb)-j:
		return 1
	}
	return 0
}
