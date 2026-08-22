package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *store) createMediaImport(ctx context.Context, item MediaImport, mediaFile string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var capturedAt any
	if item.CapturedAt != nil {
		capturedAt = item.CapturedAt.Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO media_imports(id, family_id, member_id, media_type, mime_type, original_name, media_file, captured_at, device_id, client_media_id, sha256, analysis_status, analysis_json, analysis_error, share_decision, update_id, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, '', ?, ?)`, item.ID, item.FamilyID, item.MemberID, item.MediaType, item.MimeType,
		item.OriginalName, mediaFile, capturedAt, item.DeviceID, item.ClientMediaID, item.SHA256, item.AnalysisStatus, item.ShareDecision, item.CreatedAt.Format(time.RFC3339Nano), item.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "media_import.created", "media_import", item.ID, map[string]any{
		"memberId": item.MemberID, "mediaType": item.MediaType, "mimeType": item.MimeType, "mediaFile": mediaFile,
	}, item.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) saveMediaImportAnalysis(ctx context.Context, item MediaImport) error {
	analysisJSON := ""
	if item.Analysis != nil {
		data, err := json.Marshal(item.Analysis)
		if err != nil {
			return err
		}
		analysisJSON = string(data)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE media_imports SET analysis_status = ?, analysis_json = ?, analysis_error = ?, updated_at = ? WHERE id = ? AND member_id = ?`,
		item.AnalysisStatus, analysisJSON, item.AnalysisError, item.UpdatedAt.Format(time.RFC3339Nano), item.ID, item.MemberID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if err := appendAudit(ctx, tx, "media_import.analyzed", "media_import", item.ID, map[string]any{"status": item.AnalysisStatus}, item.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) listMediaImports(ctx context.Context, memberID string) ([]MediaImport, error) {
	rows, err := s.db.QueryContext(ctx, mediaImportSelect+` WHERE member_id = ? ORDER BY created_at DESC`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]MediaImport, 0)
	for rows.Next() {
		item, err := scanMediaImport(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *store) getMediaImport(ctx context.Context, id, memberID string) (MediaImport, error) {
	return scanMediaImport(s.db.QueryRowContext(ctx, mediaImportSelect+` WHERE id = ? AND member_id = ?`, id, memberID))
}

func (s *store) mediaImportByClientID(ctx context.Context, memberID, deviceID, clientMediaID string) (MediaImport, error) {
	return scanMediaImport(s.db.QueryRowContext(ctx, mediaImportSelect+` WHERE member_id = ? AND device_id = ? AND client_media_id = ?`, memberID, deviceID, clientMediaID))
}

func (s *store) markMediaImportPrivate(ctx context.Context, id, memberID string, changedAt time.Time) (MediaImport, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaImport{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE media_imports SET share_decision = 'private', updated_at = ? WHERE id = ? AND member_id = ? AND update_id = ''`, changedAt.Format(time.RFC3339Nano), id, memberID)
	if err != nil {
		return MediaImport{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		item, getErr := scanMediaImport(tx.QueryRowContext(ctx, mediaImportSelect+` WHERE id = ? AND member_id = ?`, id, memberID))
		if getErr != nil {
			return MediaImport{}, getErr
		}
		if item.UpdateID != "" {
			return MediaImport{}, errors.New("media import is already shared")
		}
	}
	if err := appendAudit(ctx, tx, "media_import.kept_private", "media_import", id, map[string]any{}, changedAt); err != nil {
		return MediaImport{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaImport{}, err
	}
	return s.getMediaImport(ctx, id, memberID)
}

func (s *store) shareMediaImport(ctx context.Context, id, memberID string, update Update, mediaFile string, changedAt time.Time) (MediaImport, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaImport{}, err
	}
	defer tx.Rollback()
	var existingUpdateID string
	err = tx.QueryRowContext(ctx, `SELECT update_id FROM media_imports WHERE id = ? AND member_id = ?`, id, memberID).Scan(&existingUpdateID)
	if err != nil {
		return MediaImport{}, err
	}
	if existingUpdateID == "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO updates(id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at)
			VALUES(?, ?, ?, ?, ?, 'family', ?, '', ?, ?, ?)`, update.ID, update.FamilyID, update.MemberID, update.Type, update.Text, mediaFile, update.AISummary, update.Source, update.CreatedAt.Format(time.RFC3339Nano))
		if err != nil {
			return MediaImport{}, err
		}
		if err := appendAudit(ctx, tx, "update.created", "update", update.ID, update, update.CreatedAt); err != nil {
			return MediaImport{}, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE media_imports SET share_decision = 'family', update_id = ?, updated_at = ? WHERE id = ? AND member_id = ?`, update.ID, changedAt.Format(time.RFC3339Nano), id, memberID)
		if err != nil {
			return MediaImport{}, err
		}
		if err := appendAudit(ctx, tx, "media_import.shared", "media_import", id, map[string]any{"updateId": update.ID}, changedAt); err != nil {
			return MediaImport{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return MediaImport{}, err
	}
	return s.getMediaImport(ctx, id, memberID)
}

const mediaImportSelect = `SELECT id, family_id, member_id, media_type, mime_type, original_name, media_file, captured_at, device_id, client_media_id, sha256, analysis_status, analysis_json, analysis_error, share_decision, update_id, created_at, updated_at FROM media_imports`

func scanMediaImport(row rowScanner) (MediaImport, error) {
	var item MediaImport
	var mediaFile, analysisJSON, createdAt, updatedAt string
	var capturedAt sql.NullString
	if err := row.Scan(&item.ID, &item.FamilyID, &item.MemberID, &item.MediaType, &item.MimeType, &item.OriginalName, &mediaFile, &capturedAt,
		&item.DeviceID, &item.ClientMediaID, &item.SHA256, &item.AnalysisStatus, &analysisJSON, &item.AnalysisError, &item.ShareDecision, &item.UpdateID, &createdAt, &updatedAt); err != nil {
		return MediaImport{}, err
	}
	item.MediaURL = "/space-files/members/" + item.MemberID + "/media/" + mediaFile
	if capturedAt.Valid {
		value, err := time.Parse(time.RFC3339Nano, capturedAt.String)
		if err != nil {
			return MediaImport{}, err
		}
		item.CapturedAt = &value
	}
	if analysisJSON != "" {
		var analysis MediaAnalysis
		if err := json.Unmarshal([]byte(analysisJSON), &analysis); err != nil {
			return MediaImport{}, err
		}
		item.Analysis = &analysis
	}
	var err error
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return MediaImport{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	return item, err
}
