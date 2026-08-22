package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func prepareSpacesRoot(storageRoot string) (string, error) {
	root := filepath.Join(storageRoot, "spaces")
	for _, dir := range []string{
		filepath.Join(root, "members"),
		filepath.Join(root, "shared", "updates"),
		filepath.Join(root, "shared", "media"),
		filepath.Join(root, "shared", "summaries"),
		filepath.Join(root, "shared", "sources"),
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", err
		}
	}
	return root, nil
}

func createMemberSpace(spacesRoot string, member Member) error {
	root := filepath.Join(spacesRoot, "members", member.ID)
	for _, name := range []string{"private", "updates", "media", "summaries", "jobs"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(member, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomically(root, "profile.json", append(data, '\n'))
}

func persistUpdateToSpace(spacesRoot string, update Update) error {
	memberDir := filepath.Join(spacesRoot, "members", update.MemberID, "updates")
	if err := os.MkdirAll(memberDir, 0o750); err != nil {
		return err
	}
	content := fmt.Sprintf("---\nid: %s\nfamily_id: %s\nmember_id: %s\ntype: %s\nvisibility: %s\nsource: %s\ncreated_at: %s\n---\n\n%s\n",
		update.ID, update.FamilyID, update.MemberID, update.Type, update.Visibility, update.Source, update.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), update.Text)
	if update.Transcript != "" && update.Transcript != update.Text {
		content += "\n## Transcript\n\n" + update.Transcript + "\n"
	}
	if err := writeFileAtomically(memberDir, update.ID+".md", []byte(content)); err != nil {
		return err
	}
	if update.Visibility == "family" {
		sharedData, err := json.MarshalIndent(update, "", "  ")
		if err != nil {
			return err
		}
		return writeFileAtomically(filepath.Join(spacesRoot, "shared", "updates"), update.ID+".json", append(sharedData, '\n'))
	}
	return nil
}

func persistSummaryToSpace(spacesRoot string, summary DailySummary) error {
	name := summary.Date + "-" + summary.ID + ".md"
	content := fmt.Sprintf("---\nid: %s\ndate: %s\nupdate_count: %d\ncreated_at: %s\n---\n\n# 我们家今天\n\n%s\n",
		summary.ID, summary.Date, summary.UpdateCount, summary.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), strings.TrimSpace(summary.Content))
	return writeFileAtomically(filepath.Join(spacesRoot, "shared", "summaries"), name, []byte(content))
}
