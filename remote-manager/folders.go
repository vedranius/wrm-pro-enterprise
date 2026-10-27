// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

const folderMaxDepth = 32

// Serialize hierarchy changes so concurrent moves cannot introduce a cycle.
var folderHierarchyMu sync.Mutex

func folderPath(list []Folder, id int) string {
	byID := map[int]Folder{}
	for _, f := range list {
		byID[f.ID] = f
	}
	parts := []string{}
	seen := map[int]bool{}
	for id > 0 && !seen[id] && len(parts) < folderMaxDepth {
		f, ok := byID[id]
		if !ok {
			break
		}
		seen[id] = true
		parts = append([]string{f.Name}, parts...)
		id = 0
		if f.ParentID != nil {
			id = *f.ParentID
		}
	}
	return strings.Join(parts, " / ")
}

func folderPathOf(userID int, id *int) string {
	if id == nil {
		return ""
	}
	return folderPath(loadFolders(userID), *id)
}

// folderSubtreeIDs returns the folder and every folder below it.
func folderSubtreeIDs(folders []Folder, rootID int) []int {
	children := map[int][]int{}
	found := false
	for _, f := range folders {
		if f.ID == rootID {
			found = true
		}
		if f.ParentID != nil {
			children[*f.ParentID] = append(children[*f.ParentID], f.ID)
		}
	}
	if !found {
		return nil
	}
	out := make([]int, 0)
	seen := map[int]bool{}
	var walk func(int)
	walk = func(id int) {
		if seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
		for _, child := range children[id] {
			walk(child)
		}
	}
	walk(rootID)
	return out
}

func idChunks(ids []int, size int) [][]int {
	if size <= 0 {
		size = 400
	}
	out := make([][]int, 0, (len(ids)+size-1)/size)
	for len(ids) > 0 {
		n := size
		if n > len(ids) {
			n = len(ids)
		}
		out = append(out, ids[:n])
		ids = ids[n:]
	}
	return out
}

