package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (s *store) createBedtimeStory(ctx context.Context, story BedtimeStory) error {
	sources, err := json.Marshal(story.SourceUpdateIDs)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO bedtime_stories(id, family_id, child_id, child_name, audience_age, language, title, content, source_update_ids_json, voice, audio_file, status, error_message, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?)`, story.ID, story.FamilyID, story.ChildID, story.ChildName, story.AudienceAge, story.Language,
		story.Title, story.Content, string(sources), story.Voice, story.Status, story.ErrorMessage, story.CreatedAt.Format(time.RFC3339Nano), story.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "bedtime_story.created", "bedtime_story", story.ID, map[string]any{
		"childId": story.ChildID, "language": story.Language, "sourceUpdateIds": story.SourceUpdateIDs, "status": story.Status,
	}, story.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) finishBedtimeStoryAudio(ctx context.Context, story BedtimeStory, audioFile string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE bedtime_stories SET audio_file = ?, status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
		audioFile, story.Status, story.ErrorMessage, story.UpdatedAt.Format(time.RFC3339Nano), story.ID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if err := appendAudit(ctx, tx, "bedtime_story.audio_finished", "bedtime_story", story.ID, map[string]any{"status": story.Status, "hasAudio": audioFile != ""}, story.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) getBedtimeStory(ctx context.Context, id, familyID string) (BedtimeStory, error) {
	return scanBedtimeStory(s.db.QueryRowContext(ctx, bedtimeStorySelect+` WHERE id = ? AND family_id = ?`, id, familyID))
}

func (s *store) listBedtimeStories(ctx context.Context, familyID, childID, language string) ([]BedtimeStory, error) {
	query := bedtimeStorySelect + ` WHERE family_id = ? AND language = ?`
	args := []any{familyID, language}
	if childID != "" {
		query += ` AND child_id = ?`
		args = append(args, childID)
	}
	query += ` ORDER BY created_at DESC LIMIT 50`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stories := make([]BedtimeStory, 0)
	for rows.Next() {
		story, err := scanBedtimeStory(rows)
		if err != nil {
			return nil, err
		}
		stories = append(stories, story)
	}
	return stories, rows.Err()
}

const bedtimeStorySelect = `SELECT id, family_id, child_id, child_name, audience_age, language, title, content, source_update_ids_json, voice, audio_file, status, error_message, created_at, updated_at FROM bedtime_stories`

func scanBedtimeStory(row rowScanner) (BedtimeStory, error) {
	var story BedtimeStory
	var sourcesJSON, audioFile, createdAt, updatedAt string
	if err := row.Scan(&story.ID, &story.FamilyID, &story.ChildID, &story.ChildName, &story.AudienceAge, &story.Language, &story.Title, &story.Content,
		&sourcesJSON, &story.Voice, &audioFile, &story.Status, &story.ErrorMessage, &createdAt, &updatedAt); err != nil {
		return BedtimeStory{}, err
	}
	if err := json.Unmarshal([]byte(sourcesJSON), &story.SourceUpdateIDs); err != nil {
		return BedtimeStory{}, err
	}
	if audioFile != "" {
		story.AudioURL = "/api/v1/bedtime-stories/" + story.ID + "/audio"
	}
	var err error
	story.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return BedtimeStory{}, err
	}
	story.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	return story, err
}
