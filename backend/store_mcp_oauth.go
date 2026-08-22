package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type MCPOAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
	CreatedAt    time.Time
}

type MCPOAuthCode struct {
	ClientID      string
	MemberID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	ExpiresAt     time.Time
}

type MCPOAuthRefreshGrant struct {
	ClientID  string
	MemberID  string
	Resource  string
	Scope     string
	ExpiresAt time.Time
}

func (s *store) migrateMCPOAuth(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS mcp_oauth_clients (
  client_id TEXT PRIMARY KEY,
  client_name TEXT NOT NULL,
  redirect_uris_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS mcp_oauth_codes (
  code_hash TEXT PRIMARY KEY,
  client_id TEXT NOT NULL REFERENCES mcp_oauth_clients(client_id) ON DELETE CASCADE,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  redirect_uri TEXT NOT NULL,
  code_challenge TEXT NOT NULL,
  resource TEXT NOT NULL,
  scope TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at TEXT
);
CREATE TABLE IF NOT EXISTS mcp_oauth_refresh_tokens (
  token_hash TEXT PRIMARY KEY,
  client_id TEXT NOT NULL REFERENCES mcp_oauth_clients(client_id) ON DELETE CASCADE,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  resource TEXT NOT NULL,
  scope TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_codes_expiry ON mcp_oauth_codes(expires_at);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_refresh_member ON mcp_oauth_refresh_tokens(member_id, expires_at);
`)
	return err
}

func (s *store) createMCPOAuthClient(ctx context.Context, client MCPOAuthClient) error {
	redirects, _ := json.Marshal(client.RedirectURIs)
	_, err := s.db.ExecContext(ctx, `INSERT INTO mcp_oauth_clients(client_id, client_name, redirect_uris_json, created_at) VALUES(?, ?, ?, ?)`,
		client.ID, client.Name, string(redirects), client.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *store) getMCPOAuthClient(ctx context.Context, clientID string) (MCPOAuthClient, error) {
	var client MCPOAuthClient
	var redirects, createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT client_id, client_name, redirect_uris_json, created_at FROM mcp_oauth_clients WHERE client_id = ?`, clientID).
		Scan(&client.ID, &client.Name, &redirects, &createdAt)
	if err != nil {
		return MCPOAuthClient{}, err
	}
	if err := json.Unmarshal([]byte(redirects), &client.RedirectURIs); err != nil {
		return MCPOAuthClient{}, err
	}
	client.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return client, err
}

func (s *store) createMCPOAuthCode(ctx context.Context, codeHash string, code MCPOAuthCode) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO mcp_oauth_codes(code_hash, client_id, member_id, redirect_uri, code_challenge, resource, scope, expires_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, codeHash, code.ClientID, code.MemberID, code.RedirectURI, code.CodeChallenge, code.Resource, code.Scope, code.ExpiresAt.Format(time.RFC3339Nano))
	return err
}

func (s *store) consumeMCPOAuthCode(ctx context.Context, codeHash, clientID, redirectURI, resource string, now time.Time) (MCPOAuthCode, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MCPOAuthCode{}, err
	}
	defer tx.Rollback()
	var code MCPOAuthCode
	var expiresAt string
	err = tx.QueryRowContext(ctx, `SELECT client_id, member_id, redirect_uri, code_challenge, resource, scope, expires_at
FROM mcp_oauth_codes WHERE code_hash = ? AND used_at IS NULL`, codeHash).
		Scan(&code.ClientID, &code.MemberID, &code.RedirectURI, &code.CodeChallenge, &code.Resource, &code.Scope, &expiresAt)
	if err != nil {
		return MCPOAuthCode{}, err
	}
	code.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !code.ExpiresAt.After(now) || code.ClientID != clientID || code.RedirectURI != redirectURI || code.Resource != resource {
		return MCPOAuthCode{}, sql.ErrNoRows
	}
	result, err := tx.ExecContext(ctx, `UPDATE mcp_oauth_codes SET used_at = ? WHERE code_hash = ? AND used_at IS NULL`, now.Format(time.RFC3339Nano), codeHash)
	if err != nil {
		return MCPOAuthCode{}, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return MCPOAuthCode{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return MCPOAuthCode{}, err
	}
	return code, nil
}

func (s *store) createMCPOAuthRefreshToken(ctx context.Context, tokenHash string, grant MCPOAuthRefreshGrant) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO mcp_oauth_refresh_tokens(token_hash, client_id, member_id, resource, scope, expires_at)
VALUES(?, ?, ?, ?, ?, ?)`, tokenHash, grant.ClientID, grant.MemberID, grant.Resource, grant.Scope, grant.ExpiresAt.Format(time.RFC3339Nano))
	return err
}

func (s *store) rotateMCPOAuthRefreshToken(ctx context.Context, oldHash, newHash, clientID, resource string, now time.Time) (MCPOAuthRefreshGrant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MCPOAuthRefreshGrant{}, err
	}
	defer tx.Rollback()
	var grant MCPOAuthRefreshGrant
	var expiresAt string
	err = tx.QueryRowContext(ctx, `SELECT client_id, member_id, resource, scope, expires_at FROM mcp_oauth_refresh_tokens
WHERE token_hash = ? AND revoked_at IS NULL`, oldHash).Scan(&grant.ClientID, &grant.MemberID, &grant.Resource, &grant.Scope, &expiresAt)
	if err != nil {
		return MCPOAuthRefreshGrant{}, err
	}
	grant.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !grant.ExpiresAt.After(now) || grant.ClientID != clientID || grant.Resource != resource {
		return MCPOAuthRefreshGrant{}, sql.ErrNoRows
	}
	result, err := tx.ExecContext(ctx, `UPDATE mcp_oauth_refresh_tokens SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL`, now.Format(time.RFC3339Nano), oldHash)
	if err != nil {
		return MCPOAuthRefreshGrant{}, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return MCPOAuthRefreshGrant{}, sql.ErrNoRows
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mcp_oauth_refresh_tokens(token_hash, client_id, member_id, resource, scope, expires_at)
VALUES(?, ?, ?, ?, ?, ?)`, newHash, grant.ClientID, grant.MemberID, grant.Resource, grant.Scope, grant.ExpiresAt.Format(time.RFC3339Nano))
	if err != nil {
		return MCPOAuthRefreshGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return MCPOAuthRefreshGrant{}, err
	}
	return grant, nil
}

func (s *store) revokeAllMCPOAuthRefreshTokens(ctx context.Context, memberID string, revokedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE mcp_oauth_refresh_tokens SET revoked_at = ?
WHERE member_id = ? AND revoked_at IS NULL`, revokedAt.Format(time.RFC3339Nano), memberID)
	return err
}
