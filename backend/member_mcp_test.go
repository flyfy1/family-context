package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminMemberImagePolicyAndMCPLoop(t *testing.T) {
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

	unauthorized, _ := http.Get(server.URL + "/api/v1/admin/members")
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("admin route without token status = %d", unauthorized.StatusCode)
	}

	credential := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": "瓜瓜", "role": "child", "color": "#54706A",
	}, http.StatusCreated)
	if credential.Member.Role != "child" {
		t.Fatalf("admin child role was not preserved: %+v", credential.Member)
	}
	if !strings.HasPrefix(credential.AccessToken, "fdm_") {
		t.Fatalf("unexpected member access token")
	}
	me := requestMemberJSON[Member](t, server.Client(), http.MethodGet, server.URL+"/api/v1/me", nil, credential.AccessToken, http.StatusOK)
	if me.ID != credential.Member.ID {
		t.Fatalf("member token resolved to %+v", me)
	}

	settings := requestMemberJSON[MemberSettings](t, server.Client(), http.MethodPut, server.URL+"/api/v1/me/share-policy", map[string]string{
		"shareMode": "auto", "sharePrompt": "只有明确描述家庭活动且不包含隐私时，才允许分享。",
	}, credential.AccessToken, http.StatusOK)
	if settings.ShareMode != "auto" || settings.SharePrompt == "" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
	if _, err := os.Stat(filepath.Join(temp, "spaces", "members", me.ID, "share-policy.json")); err != nil {
		t.Fatalf("share policy file missing: %v", err)
	}

	image := uploadMemberImage(t, server.Client(), server.URL, credential.AccessToken)
	if image.Type != "image" || image.MediaURL == "" {
		t.Fatalf("unexpected image update: %+v", image)
	}
	imageReq, _ := http.NewRequest(http.MethodGet, server.URL+image.MediaURL, nil)
	imageReq.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	imageResp, err := server.Client().Do(imageReq)
	if err != nil {
		t.Fatal(err)
	}
	imageResp.Body.Close()
	if imageResp.StatusCode != http.StatusOK {
		t.Fatalf("member image status = %d", imageResp.StatusCode)
	}

	sessionID := initializeMCP(t, server.Client(), server.URL, me.ID, credential.AccessToken)
	tools := callMCP(t, server.Client(), server.URL, me.ID, credential.AccessToken, sessionID, 2, "tools/list", map[string]any{})
	if tools.Error != nil {
		t.Fatalf("tools/list failed: %+v", tools.Error)
	}
	write := callMCP(t, server.Client(), server.URL, me.ID, credential.AccessToken, sessionID, 3, "tools/call", map[string]any{
		"name": "write_context_file", "arguments": map[string]any{"name": "preferences.md", "content": "喜欢看家人的照片。"},
	})
	if write.Error != nil {
		t.Fatalf("write_context_file failed: %+v", write.Error)
	}
	if data, err := os.ReadFile(filepath.Join(temp, "spaces", "members", me.ID, "context", "preferences.md")); err != nil || string(data) != "喜欢看家人的照片。" {
		t.Fatalf("unexpected MCP file: %q, %v", data, err)
	}
	created := callMCP(t, server.Client(), server.URL, me.ID, credential.AccessToken, sessionID, 4, "tools/call", map[string]any{
		"name": "create_update", "arguments": map[string]any{"text": "今天一起吃了晚饭。", "visibility": "family"},
	})
	if created.Error != nil || strings.Contains(string(mustJSON(created.Result)), `"isError":true`) {
		t.Fatalf("MCP create_update failed: %+v", created)
	}

	badOriginReq, _ := http.NewRequest(http.MethodPost, server.URL+"/mcp/members/"+me.ID, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	badOriginReq.Header.Set("Content-Type", "application/json")
	badOriginReq.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	badOriginReq.Header.Set("Origin", "https://attacker.example")
	badOriginResp, err := server.Client().Do(badOriginReq)
	if err != nil {
		t.Fatal(err)
	}
	badOriginResp.Body.Close()
	if badOriginResp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad MCP origin status = %d", badOriginResp.StatusCode)
	}

	rotated := requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members/"+me.ID+"/token", map[string]string{}, http.StatusOK)
	if rotated.AccessToken == credential.AccessToken {
		t.Fatal("rotated token did not change")
	}
	oldTokenReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/me", nil)
	oldTokenReq.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	oldTokenResp, err := server.Client().Do(oldTokenReq)
	if err != nil {
		t.Fatal(err)
	}
	oldTokenResp.Body.Close()
	if oldTokenResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token status after rotation = %d", oldTokenResp.StatusCode)
	}
}

func uploadMemberImage(t *testing.T, client *http.Client, serverURL, token string) Update {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("visibility", "family")
	_ = writer.WriteField("text", "公园里的花开了。")
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="image"; filename="park.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("\x89PNG\r\n\x1a\nminimal"))
	_ = writer.Close()
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/me/updates/image", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("image upload status = %d", resp.StatusCode)
	}
	var update Update
	if err := json.NewDecoder(resp.Body).Decode(&update); err != nil {
		t.Fatal(err)
	}
	return update
}

func initializeMCP(t *testing.T, client *http.Client, serverURL, memberID, token string) string {
	t.Helper()
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": mcpProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "test", "version": "1"},
	}}
	data, _ := json.Marshal(request)
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/mcp/members/"+memberID, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Mcp-Session-Id") == "" {
		t.Fatalf("MCP initialize status=%d session=%q", resp.StatusCode, resp.Header.Get("Mcp-Session-Id"))
	}
	return resp.Header.Get("Mcp-Session-Id")
}

func callMCP(t *testing.T, client *http.Client, serverURL, memberID, token, sessionID string, id int, method string, params any) mcpResponse {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/mcp/members/"+memberID, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var output mcpResponse
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		t.Fatal(err)
	}
	return output
}

func requestAdminJSON[T any](t *testing.T, client *http.Client, method, url string, input any, status int) T {
	t.Helper()
	return requestScopedJSON[T](t, client, method, url, input, "X-Admin-Token", "admin-token", status)
}

func requestMemberJSON[T any](t *testing.T, client *http.Client, method, url string, input any, token string, status int) T {
	t.Helper()
	return requestScopedJSON[T](t, client, method, url, input, "Authorization", "Bearer "+token, status)
}

func requestScopedJSON[T any](t *testing.T, client *http.Client, method, url string, input any, header, value string, status int) T {
	t.Helper()
	data, _ := json.Marshal(input)
	req, _ := http.NewRequest(method, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(header, value)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		t.Fatalf("%s %s status=%d want=%d", method, url, resp.StatusCode, status)
	}
	var output T
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		t.Fatal(err)
	}
	return output
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}
