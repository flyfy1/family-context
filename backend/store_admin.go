package main

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *store) getMember(ctx context.Context, id string) (Member, error) {
	var member Member
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, family_id, name, role, color, created_at FROM members WHERE id = ?`, id).
		Scan(&member.ID, &member.FamilyID, &member.Name, &member.Role, &member.Color, &createdAt)
	if err != nil {
		return Member{}, err
	}
	member.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return member, err
}

func (s *store) updateMember(ctx context.Context, member Member, changedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE members SET name = ?, role = ?, color = ? WHERE id = ?`, member.Name, member.Role, member.Color, member.ID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if err := appendAudit(ctx, tx, "member.updated", "member", member.ID, member, changedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) setMemberTokenHash(ctx context.Context, memberID, tokenHash string, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO member_tokens(member_id, token_hash, created_at) VALUES(?, ?, ?)
		ON CONFLICT(member_id) DO UPDATE SET token_hash = excluded.token_hash, created_at = excluded.created_at`, memberID, tokenHash, createdAt.Format(time.RFC3339Nano))
	return err
}

func (s *store) memberByTokenHash(ctx context.Context, tokenHash string) (Member, error) {
	var member Member
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT m.id, m.family_id, m.name, m.role, m.color, m.created_at
		FROM members m JOIN member_tokens t ON t.member_id = m.id WHERE t.token_hash = ?`, tokenHash).
		Scan(&member.ID, &member.FamilyID, &member.Name, &member.Role, &member.Color, &createdAt)
	if err != nil {
		return Member{}, err
	}
	member.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return member, err
}

func (s *store) getMemberSettings(ctx context.Context, memberID string) (MemberSettings, error) {
	var settings MemberSettings
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT member_id, share_mode, share_prompt, updated_at FROM member_settings WHERE member_id = ?`, memberID).
		Scan(&settings.MemberID, &settings.ShareMode, &settings.SharePrompt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MemberSettings{MemberID: memberID, ShareMode: "manual"}, nil
	}
	if err != nil {
		return MemberSettings{}, err
	}
	settings.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	return settings, err
}

func (s *store) saveMemberSettings(ctx context.Context, settings MemberSettings) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO member_settings(member_id, share_mode, share_prompt, updated_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(member_id) DO UPDATE SET share_mode = excluded.share_mode, share_prompt = excluded.share_prompt, updated_at = excluded.updated_at`,
		settings.MemberID, settings.ShareMode, settings.SharePrompt, settings.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "member.share_policy_updated", "member", settings.MemberID, map[string]any{
		"shareMode": settings.ShareMode, "hasPrompt": settings.SharePrompt != "",
	}, settings.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) listAllUpdates(ctx context.Context, familyID, memberID string) ([]Update, error) {
	query := `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at FROM updates WHERE family_id = ?`
	args := []any{familyID}
	if memberID != "" {
		query += ` AND member_id = ?`
		args = append(args, memberID)
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

func (s *store) updateVisibility(ctx context.Context, updateID, visibility string, changedAt time.Time) (Update, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Update{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE updates SET visibility = ? WHERE id = ?`, visibility, updateID)
	if err != nil {
		return Update{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return Update{}, sql.ErrNoRows
	}
	row := tx.QueryRowContext(ctx, `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at FROM updates WHERE id = ?`, updateID)
	update, err := scanUpdate(row)
	if err != nil {
		return Update{}, err
	}
	if err := appendAudit(ctx, tx, "update.visibility_changed", "update", updateID, map[string]any{"visibility": visibility}, changedAt); err != nil {
		return Update{}, err
	}
	if err := tx.Commit(); err != nil {
		return Update{}, err
	}
	return update, nil
}