func intPlaceholders(n int) string {
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func intArgs(ids []int) []interface{} {
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// deleteFolderTree deletes a folder, all its subfolders and every connection in that
// subtree in one transaction. It returns the IDs that were removed so the caller can
// stop terminals and tunnels for them.
func deleteFolderTree(userID, rootID int) (folderIDs, connIDs []int, err error) {
	all := loadFolders(userID)
	folderIDs = folderSubtreeIDs(all, rootID)
	if len(folderIDs) == 0 {
		return nil, nil, fmt.Errorf("folder not found")
	}
	for _, chunk := range idChunks(folderIDs, 400) {
		args := append([]interface{}{userID}, intArgs(chunk)...)
		rows, qerr := db.Query("SELECT id FROM connections WHERE user_id=? AND folder_id IN ("+intPlaceholders(len(chunk))+")", args...)
		if qerr != nil {
			return nil, nil, qerr
		}
		for rows.Next() {
			var id int
			if qerr = rows.Scan(&id); qerr == nil {
				connIDs = append(connIDs, id)
			}
		}
		rows.Close()
		if qerr != nil {
			return nil, nil, qerr
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	// The query templates contain %s, replaced by one placeholder per ID chunk.
	exec := func(queryFmt string, ids []int, prefix ...interface{}) error {
		if len(ids) == 0 {
			return nil
		}
		for _, chunk := range idChunks(ids, 400) {
			args := append(append([]interface{}{}, prefix...), intArgs(chunk)...)
			if _, e := tx.Exec(fmt.Sprintf(queryFmt, intPlaceholders(len(chunk))), args...); e != nil {
				return e
			}
		}
		return nil
	}
	// Connection-scoped data first, while the connections still exist.
	for _, set := range []struct {
		query  string
		prefix []interface{}
	}{
		{"DELETE FROM connection_tunnels WHERE user_id=? AND conn_id IN (%s)", []interface{}{userID}},
		{"DELETE FROM ssh_key_deployments WHERE user_id=? AND conn_id IN (%s)", []interface{}{userID}},
		{"DELETE FROM snippets WHERE user_id=? AND scope='connection' AND scope_id IN (%s)", []interface{}{userID}},
		{"DELETE FROM bookmarks WHERE user_id=? AND scope='connection' AND scope_id IN (%s)", []interface{}{userID}},
		{"DELETE FROM share_items WHERE connection_id IN (%s)", nil},
	} {
		if e := exec(set.query, connIDs, set.prefix...); e != nil {
			return nil, nil, e
		}
	}
	if e := exec("UPDATE connections SET jump_conn_id=NULL WHERE jump_conn_id IN (%s)", connIDs); e != nil {
		return nil, nil, e
	}
	if e := exec("UPDATE folders SET jump_conn_id=NULL WHERE jump_conn_id IN (%s)", connIDs); e != nil {
		return nil, nil, e
	}
	if e := exec("DELETE FROM connections WHERE user_id=? AND id IN (%s)", connIDs, userID); e != nil {
		return nil, nil, e
	}

	// Folder-scoped data (snippets, bookmarks, shares), then the folders themselves.
	if e := exec("DELETE FROM snippets WHERE user_id=? AND scope='folder' AND scope_id IN (%s)", folderIDs, userID); e != nil {
		return nil, nil, e
	}
	if e := exec("DELETE FROM bookmarks WHERE user_id=? AND scope='folder' AND scope_id IN (%s)", folderIDs, userID); e != nil {
		return nil, nil, e
	}
	if e := exec("DELETE FROM share_items WHERE folder_id IN (%s)", folderIDs); e != nil {
		return nil, nil, e
	}
	if e := exec("DELETE FROM folders WHERE user_id=? AND id IN (%s)", folderIDs, userID); e != nil {
		return nil, nil, e
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return folderIDs, connIDs, nil
}

func validateFolderParent(userID int, f *Folder) error {
	if f.ParentID != nil && *f.ParentID <= 0 {
		f.ParentID = nil
	}
	list := loadFolders(userID)
	byID := map[int]Folder{}
	for _, x := range list {
		byID[x.ID] = x
	}
	if f.ParentID != nil {
		if _, ok := byID[*f.ParentID]; !ok {
			return fmt.Errorf("parent folder not found")
		}
	}
	// Apply the proposed parent before checking every descendant's depth.
	byID[f.ID] = *f
	for start := range byID {
		seen := map[int]bool{}
		id := start
		for depth := 0; ; depth++ {
			if depth >= folderMaxDepth {
				return fmt.Errorf("at most %d nested folders", folderMaxDepth)
			}
			if seen[id] {
				return fmt.Errorf("a folder cannot contain itself or one of its ancestors")
			}
			seen[id] = true
			x, ok := byID[id]
			if !ok || x.ParentID == nil {
				break
			}
			id = *x.ParentID
		}
	}
	return nil
}

func folderImportKey(f Folder) string {
	if f.ID > 0 {
		return fmt.Sprintf("id:%d", f.ID)
	}
	return "name:" + strings.TrimSpace(f.Name)
}

// Order parents before children, regardless of the export's alphabetical order.
func folderImportOrder(list []Folder) ([]int, error) {
	byID := map[int]int{}
	for i, f := range list {
		if f.ID > 0 {
			if _, dup := byID[f.ID]; dup {
				return nil, fmt.Errorf("duplicate folder id")
			}
			byID[f.ID] = i
		}
	}
	state := map[int]int{}
	order := []int{}
	var visit func(int, int) error
	visit = func(i, depth int) error {
		if depth >= folderMaxDepth {
			return fmt.Errorf("at most %d nested folders", folderMaxDepth)
		}
		if state[i] == 1 {
			return fmt.Errorf("folder hierarchy contains a cycle")
		}
		if state[i] == 2 {
			return nil
		}
		state[i] = 1
		if p := list[i].ParentID; p != nil && *p > 0 {
			j, ok := byID[*p]
			if !ok || strings.TrimSpace(list[j].Name) == "" {
				return fmt.Errorf("parent folder not found in import")
			}
			if err := visit(j, depth+1); err != nil {
				return err
			}
		}
		state[i] = 2
		order = append(order, i)
		return nil
	}
	for i := range list {
		if err := visit(i, 0); err != nil {
			return nil, err
		}
	}
	// Cached visits alone cannot measure a child of a previously visited deep parent.
	for i := range list {
		depth := 1
		for p := list[i].ParentID; p != nil && *p > 0; {
			depth++
			if depth > folderMaxDepth {
				return nil, fmt.Errorf("at most %d nested folders", folderMaxDepth)
			}
			p = list[byID[*p]].ParentID
		}
	}
	return order, nil
}

// Reorder siblings or move a folder under a new parent in one transaction.
func apiFolderReorderHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var in struct {
		ID       int  `json:"id"`
		ParentID *int `json:"parent_id"`
		BeforeID int  `json:"before_id"`
		AfterID  int  `json:"after_id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.ID <= 0 || (in.BeforeID != 0 && in.AfterID != 0) {
		jsonError(w, "Invalid folder order", 400)
		return
	}
	folderHierarchyMu.Lock()
	defer folderHierarchyMu.Unlock()
	list := loadFolders(uid)
	var moved Folder
	found := false
	for _, f := range list {
		if f.ID == in.ID {
			moved = f
			found = true
			break
		}
	}
	if !found {
		jsonError(w, "Not found", 404)
		return
	}
	moved.ParentID = in.ParentID
	if e := validateFolderParent(uid, &moved); e != nil {
		jsonError(w, e.Error(), 400)
		return
	}
	siblings := []int{}
	for _, f := range list {
		if f.ID != in.ID && intPtrEq(f.ParentID, moved.ParentID) {
			siblings = append(siblings, f.ID)
		}
	}
	index := len(siblings)
	anchor := in.BeforeID
	if anchor == 0 {
		anchor = in.AfterID
	}
	if anchor != 0 {
		index = -1
		for i, id := range siblings {
			if id == anchor {
				index = i
				if in.AfterID != 0 {
					index++
				}
				break
			}
		}
		if index < 0 {
			jsonError(w, "Order anchor is not a sibling owned by you", 400)
			return
		}
	}
	siblings = append(siblings, 0)
	copy(siblings[index+1:], siblings[index:])
	siblings[index] = in.ID
	tx, e := db.Begin()
	if e != nil {
		jsonError(w, "Cannot change folder order", 500)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE folders SET parent_id=? WHERE id=? AND user_id=?", moved.ParentID, in.ID, uid); e == nil {
		for i, id := range siblings {
			if _, e = tx.Exec("UPDATE folders SET sort_order=? WHERE id=? AND user_id=?", i+1, id, uid); e != nil {
				break
			}
		}
	}
	if e != nil {
		jsonError(w, "Cannot change folder order", 500)
		return
	}
	if e = tx.Commit(); e != nil {
		jsonError(w, "Cannot change folder order", 500)
		return
	}
	autoSortFolders(uid) // a folder dropped into a sorted folder takes its place by name
	revalidateFolderShares(uid)
	auditLog(r, uid, "folder.reordered", moved.Name, map[string]interface{}{"folder_id": in.ID, "parent_id": moved.ParentID})
	jsonOK(w, loadFolders(uid))
}
