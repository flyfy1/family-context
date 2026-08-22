package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPOAuthAuthorizationCodeAndRefreshFlow(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "")
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
	credential := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": "妈妈", "role": "member", "color": "#AD4C34",
	}, http.StatusCreated)
	resource := server.URL + "/mcp/members/" + credential.Member.ID

	unauthorized := initializeMCPRequest(t, resource, "")
	unauthorizedResponse, err := server.Client().Do(unauthorized)
	if err != nil {
		t.Fatal(err)
	}
	unauthorizedResponse.Body.Close()
	challenge := unauthorizedResponse.Header.Get("WWW-Authenticate")
	if unauthorizedResponse.StatusCode != http.StatusUnauthorized || !strings.Contains(challenge, `resource_metadata="`+server.URL+`/.well-known/oauth-protected-resource/mcp/members/`+credential.Member.ID+`"`) {
		t.Fatalf("unexpected OAuth challenge: status=%d header=%q", unauthorizedResponse.StatusCode, challenge)
	}

	metadata := requestJSONMap(t, server.Client(), http.MethodGet, server.URL+"/.well-known/oauth-authorization-server", nil, http.StatusOK)
	if metadata["authorization_endpoint"] != server.URL+"/oauth/authorize" || metadata["registration_endpoint"] != server.URL+"/oauth/register" {
		t.Fatalf("unexpected authorization metadata: %+v", metadata)
	}
	protected := requestJSONMap(t, server.Client(), http.MethodGet, server.URL+"/.well-known/oauth-protected-resource/mcp/members/"+credential.Member.ID, nil, http.StatusOK)
	if protected["resource"] != resource {
		t.Fatalf("protected resource = %v", protected["resource"])
	}

	registered := requestJSONMap(t, server.Client(), http.MethodPost, server.URL+"/oauth/register", map[string]any{
		"client_name": "ChatGPT", "redirect_uris": []string{"https://chatgpt.example/oauth/callback"},
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	}, http.StatusCreated)
	clientID, _ := registered["client_id"].(string)
	if !strings.HasPrefix(clientID, "fdmcp_client_") {
		t.Fatalf("client id = %q", clientID)
	}

	verifier := strings.Repeat("v", 64)
	sum := sha256.Sum256([]byte(verifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	authorizeValues := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {"https://chatgpt.example/oauth/callback"},
		"code_challenge": {codeChallenge}, "code_challenge_method": {"S256"}, "resource": {resource},
		"scope": {"mcp:context offline_access"}, "state": {"state-123"},
	}
	authorizePage, err := server.Client().Get(server.URL + "/oauth/authorize?" + authorizeValues.Encode())
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(authorizePage.Body)
	authorizePage.Body.Close()
	if authorizePage.StatusCode != http.StatusOK || !strings.Contains(string(page), "ChatGPT") || !strings.Contains(string(page), resource) {
		t.Fatalf("unexpected consent page status=%d body=%s", authorizePage.StatusCode, page)
	}

	bootstrap := requestMemberJSON[MemberMCPSessionCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/mcp-sessions", map[string]string{
		"label": "ChatGPT authorization",
	}, credential.AccessToken, http.StatusCreated)
	authorizeValues.Set("member_access_token", bootstrap.AccessToken)
	noRedirectClient := *server.Client()
	noRedirectClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	authorizeResponse, err := noRedirectClient.PostForm(server.URL+"/oauth/authorize", authorizeValues)
	if err != nil {
		t.Fatal(err)
	}
	authorizeResponse.Body.Close()
	if authorizeResponse.StatusCode != http.StatusFound {
		t.Fatalf("authorization status = %d", authorizeResponse.StatusCode)
	}
	callback, _ := url.Parse(authorizeResponse.Header.Get("Location"))
	code := callback.Query().Get("code")
	if code == "" || callback.Query().Get("state") != "state-123" {
		t.Fatalf("callback = %s", callback)
	}

	tokens := postOAuthToken(t, server.Client(), server.URL, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID},
		"redirect_uri": {"https://chatgpt.example/oauth/callback"}, "code_verifier": {verifier}, "resource": {resource},
	}, http.StatusOK)
	access, _ := tokens["access_token"].(string)
	refresh, _ := tokens["refresh_token"].(string)
	if !strings.HasPrefix(access, "fdmcp_") || !strings.HasPrefix(refresh, "fdmcp_refresh_") || tokens["expires_in"].(float64) != 3600 {
		t.Fatalf("unexpected OAuth tokens: %+v", tokens)
	}
	transportSession := initializeMCP(t, server.Client(), server.URL, credential.Member.ID, access)
	if tools := callMCP(t, server.Client(), server.URL, credential.Member.ID, access, transportSession, 2, "tools/list", map[string]any{}); tools.Error != nil {
		t.Fatalf("OAuth access token failed: %+v", tools.Error)
	}

	reusedCode := postOAuthToken(t, server.Client(), server.URL, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID},
		"redirect_uri": {"https://chatgpt.example/oauth/callback"}, "code_verifier": {verifier}, "resource": {resource},
	}, http.StatusBadRequest)
	if reusedCode["error"] != "invalid_grant" {
		t.Fatalf("reused code result = %+v", reusedCode)
	}

	refreshed := postOAuthToken(t, server.Client(), server.URL, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}, "resource": {resource},
	}, http.StatusOK)
	rotatedRefresh, _ := refreshed["refresh_token"].(string)
	if rotatedRefresh == "" || rotatedRefresh == refresh {
		t.Fatalf("refresh token was not rotated: %+v", refreshed)
	}
	oldRefresh := postOAuthToken(t, server.Client(), server.URL, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}, "resource": {resource},
	}, http.StatusBadRequest)
	if oldRefresh["error"] != "invalid_grant" {
		t.Fatalf("old refresh result = %+v", oldRefresh)
	}

	requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members/"+credential.Member.ID+"/token", map[string]string{}, http.StatusOK)
	revokedByRotation := postOAuthToken(t, server.Client(), server.URL, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {rotatedRefresh}, "client_id": {clientID}, "resource": {resource},
	}, http.StatusBadRequest)
	if revokedByRotation["error"] != "invalid_grant" {
		t.Fatalf("refresh after member rotation = %+v", revokedByRotation)
	}
}

