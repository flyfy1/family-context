package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type transcriptBackfillReport struct {
	Found     map[string]int              `json:"found"`
	Completed map[string]int              `json:"completed"`
	Failures  []transcriptBackfillFailure `json:"failures"`
}

type transcriptBackfillFailure struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Error string `json:"error"`
}

type transcriptBackfillTarget struct {
	Kind     string
	ID       string
	MemberID string
	File     string
	MimeType string
	UpdateID string
}

func runTranscriptBackfill(ctx context.Context, s *store, ai audioProcessor, storageRoot string) (transcriptBackfillReport, error) {
	report := transcriptBackfillReport{Found: map[string]int{}, Completed: map[string]int{}, Failures: []transcriptBackfillFailure{}}
	if _, err := prepareSpacesRoot(storageRoot); err != nil {
		return report, fmt.Errorf("prepare spaces: %w", err)
	}
	targets, err := transcriptBackfillTargets(ctx, s)
	if err != nil {
		return report, err
	}
	transcriber, canTranscribe := ai.(mediaTranscriber)
	for _, target := range targets {
		report.Found[target.Kind]++
		mediaPath := transcriptBackfillPath(storageRoot, target)
		data, readErr := os.ReadFile(mediaPath)
		if readErr != nil {
			report.Failures = append(report.Failures, transcriptBackfillFailure{Kind: target.Kind, ID: target.ID, Error: "read media: " + readErr.Error()})
			continue
		}
		mimeType := target.MimeType
		if mimeType == "" {
			mimeType = transcriptMIMEType(target.File, target.Kind)
		}
		var transcript, summary string
		if target.Kind == "answer" || target.Kind == "archived_answer" || target.Kind == "voice_update" {
			result, processErr := ai.Process(ctx, data, mimeType)
			if processErr != nil {
				report.Failures = append(report.Failures, transcriptBackfillFailure{Kind: target.Kind, ID: target.ID, Error: processErr.Error()})
				continue
			}
			transcript, summary = strings.TrimSpace(result.Transcript), strings.TrimSpace(result.Summary)
		} else {
			if !canTranscribe {
				report.Failures = append(report.Failures, transcriptBackfillFailure{Kind: target.Kind, ID: target.ID, Error: "AI processor cannot transcribe media"})
				continue
			}
			value, transcribeErr := transcriber.Transcribe(ctx, data, mimeType)
			if transcribeErr != nil {
				report.Failures = append(report.Failures, transcriptBackfillFailure{Kind: target.Kind, ID: target.ID, Error: transcribeErr.Error()})
				continue
			}
			transcript = strings.TrimSpace(value)
		}
		if transcript == "" {
			report.Failures = append(report.Failures, transcriptBackfillFailure{Kind: target.Kind, ID: target.ID, Error: "empty transcript"})
			continue
		}
		if summary == "" {
			summary = transcript
		}
		if err := saveBackfilledTranscript(ctx, s, target, transcript, summary, time.Now().UTC()); err != nil {
			report.Failures = append(report.Failures, transcriptBackfillFailure{Kind: target.Kind, ID: target.ID, Error: "save transcript: " + err.Error()})
			continue
		}
		if err := persistBackfilledTranscript(ctx, s, storageRoot, target); err != nil {
			report.Failures = append(report.Failures, transcriptBackfillFailure{Kind: target.Kind, ID: target.ID, Error: "persist space metadata: " + err.Error()})
			continue
		}
		report.Completed[target.Kind]++
	}
	return report, nil
}

