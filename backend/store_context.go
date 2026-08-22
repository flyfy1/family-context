package main

import (
	"context"
	"time"
)

func (s *store) createMember(ctx context.Context, member Member) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO members(id, family_id, name, role, color, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		member.ID, member.FamilyID, member.Name, member.Role, member.Color, member.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "member.created", "member", member.ID, member, member.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) listMembers(ctx context.Context, familyID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, family_id, name, role, color, created_at FROM members WHERE family_id = ? ORDER BY created_at`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]Member, 0)
	for rows.Next() {
		var member Member
		var createdAt string
		if err := rows.Scan(&member.ID, &member.FamilyID, &member.Name, &member.Role, &member.Color, &createdAt); err != nil {
			return nil, err
		}
		member.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *store) memberExists(ctx context.Context, id, familyID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM members WHERE id = ? AND family_id = ?`, id, familyID).Scan(&count)
	return count == 1, err
}

func (s *store) createUpdate(ctx context.Context, update Update, audioFile string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO updates(id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, update.ID, update.FamilyID, update.MemberID, update.Type, update.Text,
		update.Visibility, audioFile, update.Transcript, update.AISummary, update.Source, update.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "update.created", "update", update.ID, update, update.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) listUpdates(ctx context.Context, familyID, memberID, scope string) ([]Update, error) {
	query := `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at FROM updates WHERE family_id = ?`
	args := []any{familyID}
	if scope == "mine" {
		query += ` AND member_id = ?`
		args = append(args, memberID)
	} else {
		query += ` AND visibility = 'family'`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	updates := make([]Update, 0)
	for rows.Next() {
		update, err := scanUpdate(rows)
		if err != nil {
			return nil, err
		}
		updates = append(updates, update)
	}
	return updates, rows.Err()
}

func (s *store) sharedUpdatesForDate(ctx context.Context, familyID, date string) ([]Update, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at
		FROM updates WHERE family_id = ? AND visibility = 'family' AND substr(created_at, 1, 10) = ? ORDER BY created_at`, familyID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	updates := make([]Update, 0)
	for rows.Next() {
		update, err := scanUpdate(rows)
		if err != nil {
			return nil, err
		}
		updates = append(updates, update)
	}
	return updates, rows.Err()
}

func (s *store) createDailySummary(ctx context.Context, summary DailySummary) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO daily_summaries(id, family_id, summary_date, content, update_count, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		summary.ID, summary.FamilyID, summary.Date, summary.Content, summary.UpdateCount, summary.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "daily_summary.created", "daily_summary", summary.ID, summary, summary.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) latestDailySummary(ctx context.Context, familyID string) (DailySummary, error) {
	var summary DailySummary
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, family_id, summary_date, content, update_count, created_at FROM daily_summaries WHERE family_id = ? ORDER BY summary_date DESC, created_at DESC LIMIT 1`, familyID).
		Scan(&summary.ID, &summary.FamilyID, &summary.Date, &summary.Content, &summary.UpdateCount, &createdAt)
	if err != nil {
		return DailySummary{}, err
	}
	summary.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return summary, err
}

func scanUpdate(row rowScanner) (Update, error) {
	var update Update
	var audioFile, createdAt string
	if err := row.Scan(&update.ID, &update.FamilyID, &update.MemberID, &update.Type, &update.Text, &update.Visibility,
		&audioFile, &update.Transcript, &update.AISummary, &update.Source, &createdAt); err != nil {
		return Update{}, err
	}
	if audioFile != "" {
		update.AudioURL = "/space-files/members/" + update.MemberID + "/media/" + audioFile
	}
	var err error
	update.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return update, err
}
