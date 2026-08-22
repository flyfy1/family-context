package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestMCPSessionConfigurationAdvertisesCanonicalTools(t *testing.T) {
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, filepath.Join(temp, "media"), "admin-token").routes())
	t.Cleanup(server.Close)

	credential := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": "妈妈", "role": "member", "color": "#AD4C34",
	}, http.StatusCreated)
	configuration := requestAdminJSON[struct {
		Tools []map[string]any `json:"tools"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/admin/members/"+credential.Member.ID+"/mcp-sessions", nil, http.StatusOK)

	expected := memberMCPTools()
	if len(configuration.Tools) != len(expected) {
		t.Fatalf("configuration tools=%d want=%d", len(configuration.Tools), len(expected))
	}
	for index, tool := range expected {
		if configuration.Tools[index]["name"] != tool["name"] {
			t.Fatalf("configuration tool %d=%v want=%v", index, configuration.Tools[index]["name"], tool["name"])
		}
	}
}
