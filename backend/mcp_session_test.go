package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemberMCPSessionIsDurableScopedAndRevocable(t *testing.T) {
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	mediaDir := filepath.Join(temp, "media")
	if err := os.MkdirAll(mediaDir, 0o750); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, mediaDir, "admin-token").routes())
	t.Cleanup(server.Close)

	first := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": "妈妈", "role": "member", "color": "#54706A",
	}, http.StatusCreated)
	second := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": "爸爸", "role": "member", "color": "#AD4C34",
	}, http.StatusCreated)

	created := requestMemberJSON[MemberMCPSessionCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/mcp-sessions", map[string]string{
		"label": "Claude Code on Mac",
	}, first.AccessToken, http.StatusCreated)
	if !strings.HasPrefix(created.AccessToken, "fdmcp_") || created.Session.MemberID != first.Member.ID {
		t.Fatalf("unexpected MCP credential: %+v", created)
	}
	lifetime := created.Session.ExpiresAt.Sub(created.Session.CreatedAt)
	if lifetime != memberMCPSessionLifetime || lifetime < 30*24*time.Hour {
		t.Fatalf("MCP access lifetime = %v", lifetime)
	}
	if created.ServerURL != server.URL+"/mcp/members/"+first.Member.ID {
		t.Fatalf("server URL = %q", created.ServerURL)
	}

	listed := requestMemberJSON[struct {
		Sessions []MemberMCPSession `json:"sessions"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/me/mcp-sessions", nil, first.AccessToken, http.StatusOK)
	if len(listed.Sessions) != 1 || listed.Sessions[0].ID != created.Session.ID {
		t.Fatalf("listed sessions = %+v", listed.Sessions)
	}

	// The durable access token survives a fresh app instance even though the short MCP transport session does not.
	restarted := httptest.NewServer(newApp(store, stubAudioProcessor{}, mediaDir, "admin-token").routes())
	t.Cleanup(restarted.Close)
	transportSession := initializeMCP(t, restarted.Client(), restarted.URL, first.Member.ID, created.AccessToken)
	tools := callMCP(t, restarted.Client(), restarted.URL, first.Member.ID, created.AccessToken, transportSession, 2, "tools/list", map[string]any{})
	if tools.Error != nil {
		t.Fatalf("durable MCP token failed after app restart: %+v", tools.Error)
	}

	crossMember := initializeMCPStatus(t, restarted.Client(), restarted.URL, second.Member.ID, created.AccessToken)
	if crossMember != http.StatusUnauthorized {
		t.Fatalf("cross-member MCP status = %d", crossMember)
	}

	requestMemberNoContent(t, server.Client(), http.MethodDelete,
		server.URL+"/api/v1/me/mcp-sessions/"+created.Session.ID, first.AccessToken)
	if status := initializeMCPStatus(t, restarted.Client(), restarted.URL, first.Member.ID, created.AccessToken); status != http.StatusUnauthorized {
		t.Fatalf("revoked MCP token status = %d", status)
	}
}

func initializeMCPStatus(t *testing.T, client *http.Client, serverURL, memberID, token string) int {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/mcp/members/"+memberID, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func requestMemberNoContent(t *testing.T, client *http.Client, method, url, token string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("%s %s status=%d want=%d", method, url, resp.StatusCode, http.StatusNoContent)
	}
}
