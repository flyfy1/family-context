package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrivateThoughtJudgmentRequiresManualShare(t *testing.T) {
	t.Parallel()
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
	member := Member{ID: "member-1", FamilyID: defaultFamilyID, Name: "洋宇", Role: "member", Color: "#AD4C34", CreatedAt: time.Now().UTC()}
	if err := store.createMember(context.Background(), member); err != nil {
		t.Fatal(err)
	}
	app := newApp(store, stubAudioProcessor{}, mediaDir, "test-token")
	if err := createMemberSpace(app.spacesRoot, member); err != nil {
		t.Fatal(err)
	}
	judgmentRoutes := http.NewServeMux()
	app.registerJudgmentRoutes(judgmentRoutes)
	judgmentRoutes.Handle("/", app.routes())
	server := httptest.NewServer(judgmentRoutes)
	t.Cleanup(server.Close)

	settings := memberRequestJSON[MemberSettings](t, server.Client(), http.MethodPut, server.URL+"/api/v1/me/share-policy", member.ID, map[string]string{
		"shareMode": "review", "sharePrompt": "家庭生活图片和文字想法可以建议分享；账户、医疗和地址保持私密。",
	}, http.StatusOK)
	if settings.MemberID != member.ID || settings.ShareMode != "review" || settings.SharePrompt == "" {
		t.Fatalf("browser identity did not save its own share policy: %+v", settings)
	}
	loadedSettings := memberRequestJSON[MemberSettings](t, server.Client(), http.MethodGet, server.URL+"/api/v1/me/share-policy", member.ID, nil, http.StatusOK)
	if loadedSettings.MemberID != member.ID || loadedSettings.SharePrompt != settings.SharePrompt {
		t.Fatalf("browser identity loaded the wrong share policy: %+v", loadedSettings)
	}

	prompt := memberRequestJSON[JudgmentPrompt](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/judgment-prompts", member.ID, map[string]string{
		"name": "家庭分享判断", "instruction": "判断这条近况是否适合分享给家人，并忠实整理。",
	}, http.StatusCreated)
	created := memberRequestJSON[struct {
		Update   Update             `json:"update"`
		Judgment JudgmentEvaluation `json:"judgment"`
	}](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/thoughts", member.ID, map[string]string{
		"text": "今天第一次做红烧鱼，味道还不错。", "promptId": prompt.ID,
	}, http.StatusCreated)
	if created.Update.Visibility != "private" || created.Judgment.Decision != "suggest_share" {
		t.Fatalf("unexpected private judgment: %+v", created)
	}

	feed := requestJSON[struct {
		Updates []Update `json:"updates"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/updates?scope=family", nil, http.StatusOK)
	if len(feed.Updates) != 0 {
		t.Fatalf("private thought leaked into family feed: %+v", feed.Updates)
	}

	shared := memberRequestJSON[Update](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/thoughts/"+created.Update.ID+"/share", member.ID, map[string]any{}, http.StatusOK)
	if shared.Visibility != "family" {
		t.Fatalf("shared visibility = %q", shared.Visibility)
	}
	feed = requestJSON[struct {
		Updates []Update `json:"updates"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/updates?scope=family", nil, http.StatusOK)
	if len(feed.Updates) != 1 || feed.Updates[0].ID != created.Update.ID {
		t.Fatalf("shared thought missing from family feed: %+v", feed.Updates)
	}
}

func memberRequestJSON[T any](t *testing.T, client *http.Client, method, url, memberID string, input any, wantStatus int) T {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Admin-Token", "test-token")
	req.Header.Set("X-Member-ID", memberID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		var body any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("%s %s status = %d, want %d, body = %+v", method, url, resp.StatusCode, wantStatus, body)
	}
	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
