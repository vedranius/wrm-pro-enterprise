// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ─── AI: ADDON TOOLS ─────────────────────────────────
//
// These tools act on WRM itself, not on the session's server: the model drafts
// an addon in <DB>.addons/.drafts/<id>/ and the user reviews and installs it in
// Settings → Addons. Every write and every run asks for approval in every mode;
// a draft is never enabled by a tool.

const aiAddonPreviewLimit = 100 << 10

func aiAddonID(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !addonIDPattern.MatchString(v) {
		return "", fmt.Errorf("the addon id must be 2-64 characters: lowercase letters, digits, dot, dash or underscore")
	}
	return v, nil
}

// aiAddonContent returns the bytes of an addon_write file: text or base64.
func aiAddonContent(in aiToolInput) ([]byte, error) {
	if in.Content != "" && in.ContentBase64 != "" {
		return nil, fmt.Errorf("give content or content_base64, not both")
	}
	if in.ContentBase64 != "" {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.ContentBase64))
		if err != nil {
			return nil, fmt.Errorf("content_base64 is not valid base64")
		}
		if len(b) > addonMaxFile {
			return nil, fmt.Errorf("the file is larger than 8 MB")
		}
		return b, nil
	}
	if len(in.Content) > addonMaxFile {
		return nil, fmt.Errorf("the file is larger than 8 MB")
	}
	return []byte(in.Content), nil
}

// aiAddonPreview shows the change a draft write would make, before the user
// approves it. It compares the new state with the current draft (or with
// nothing for a new draft).
func aiAddonPreview(baseDir string, files map[string]string, replace bool) string {
	base := map[string]string{}
	if baseDir != "" {
		if list, _, err := addonDraftFiles(baseDir); err == nil {
			for _, f := range list {
				if s, ok := addonDraftRead(baseDir, f.Path); ok {
					base[f.Path] = s
				} else {
					base[f.Path] = "\x00"
				}
			}
		}
	}
	next := map[string]string{}
	if !replace {
		for k, v := range base {
			next[k] = v
		}
	}
	for k, v := range files {
		next[k] = v
	}
	names := map[string]bool{}
	for k := range base {
		names[k] = true
	}
	for k := range next {
		names[k] = true
	}
	sorted := make([]string, 0, len(names))
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var sb strings.Builder
	for _, rel := range sorted {
		old, oldOK := base[rel]
		newText, newOK := next[rel]
		if oldOK && newOK && old == newText {
			continue
		}
		switch {
		case oldOK && strings.Contains(old, "\x00"):
			fmt.Fprintf(&sb, "Binary file %s changed\n", rel)
		case newOK && strings.Contains(newText, "\x00"):
			fmt.Fprintf(&sb, "Binary file %s added or changed (%d bytes)\n", rel, len(newText))
		case oldOK && newOK:
			sb.WriteString(aiUnifiedDiff(rel, old, newText, false))
		case newOK:
			sb.WriteString(aiUnifiedDiff(rel, "", newText, true))
		default:
			sb.WriteString(aiUnifiedDiff(rel, old, "", false))
		}
	}
	out := sb.String()
	if strings.TrimSpace(out) == "" {
		return "(no changes)\n"
	}
	if len(out) > aiAddonPreviewLimit {
		out = out[:aiAddonPreviewLimit] + "\n… (diff truncated)\n"
	}
	return out
}

