package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestMemberUsernamePasswordLoginAndLogout(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, t.TempDir(), "admin-token").routes())
	t.Cleanup(server.Close)

	credential := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]any{
		"familyId": defaultFamilyID, "name": "妈妈", "role": "member", "isAdmin": false, "color": "#54706A",
	}, http.StatusCreated)
	status := requestAdminJSON[MemberLoginStatus](t, server.Client(), http.MethodPut, server.URL+"/api/v1/admin/members/"+credential.Member.ID+"/login", map[string]string{
		"username": " Mama ", "password": "correct horse family battery",
	}, http.StatusOK)
	if !status.HasLogin || status.Username != "mama" {
		t.Fatalf("unexpected login status: %+v", status)
	}

	requestScopedJSON[map[string]any](t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{
		"username": "mama", "password": "wrong password",
	}, "", "", http.StatusUnauthorized)
	login := requestScopedJSON[MemberLoginCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{
		"username": "MAMA", "password": "correct horse family battery",
	}, "", "", http.StatusOK)
	if login.Member.ID != credential.Member.ID || len(login.AccessToken) < 20 || login.ExpiresAt.IsZero() {
		t.Fatalf("unexpected login credential: %+v", login)
	}
	me := requestMemberJSON[Member](t, server.Client(), http.MethodGet, server.URL+"/api/v1/me", nil, login.AccessToken, http.StatusOK)
	if me.ID != credential.Member.ID {
		t.Fatalf("session resolved to wrong member: %+v", me)
	}

	requestMemberJSON[map[string]any](t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/logout", nil, login.AccessToken, http.StatusNoContent)
	requestMemberJSON[map[string]any](t, server.Client(), http.MethodGet, server.URL+"/api/v1/me", nil, login.AccessToken, http.StatusUnauthorized)
}

func TestResettingMemberPasswordRevokesExistingWebSessions(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, t.TempDir(), "admin-token").routes())
	t.Cleanup(server.Close)

	credential := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]any{
		"familyId": defaultFamilyID, "name": "爸爸", "role": "member", "isAdmin": true, "color": "#AD4C34",
	}, http.StatusCreated)
	setURL := server.URL + "/api/v1/admin/members/" + credential.Member.ID + "/login"
	requestAdminJSON[MemberLoginStatus](t, server.Client(), http.MethodPut, setURL, map[string]string{"username": "baba", "password": "first secure family password"}, http.StatusOK)
	login := requestScopedJSON[MemberLoginCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{"username": "baba", "password": "first secure family password"}, "", "", http.StatusOK)

	requestAdminJSON[MemberLoginStatus](t, server.Client(), http.MethodPut, setURL, map[string]string{"username": "baba", "password": "second secure family password"}, http.StatusOK)
	requestMemberJSON[map[string]any](t, server.Client(), http.MethodGet, server.URL+"/api/v1/me", nil, login.AccessToken, http.StatusUnauthorized)
	requestScopedJSON[map[string]any](t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{"username": "baba", "password": "first secure family password"}, "", "", http.StatusUnauthorized)
	requestScopedJSON[MemberLoginCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{"username": "baba", "password": "second secure family password"}, "", "", http.StatusOK)
}
