// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestStorageMCPTools covers STG-09: an AI connection reaches a storage only through its own
// scopes; a read-only grant cannot write, and a delete needs the delete scope, the storage's
// permission and the explicit confirmation.
func TestStorageMCPTools(t *testing.T) {
	u, stub, _ := storageTestEnv(t, "storage-mcp", func(b map[string]interface{}) {
		b["allow_write"], b["allow_delete"] = true, true
	})
	stub.put("team-a/config.json", []byte(`{"a":1}`))

	readOnly := &mcpGrant{ID: 1, UserID: u.userID, Mode: aiModeAsk, Scopes: []string{"storage_read"}}
	names := func(g *mcpGrant) []string {
		var out []string
		for _, tool := range mcpStorageTools(g) {
			out = append(out, tool["name"].(string))
		}
		return out
	}
	if got := strings.Join(names(readOnly), ","); got != "storage_list,storage_read" {
		t.Fatalf("read-only tools: %s", got)
	}
	if got := strings.Join(names(&mcpGrant{ID: 2, UserID: u.userID, Mode: aiModeAsk,
		Scopes: []string{"storage_read", "storage_write", "storage_delete"}}), ","); got != "storage_list,storage_read,storage_write,storage_delete" {
		t.Fatalf("full tools: %s", got)
	}
	if got := strings.Join(names(&mcpGrant{ID: 3, UserID: u.userID, Mode: aiModeReadOnly,
		Scopes: []string{"storage_read", "storage_write", "storage_delete"}}), ","); got != "storage_list,storage_read" {
		t.Fatalf("read-only mode tools: %s", got)
	}

	list, err := mcpStorageList(readOnly, map[string]interface{}{"storage": "hetzner"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if objs, _ := list["objects"].([]s3Entry); len(objs) != 1 || objs[0].Key != "config.json" {
		t.Fatalf("list objects: %+v", list["objects"])
	}

	read, err := mcpStorageRead(readOnly, map[string]interface{}{"storage": "hetzner", "key": "config.json"})
	if err != nil || !strings.Contains(read["content"].(string), `"a":1`) {
		t.Fatalf("read: %v %+v", err, read)
	}

	if _, err := mcpStorageWrite(readOnly, map[string]interface{}{"storage": "hetzner", "key": "config.json", "content": "{}"}); err == nil {
		t.Fatal("a read-only grant wrote an object")
	}

	writer := &mcpGrant{ID: 4, UserID: u.userID, Mode: aiModeAsk, Scopes: []string{"storage_read", "storage_write"}}
	write, err := mcpStorageWrite(writer, map[string]interface{}{"storage": "hetzner", "key": "new.json", "content": "{\n  \"b\": 2\n}"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	stub.mu.Lock()
	saved := string(stub.objects["team-a/new.json"])
	stub.mu.Unlock()
	if !strings.Contains(saved, `"b": 2`) {
		t.Fatalf("stored content: %q", saved)
	}
	if write["etag"] == "" {
		t.Fatal("write returned no etag")
	}

	// A conditional write with the old etag must fail once the object changed.
	if _, err := mcpStorageWrite(writer, map[string]interface{}{"storage": "hetzner", "key": "new.json",
		"content": "{\"b\":3}", "etag": "stale"}); err == nil {
		t.Fatal("a stale etag was accepted")
	}

	if _, err := mcpStorageDelete(writer, map[string]interface{}{"storage": "hetzner", "key": "new.json", "confirm": "delete"}); err == nil {
		t.Fatal("a grant without storage_delete deleted an object")
	}
	deleter := &mcpGrant{ID: 5, UserID: u.userID, Mode: aiModeAsk, Scopes: []string{"storage_read", "storage_delete"}}
	if _, err := mcpStorageDelete(deleter, map[string]interface{}{"storage": "hetzner", "key": "new.json"}); err == nil {
		t.Fatal("delete without the confirm argument was accepted")
	}
	if _, err := mcpStorageDelete(deleter, map[string]interface{}{"storage": "hetzner", "key": "new.json", "confirm": "delete"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	stub.mu.Lock()
	_, still := stub.objects["team-a/new.json"]
	stub.mu.Unlock()
	if still {
		t.Fatal("the object survived the delete")
	}

	// A read-only mode refuses the write even when the scope is present.
	roMode := &mcpGrant{ID: 6, UserID: u.userID, Mode: aiModeReadOnly, Scopes: []string{"storage_read", "storage_write"}}
	if _, err := mcpStorageWrite(roMode, map[string]interface{}{"storage": "hetzner", "key": "config.json", "content": "{}"}); err == nil {
		t.Fatal("a read-only mode wrote an object")
	}
}