func (s *aiSession) planHostTool(name string, in aiToolInput) (*aiPlanned, error) {
	host := in
	read := &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Host: &host}
	write := &aiPlanned{Act: aiAction{Tool: name, Kind: "write"}, Host: &host, NoEdit: true}
	switch name {
	case "addon_list":
		read.Display = "list addons and drafts"
		return read, nil
	case "addon_files":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return nil, err
		}
		dir := addonDir(id)
		if in.Draft {
			dir = addonDraftDir(id)
		}
		if _, err := os.Stat(dir); err != nil {
			if in.Draft {
				return nil, fmt.Errorf("draft %s does not exist", id)
			}
			return nil, fmt.Errorf("addon %s is not installed", id)
		}
		if in.Path != "" {
			rel, err := cleanDraftRel(in.Path)
			if err != nil {
				return nil, err
			}
			host.Path = rel
			read.Display = "read " + id + "/" + rel
		} else {
			read.Display = "list files of " + id
		}
		return read, nil
	case "addon_create":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return nil, err
		}
		if len(in.Files) == 0 {
			return nil, fmt.Errorf("give at least addon.json and the files it declares")
		}
		if _, ok := in.Files["addon.json"]; !ok {
			return nil, fmt.Errorf("files must include addon.json at the root")
		}
		var total int
		for rel, content := range in.Files {
			if !addonSafeRel(strings.TrimSpace(rel)) {
				return nil, fmt.Errorf("unsafe path %q: use a relative path inside the addon", rel)
			}
			if len(content) > addonMaxFile {
				return nil, fmt.Errorf("%s is larger than 8 MB", rel)
			}
			total += len(content)
		}
		if total > addonMaxArchive {
			return nil, fmt.Errorf("the draft is larger than 20 MB")
		}
		write.Path = "draft:" + id
		write.Diff = aiAddonPreview(addonDraftDir(id), in.Files, true)
		write.Display = fmt.Sprintf("create addon draft %s (%d files)", id, len(in.Files))
		if in.Install {
			write.Display += " and install it (a new addon stays disabled)"
		}
		return write, nil
	case "addon_write":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return nil, err
		}
		rel, err := cleanDraftRel(in.Path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(addonDraftDir(id)); err != nil {
			return nil, fmt.Errorf("draft %s does not exist; create it first with addon_create", id)
		}
		raw, err := aiAddonContent(in)
		if err != nil {
			return nil, err
		}
		host.Content = string(raw)
		host.ContentBase64 = ""
		write.Path = "draft:" + id + "/" + rel
		write.Diff = aiAddonPreview(addonDraftDir(id), map[string]string{rel: host.Content}, false)
		write.Display = "write draft " + id + "/" + rel
		if in.Install {
			write.Display += " and install the draft"
		}
		return write, nil
	case "addon_enable", "addon_disable", "addon_delete":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return nil, err
		}
		write.Path = "addon:" + id
		write.Act.Command = name + " " + id
		write.Display = strings.Replace(name, "addon_", "", 1) + " addon " + id
		if name == "addon_delete" {
			write.Display += " (its files are deleted)"
		}
		return write, nil
	case "addon_run":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return nil, err
		}
		if !addonExportPattern.MatchString(in.Export) {
			return nil, fmt.Errorf("the export must be a function name declared in addon.json")
		}
		input := strings.TrimSpace(in.Input)
		if input == "" {
			input = "{}"
		}
		if len(input) > addonInputLimit {
			return nil, fmt.Errorf("the input is larger than 64 KB")
		}
		if !json.Valid([]byte(input)) {
			return nil, fmt.Errorf("the input must be valid JSON")
		}
		host.Input = input
		run := &aiPlanned{Act: aiAction{Tool: name, Kind: "command", Command: "addon run " + id + " " + in.Export},
			Host: &host, NoEdit: true}
		run.Display = "run addon " + id + " export " + in.Export
		return run, nil
	case "addon_install_community":
		id, err := aiAddonID(in.CommunityID)
		if err != nil {
			return nil, fmt.Errorf("community_id: %v", err)
		}
		host.ID = id
		write.Path = "community:" + id
		write.Act.Command = "addon_install_community " + id
		write.Display = "install community addon " + id
		return write, nil
	}
	return nil, fmt.Errorf("unknown addon tool %q", name)
}

