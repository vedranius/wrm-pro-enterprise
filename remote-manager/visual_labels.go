// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"fmt"
	"regexp"
)

type visualLabel struct {
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

var visualIcons = []string{"server", "rack", "network", "router", "switch", "database", "storage", "backup", "security", "cloud", "monitoring", "video", "broadcast", "encoder", "audio", "terminal", "linux", "windows"}
var visualColorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validateVisualLabel(v *visualLabel) error {
	if v == nil {
		return nil
	}
	if v.Color != "" && !visualColorRE.MatchString(v.Color) {
		return fmt.Errorf("color must be #RRGGBB or empty")
	}
	if v.Icon != "" && !oneOf(v.Icon, visualIcons...) {
		return fmt.Errorf("unsupported visual icon")
	}
	return nil
}
func initVisualLabels() {
	ensureTable("visual_labels", `CREATE TABLE IF NOT EXISTS visual_labels (kind TEXT NOT NULL, ref_id INTEGER NOT NULL, user_id INTEGER NOT NULL, color TEXT NOT NULL DEFAULT '', icon TEXT NOT NULL DEFAULT '', PRIMARY KEY(kind,ref_id))`, "kind", "ref_id", "user_id", "color", "icon")
}
func loadVisualLabel(kind string, id int) *visualLabel {
	var v visualLabel
	if db.QueryRow(`SELECT color,icon FROM visual_labels WHERE kind=? AND ref_id=?`, kind, id).Scan(&v.Color, &v.Icon) != nil {
		return nil
	}
	return &v
}

type visualExecutor interface {
	Exec(string, ...interface{}) (sql.Result, error)
}

func saveVisualLabel(ex visualExecutor, kind string, id, uid int, v *visualLabel) error {
	if v == nil {
		return nil
	}
	if err := validateVisualLabel(v); err != nil {
		return err
	}
	_, err := ex.Exec(`INSERT INTO visual_labels(kind,ref_id,user_id,color,icon) VALUES(?,?,?,?,?) ON CONFLICT(kind,ref_id) DO UPDATE SET color=excluded.color,icon=excluded.icon,user_id=excluded.user_id`, kind, id, uid, v.Color, v.Icon)
	return err
}
