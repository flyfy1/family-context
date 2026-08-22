package main

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type MemberMCPSession struct {
	ID        string     `json:"id"`
	MemberID  string     `json:"memberId"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

func (s *store) migrateMemberMCPSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS member_mcp_sessions (
  id TEXT PRIMARY KEY,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_member_mcp_sessions_member ON member_mcp_sessions(member_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_member_mcp_sessions_token ON member_mcp_sessions(token_hash);
`)
	return err
}

func (s *store) createMemberMCPSession(ctx context.Context, session MemberMCPSession, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO member_mcp_sessions(id, member_id, label, token_hash, created_at, expires_at)
VALUES(?, ?, ?, ?, ?, ?)`, session.ID, session.MemberID, session.Label, tokenHash,
		session.CreatedAt.Format(time.RFC3339Nano), session.ExpiresAt.Format(time.RFC3339Nano))
	return err
}

func (s *store) listMemberMCPSessions(ctx context.Context, memberID string) ([]MemberMCPSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, member_id, label, created_at, expires_at, revoked_at
FROM member_mcp_sessions WHERE member_id = ? ORDER BY created_at DESC`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]MemberMCPSession, 0)
	for rows.Next() {
		session, err := scanMemberMCPSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *store) revokeMemberMCPSession(ctx context.Context, memberID, sessionID string, revokedAt time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE member_mcp_sessions SET revoked_at = ?
WHERE id = ? AND member_id = ? AND revoked_at IS NULL`, revokedAt.Format(time.RFC3339Nano), sessionID, memberID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *store) revokeAllMemberMCPSessions(ctx context.Context, memberID string, revokedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE member_mcp_sessions SET revoked_at = ?
WHERE member_id = ? AND revoked_at IS NULL`, revokedAt.Format(time.RFC3339Nano), memberID)
	return err
}

func (s *store) memberByMCPSessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (Member, error) {
	row := s.db.QueryRowContext(ctx, `SELECT m.id, m.family_id, m.name, m.role, m.color, m.created_at
FROM member_mcp_sessions s JOIN members m ON m.id = s.member_id
WHERE s.token_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ?`, tokenHash, now.Format(time.RFC3339Nano))
	var member Member
	var createdAt string
	if err := row.Scan(&member.ID, &member.FamilyID, &member.Name, &member.Role, &member.Color, &createdAt); err != nil {
		return Member{}, err
	}
	member.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return member, nil
}

type mcpSessionScanner interface {
	Scan(dest ...any) error
}

func scanMemberMCPSession(scanner mcpSessionScanner) (MemberMCPSession, error) {
	var session MemberMCPSession
	var createdAt, expiresAt string
	var revokedAt sql.NullString
	if err := scanner.Scan(&session.ID, &session.MemberID, &session.Label, &createdAt, &expiresAt, &revokedAt); err != nil {
		return MemberMCPSession{}, err
	}
	var err error
	if session.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return MemberMCPSession{}, err
	}
	if session.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
		return MemberMCPSession{}, err
	}
	if revokedAt.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, revokedAt.String)
		if parseErr != nil {
			return MemberMCPSession{}, parseErr
		}
		session.RevokedAt = &value
	}
	return session, nil
}

var errMCPSessionNotFound = errors.New("MCP session not found")
