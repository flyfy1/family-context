package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemberUpdateAndDailySummaryLoop(t *testing.T) {
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
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, mediaDir, "test-token").routes())
	t.Cleanup(server.Close)

	member := requestJSON[Member](t, server.Client(), http.MethodPost, server.URL+"/api/v1/members", map[string]string{
		"familyId": defaultFamilyID, "name": "爸爸", "role": "elder", "color": "#AD4C34",
	}, http.StatusCreated)
	update := requestJSON[Update](t, server.Client(), http.MethodPost, server.URL+"/api/v1/updates", map[string]string{
		"familyId": defaultFamilyID, "memberId": member.ID, "text": "下午和老张下棋，赢了两盘。", "visibility": "family",
	}, http.StatusCreated)

	if _, err := os.Stat(filepath.Join(temp, "spaces", "members", member.ID, "updates", update.ID+".md")); err != nil {
		t.Fatalf("member update file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(temp, "spaces", "shared", "updates", update.ID+".json")); err != nil {
		t.Fatalf("shared update projection missing: %v", err)
	}

	feed := requestJSON[struct {
		Updates []Update `json:"updates"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/updates?scope=family", nil, http.StatusOK)
	if len(feed.Updates) != 1 || feed.Updates[0].Text != update.Text {
		t.Fatalf("unexpected family feed: %+v", feed)
	}

	today := time.Now().UTC().Format("2006-01-02")
	summary := requestJSON[DailySummary](t, server.Client(), http.MethodPost, server.URL+"/api/v1/daily-summaries/generate", map[string]string{
		"familyId": defaultFamilyID, "date": today,
	}, http.StatusCreated)
	if summary.UpdateCount != 1 || summary.Content == "" {
		t.Fatalf("unexpected daily summary: %+v", summary)
	}
	latest := requestJSON[struct {
		Summary *DailySummary `json:"summary"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/daily-summaries/latest", nil, http.StatusOK)
	if latest.Summary == nil || latest.Summary.ID != summary.ID {
		t.Fatalf("unexpected latest summary: %+v", latest)
	}
}

func TestDevelopmentCORSAllowsVite(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, filepath.Join(temp, "media"), "test-token").routes())
	t.Cleanup(server.Close)
	req, _ := http.NewRequest(http.MethodOptions, server.URL+"/api/v1/members", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("unexpected CORS response: status=%d origin=%q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
}