func (s *aiSession) runHostTool(ctx context.Context, name string, in aiToolInput) (string, int, error) {
	switch name {
	case "addon_list":
		b, _ := json.MarshalIndent(map[string]interface{}{
			"addons": addonRecords(false), "drafts": addonDraftList(), "capabilities": addonCapabilities,
		}, "", "  ")
		return string(b), 0, nil
	case "addon_files":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return "", 2, err
		}
		dir := addonDir(id)
		if in.Draft {
			dir = addonDraftDir(id)
		}
		files, total, err := addonDraftFiles(dir)
		if err != nil {
			return "", 2, err
		}
		out := map[string]interface{}{"id": id, "draft": in.Draft, "files": files, "total_bytes": total}
		if m, err := addonDraftManifest(dir); err == nil {
			out["manifest"] = m
		}
		if in.Path != "" {
			rel, err := cleanDraftRel(in.Path)
			if err != nil {
				return "", 2, err
			}
			text, ok := addonDraftRead(dir, rel)
			if !ok {
				return "", 2, fmt.Errorf("%s is binary, missing or larger than 2 MB", rel)
			}
			out["path"], out["content"] = rel, text
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		return string(b), 0, nil
	case "addon_create", "addon_write":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return "", 2, err
		}
		files := in.Files
		merge := name == "addon_write"
		if merge {
			rel, err := cleanDraftRel(in.Path)
			if err != nil {
				return "", 2, err
			}
			files = map[string]string{rel: in.Content}
		}
		m, err := addonDraftSave(ctx, id, files, merge)
		if err != nil {
			return "", 2, err
		}
		action := "addon.draft_created"
		if merge {
			action = "addon.draft_written"
		}
		auditLog(nil, s.UserID, action, id, map[string]interface{}{"version": m.Version, "files": len(files)})
		result := map[string]interface{}{"id": id, "version": m.Version, "valid": true, "installed": false, "draft": addonDraftViewOf(id)}
		if in.Install {
			body, err := addonDraftZip(addonDraftDir(id))
			if err != nil {
				return "", 2, err
			}
			inst, status, err := addonInstallArchive(ctx, nil, s.UserID, body, "draft:"+id)
			if err != nil {
				return "", status, err
			}
			result["installed"], result["enabled"], result["installed_version"] = true, inst.Enabled, inst.Version
		}
		b, _ := json.MarshalIndent(result, "", "  ")
		return string(b), 0, nil
	case "addon_enable", "addon_disable":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return "", 2, err
		}
		rec, status, err := addonSetEnabledCore(s.UserID, id, name == "addon_enable")
		if err != nil {
			return "", status, err
		}
		return fmt.Sprintf("Addon %s is %s (version %s).", rec.ID, map[bool]string{true: "enabled", false: "disabled"}[name == "addon_enable"], rec.Version), 0, nil
	case "addon_delete":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return "", 2, err
		}
		rec, status, err := addonRemoveCore(s.UserID, id)
		if err != nil {
			return "", status, err
		}
		return fmt.Sprintf("Addon %s (version %s) is removed.", rec.ID, rec.Version), 0, nil
	case "addon_run":
		id, err := aiAddonID(in.ID)
		if err != nil {
			return "", 2, err
		}
		rec, ok := addonGet(id)
		if !ok {
			return "", 2, fmt.Errorf("addon %s is not installed", id)
		}
		if !rec.Enabled {
			return "", 2, fmt.Errorf("enable the addon before running its code")
		}
		out, err := addonCallWASM(ctx, rec, in.Export, []byte(in.Input))
		if err != nil {
			return "", 2, err
		}
		auditLog(nil, s.UserID, "addon.call", id+"/"+in.Export, map[string]interface{}{"version": rec.Version, "bytes_in": len(in.Input), "bytes_out": len(out), "via": "ai"})
		return out, 0, nil
	case "addon_install_community":
		id, err := aiAddonID(in.CommunityID)
		if err != nil {
			return "", 2, err
		}
		inst, status, err := addonCommunityInstall(ctx, nil, s.UserID, id)
		if err != nil {
			return "", status, err
		}
		return fmt.Sprintf("Community addon %s (version %s) is installed; enabled: %v.", inst.ID, inst.Version, inst.Enabled), 0, nil
	}
	return "", 2, fmt.Errorf("unknown addon tool %q", name)
}