func TestValidProductionPublicBaseURL(t *testing.T) {
	tests := map[string]bool{
		"https://family-api.example": true, "https://family-api.example/": true,
		"http://family-api.example": false, "https://family-api.example/path": false,
		"https://user@family-api.example": false, "": false,
	}
	for value, want := range tests {
		if got := validProductionPublicBaseURL(value); got != want {
			t.Fatalf("validProductionPublicBaseURL(%q) = %v want %v", value, got, want)
		}
	}
}

func initializeMCPRequest(t *testing.T, resource, token string) *http.Request {
	t.Helper()
	requestBody := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	req, _ := http.NewRequest(http.MethodPost, resource, requestBody)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func requestJSONMap(t *testing.T, client *http.Client, method, endpoint string, input any, status int) map[string]any {
	t.Helper()
	var body io.Reader
	if input != nil {
		data, _ := json.Marshal(input)
		body = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(method, endpoint, body)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s status=%d want=%d body=%s", method, endpoint, resp.StatusCode, status, data)
	}
	var output map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		t.Fatal(err)
	}
	return output
}

func postOAuthToken(t *testing.T, client *http.Client, serverURL string, values url.Values, status int) map[string]any {
	t.Helper()
	resp, err := client.PostForm(serverURL+"/oauth/token", values)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("token status=%d want=%d body=%s", resp.StatusCode, status, data)
	}
	var output map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		t.Fatal(err)
	}
	return output
}
