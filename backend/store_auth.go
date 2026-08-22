package main

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *store) migrateMemberAuth(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS member_logins (
  member_id TEXT PRIMARY KEY REFERENCES members(id) ON DELETE CASCADE,
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash BLOB NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS member_web_sessions (
  token_hash TEXT PRIMARY KEY,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_member_web_sessions_member ON member_web_sessions(member_id, expires_at);
`)
	return err
}

func (s *store) saveMemberLogin(ctx context.Context, memberID, username string, passwordHash []byte, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO member_logins(member_id, username, password_hash, updated_at) VALUES(?, ?, ?, ?)
ON CONFLICT(member_id) DO UPDATE SET username=excluded.username, password_hash=excluded.password_hash, updated_at=excluded.updated_at`,
		memberID, username, passwordHash, now.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `UPDATE member_web_sessions SET revoked_at = ? WHERE member_id = ? AND revoked_at IS NULL`, now.Format(time.RFC3339Nano), memberID); err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "member.login_updated", "member", memberID, map[string]any{"username": username}, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) memberLoginStatus(ctx context.Context, memberID string) (MemberLoginStatus, error) {
	status := MemberLoginStatus{MemberID: memberID}
	err := s.db.QueryRowContext(ctx, `SELECT username FROM member_logins WHERE member_id = ?`, memberID).Scan(&status.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return MemberLoginStatus{}, err
	}
	status.HasLogin = true
	return status, nil
}

func (s *store) memberAndPasswordHashByUsername(ctx context.Context, username string) (Member, []byte, error) {
	var member Member
	var createdAt string
	var passwordHash []byte
	err := s.db.QueryRowContext(ctx, `SELECT m.id, m.family_id, m.name, m.role, m.is_admin, m.color, m.created_at, l.password_hash
FROM member_logins l JOIN members m ON m.id = l.member_id WHERE l.username = ? COLLATE NOCASE`, username).
		Scan(&member.ID, &member.FamilyID, &member.Name, &member.Role, &member.IsAdmin, &member.Color, &createdAt, &passwordHash)
	if err != nil {
		return Member{}, nil, err
	}
	member.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return member, passwordHash, err
}

func (s *store) createMemberWebSession(ctx context.Context, memberID, tokenHash string, createdAt, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO member_web_sessions(token_hash, member_id, created_at, expires_at) VALUES(?, ?, ?, ?)`,
		tokenHash, memberID, createdAt.Format(time.RFC3339Nano), expiresAt.Format(time.RFC3339Nano))
	return err
}

func (s *store) memberByWebSessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (Member, error) {
	var member Member
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT m.id, m.family_id, m.name, m.role, m.is_admin, m.color, m.created_at
FROM member_web_sessions s JOIN members m ON m.id = s.member_id
WHERE s.token_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ?`, tokenHash, now.Format(time.RFC3339Nano)).
		Scan(&member.ID, &member.FamilyID, &member.Name, &member.Role, &member.IsAdmin, &member.Color, &createdAt)
	if err != nil {
		return Member{}, err
	}
	member.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return member, err
}

func (s *store) revokeMemberWebSession(ctx context.Context, tokenHash string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE member_web_sessions SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL`, now.Format(time.RFC3339Nano), tokenHash)
	return err
}