func transcriptBackfillTargets(ctx context.Context, s *store) ([]transcriptBackfillTarget, error) {
	queries := []struct {
		kind  string
		query string
		scan  func(*sql.Rows) (transcriptBackfillTarget, error)
	}{
		{kind: "answer", query: `SELECT id, audio_file FROM answers WHERE trim(transcript) = ''`, scan: scanSimpleBackfillTarget("answer")},
		{kind: "archived_answer", query: `SELECT id, audio_file FROM archived_answers WHERE trim(transcript) = ''`, scan: scanSimpleBackfillTarget("archived_answer")},
		{kind: "voice_update", query: `SELECT id, member_id, audio_file FROM updates WHERE type = 'voice' AND trim(transcript) = ''`, scan: scanMemberBackfillTarget("voice_update")},
		{kind: "video_import", query: `SELECT id, member_id, media_file, mime_type, update_id FROM media_imports WHERE media_type = 'video' AND trim(transcript) = ''`, scan: func(rows *sql.Rows) (transcriptBackfillTarget, error) {
			var target transcriptBackfillTarget
			target.Kind = "video_import"
			err := rows.Scan(&target.ID, &target.MemberID, &target.File, &target.MimeType, &target.UpdateID)
			return target, err
		}},
		{kind: "activity_video", query: `SELECT id, member_id, media_file, mime_type, thread_id FROM activity_posts WHERE post_type = 'video' AND trim(transcript) = ''`, scan: func(rows *sql.Rows) (transcriptBackfillTarget, error) {
			var target transcriptBackfillTarget
			target.Kind = "activity_video"
			err := rows.Scan(&target.ID, &target.MemberID, &target.File, &target.MimeType, &target.UpdateID)
			return target, err
		}},
	}
	var targets []transcriptBackfillTarget
	for _, item := range queries {
		rows, err := s.db.QueryContext(ctx, item.query)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", item.kind, err)
		}
		for rows.Next() {
			target, scanErr := item.scan(rows)
			if scanErr != nil {
				rows.Close()
				return nil, scanErr
			}
			targets = append(targets, target)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func scanSimpleBackfillTarget(kind string) func(*sql.Rows) (transcriptBackfillTarget, error) {
	return func(rows *sql.Rows) (transcriptBackfillTarget, error) {
		target := transcriptBackfillTarget{Kind: kind}
		err := rows.Scan(&target.ID, &target.File)
		return target, err
	}
}

func scanMemberBackfillTarget(kind string) func(*sql.Rows) (transcriptBackfillTarget, error) {
	return func(rows *sql.Rows) (transcriptBackfillTarget, error) {
		target := transcriptBackfillTarget{Kind: kind}
		err := rows.Scan(&target.ID, &target.MemberID, &target.File)
		return target, err
	}
}

func transcriptBackfillPath(storageRoot string, target transcriptBackfillTarget) string {
	switch target.Kind {
	case "answer", "archived_answer":
		return filepath.Join(storageRoot, "media", filepath.Base(target.File))
	case "activity_video":
		return filepath.Join(storageRoot, "spaces", "members", target.MemberID, "activities", target.UpdateID, filepath.Base(target.File))
	default:
		return filepath.Join(storageRoot, "spaces", "members", target.MemberID, "media", filepath.Base(target.File))
	}
}

func transcriptMIMEType(name, kind string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".m4a":
		return "video/mp4"
	case ".mp4":
		if strings.Contains(kind, "video") {
			return "video/mp4"
		}
		return "audio/mp4"
	case ".aiff", ".aif":
		return "audio/aiff"
	case ".weba":
		return "video/webm"
	case ".webm":
		if strings.Contains(kind, "video") {
			return "video/webm"
		}
		return "audio/webm"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mp3"
	}
	value := strings.Split(mime.TypeByExtension(ext), ";")[0]
	if value == "" {
		return "application/octet-stream"
	}
	return value
}

func saveBackfilledTranscript(ctx context.Context, s *store, target transcriptBackfillTarget, transcript, summary string, changedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var result sql.Result
	switch target.Kind {
	case "answer":
		result, err = tx.ExecContext(ctx, `UPDATE answers SET transcript = ?, ai_summary = ?, status = CASE WHEN status = 'processing_failed' THEN 'ready' ELSE status END, error_message = CASE WHEN status = 'processing_failed' THEN '' ELSE error_message END WHERE id = ? AND trim(transcript) = ''`, transcript, summary, target.ID)
	case "archived_answer":
		result, err = tx.ExecContext(ctx, `UPDATE archived_answers SET transcript = ?, ai_summary = ? WHERE id = ? AND trim(transcript) = ''`, transcript, summary, target.ID)
	case "voice_update":
		result, err = tx.ExecContext(ctx, `UPDATE updates SET text = CASE WHEN source = 'member_voice_processing_failed' THEN ? ELSE text END, transcript = ?, ai_summary = ?, source = CASE WHEN source = 'member_voice_processing_failed' THEN 'member_voice_backfilled' ELSE source END WHERE id = ? AND trim(transcript) = ''`, summary, transcript, summary, target.ID)
	case "video_import":
		result, err = tx.ExecContext(ctx, `UPDATE media_imports SET transcript = ?, updated_at = ? WHERE id = ? AND trim(transcript) = ''`, transcript, changedAt.Format(time.RFC3339Nano), target.ID)
		if err == nil && target.UpdateID != "" {
			_, err = tx.ExecContext(ctx, `UPDATE updates SET transcript = ? WHERE id = ? AND trim(transcript) = ''`, transcript, target.UpdateID)
		}
	case "activity_video":
		result, err = tx.ExecContext(ctx, `UPDATE activity_posts SET transcript = ? WHERE id = ? AND trim(transcript) = ''`, transcript, target.ID)
	default:
		return errors.New("unknown transcript target")
	}
	if err != nil {
		return err
	}
	if result != nil {
		if rows, _ := result.RowsAffected(); rows == 0 {
			return sql.ErrNoRows
		}
	}
	if err := appendAudit(ctx, tx, "transcript.backfilled", target.Kind, target.ID, map[string]any{"hasTranscript": true}, changedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func persistBackfilledTranscript(ctx context.Context, s *store, storageRoot string, target transcriptBackfillTarget) error {
	spacesRoot := filepath.Join(storageRoot, "spaces")
	switch target.Kind {
	case "voice_update":
		update, err := scanUpdate(s.db.QueryRowContext(ctx, `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at FROM updates WHERE id = ?`, target.ID))
		if err != nil {
			return err
		}
		return persistUpdateToSpace(spacesRoot, update)
	case "video_import":
		item, err := s.getMediaImport(ctx, target.ID, target.MemberID)
		if err != nil {
			return err
		}
		if err := persistMediaImportToSpace(spacesRoot, item); err != nil {
			return err
		}
		if target.UpdateID != "" {
			update, err := scanUpdate(s.db.QueryRowContext(ctx, `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at FROM updates WHERE id = ?`, target.UpdateID))
			if err != nil {
				return err
			}
			return persistUpdateToSpace(spacesRoot, update)
		}
	}
	return nil
}
