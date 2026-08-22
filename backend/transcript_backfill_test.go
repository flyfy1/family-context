package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTranscriptBackfillProcessesEachMediaOnce(t *testing.T) {
	storageRoot := t.TempDir()
	spacesRoot, err := prepareSpacesRoot(storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openStore(filepath.Join(storageRoot, "family-daily.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC().Format(time.RFC3339Nano)
	memberID := "member-backfill"
	if _, err := store.db.Exec(`INSERT INTO members(id, family_id, name, role, color, created_at) VALUES(?, ?, ?, ?, ?, ?)`, memberID, defaultFamilyID, "Tester", "member", "blue", now); err != nil {
		t.Fatal(err)
	}
	if err := createMemberSpace(spacesRoot, Member{ID: memberID, FamilyID: defaultFamilyID, Name: "Tester", Role: "member", Color: "blue", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	mediaDir := filepath.Join(storageRoot, "media")
	memberMediaDir := filepath.Join(spacesRoot, "members", memberID, "media")
	activityDir := filepath.Join(spacesRoot, "members", memberID, "activities", "thread-backfill")
	for _, dir := range []string{mediaDir, memberMediaDir, activityDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string][]byte{
		filepath.Join(mediaDir, "answer.wav"):       []byte("audio"),
		filepath.Join(memberMediaDir, "voice.wav"):  []byte("audio"),
		filepath.Join(memberMediaDir, "video.webm"): []byte("video"),
		filepath.Join(activityDir, "activity.webm"): []byte("video"),
	} {
		if err := os.WriteFile(path, data, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO questions(id, family_id, asked_by, asked_to, text, status, created_at) VALUES('question-backfill', ?, 'A', 'B', 'Q', 'answered', ?)`, []any{defaultFamilyID, now}},
		{`INSERT INTO archived_answers(id, question_id, answered_by, audio_file, original_status, error_message, created_at, archived_at) VALUES('answer-backfill', 'question-backfill', 'B', 'answer.wav', 'processing_failed', 'old', ?, ?)`, []any{now, now}},
		{`INSERT INTO updates(id, family_id, member_id, type, text, visibility, audio_file, source, created_at) VALUES('voice-backfill', ?, ?, 'voice', 'failed', 'private', 'voice.wav', 'member_voice_processing_failed', ?)`, []any{defaultFamilyID, memberID, now}},
		{`INSERT INTO updates(id, family_id, member_id, type, text, visibility, audio_file, source, created_at) VALUES('video-update-backfill', ?, ?, 'video', 'video', 'family', 'video.webm', 'mobile_media_import', ?)`, []any{defaultFamilyID, memberID, now}},
		{`INSERT INTO media_imports(id, family_id, member_id, media_type, mime_type, original_name, media_file, sha256, analysis_status, share_decision, update_id, created_at, updated_at) VALUES('video-backfill', ?, ?, 'video', 'video/webm', 'video.webm', 'video.webm', 'digest', 'failed', 'family', 'video-update-backfill', ?, ?)`, []any{defaultFamilyID, memberID, now, now}},
		{`INSERT INTO activity_threads(id, family_id, title, topic, creator_member_id, created_at) VALUES('thread-backfill', ?, 'T', 'T', ?, ?)`, []any{defaultFamilyID, memberID, now}},
		{`INSERT INTO activity_thread_members(thread_id, member_id) VALUES('thread-backfill', ?)`, []any{memberID}},
		{`INSERT INTO activity_posts(id, thread_id, member_id, post_type, media_file, mime_type, created_at) VALUES('activity-backfill', 'thread-backfill', ?, 'video', 'activity.webm', 'video/webm', ?)`, []any{memberID, now}},
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	report, err := runTranscriptBackfill(context.Background(), store, stubAudioProcessor{}, storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 || report.Completed["archived_answer"] != 1 || report.Completed["voice_update"] != 1 || report.Completed["video_import"] != 1 || report.Completed["activity_video"] != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	checks := []struct {
		query string
		args  []any
	}{
		{`SELECT transcript FROM archived_answers WHERE id = ?`, []any{"answer-backfill"}},
		{`SELECT transcript FROM updates WHERE id = ?`, []any{"voice-backfill"}},
		{`SELECT transcript FROM media_imports WHERE id = ?`, []any{"video-backfill"}},
		{`SELECT transcript FROM updates WHERE id = ?`, []any{"video-update-backfill"}},
		{`SELECT transcript FROM activity_posts WHERE id = ?`, []any{"activity-backfill"}},
	}
	for _, check := range checks {
		var transcript string
		if err := store.db.QueryRow(check.query, check.args...).Scan(&transcript); err != nil || transcript == "" {
			t.Fatalf("missing backfilled transcript for %q: %q, %v", check.query, transcript, err)
		}
	}
	second, err := runTranscriptBackfill(context.Background(), store, stubAudioProcessor{}, storageRoot)
	if err != nil || len(second.Found) != 0 || len(second.Failures) != 0 {
		t.Fatalf("backfill is not idempotent: %+v, %v", second, err)
	}
}
