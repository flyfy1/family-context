package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemberMCPReadsOnlyFamilyVisibleUpdatesFromFamilySpace(t *testing.T) {
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, filepath.Join(temp, "media"), "admin-token").routes())
	t.Cleanup(server.Close)

	mom := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": "妈妈", "role": "member", "color": "#AD4C34",
	}, http.StatusCreated)
	dad := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": "爸爸", "role": "member", "color": "#54706A",
	}, http.StatusCreated)
	otherFamily := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": "another-family", "name": "外部成员", "role": "member", "color": "#715A75",
	}, http.StatusCreated)

	now := time.Now().UTC()
	updates := []Update{
		{ID: "mom-shared", FamilyID: defaultFamilyID, MemberID: mom.Member.ID, Type: "text", Text: "妈妈分享的家庭晚餐", Visibility: "family", Source: "test", CreatedAt: now.Add(-4 * time.Minute)},
		{ID: "mom-private", FamilyID: defaultFamilyID, MemberID: mom.Member.ID, Type: "text", Text: "妈妈自己的私密记录", Visibility: "private", Source: "test", CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "dad-shared", FamilyID: defaultFamilyID, MemberID: dad.Member.ID, Type: "text", Text: "爸爸分享的公园散步", Visibility: "family", Source: "test", CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "dad-private", FamilyID: defaultFamilyID, MemberID: dad.Member.ID, Type: "text", Text: "爸爸不能外传的私密记录", Visibility: "private", Source: "test", CreatedAt: now.Add(-time.Minute)},
		{ID: "other-family-shared", FamilyID: otherFamily.Member.FamilyID, MemberID: otherFamily.Member.ID, Type: "text", Text: "另一个家庭的共享内容", Visibility: "family", Source: "test", CreatedAt: now},
	}
	for _, update := range updates {
		if err := store.createUpdate(context.Background(), update, ""); err != nil {
			t.Fatal(err)
		}
	}

	credential := requestMemberJSON[MemberMCPSessionCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/mcp-sessions", map[string]string{
		"label": "External family context",
	}, mom.AccessToken, http.StatusCreated)
	sessionID := initializeMCP(t, server.Client(), server.URL, mom.Member.ID, credential.AccessToken)
	tools := callMCP(t, server.Client(), server.URL, mom.Member.ID, credential.AccessToken, sessionID, 2, "tools/list", map[string]any{})
	if tools.Error != nil || !strings.Contains(string(mustJSON(tools.Result)), `"list_family_updates"`) {
		t.Fatalf("list_family_updates was not advertised: %+v", tools)
	}
	read := callMCP(t, server.Client(), server.URL, mom.Member.ID, credential.AccessToken, sessionID, 3, "tools/call", map[string]any{
		"name": "list_family_updates", "arguments": map[string]any{"since": now.Add(-time.Hour).Format(time.RFC3339), "limit": 10},
	})
	data := string(mustJSON(read.Result))
	if read.Error != nil || !strings.Contains(data, "妈妈分享的家庭晚餐") || !strings.Contains(data, "爸爸分享的公园散步") || !strings.Contains(data, `"name":"妈妈"`) || !strings.Contains(data, `"name":"爸爸"`) {
		t.Fatalf("family-visible updates or authors missing: %s", data)
	}
	for _, forbidden := range []string{"妈妈自己的私密记录", "爸爸不能外传的私密记录", "另一个家庭的共享内容"} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("MCP leaked forbidden content %q: %s", forbidden, data)
		}
	}

	invalid := callMCP(t, server.Client(), server.URL, mom.Member.ID, credential.AccessToken, sessionID, 4, "tools/call", map[string]any{
		"name": "list_family_updates", "arguments": map[string]any{"limit": 101},
	})
	if invalid.Error != nil || !strings.Contains(string(mustJSON(invalid.Result)), `"isError":true`) {
		t.Fatalf("invalid family update filter was accepted: %+v", invalid)
	}
}
