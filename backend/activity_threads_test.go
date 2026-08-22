package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestScheduledActivityCreatesParticipantOnlyThreadWithPosts(t *testing.T) {
	t.Setenv("ADMIN_API_TOKEN", "admin-token")
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, filepath.Join(temp, "media"), "admin-token").routes())
	t.Cleanup(server.Close)

	first := createTestMemberCredential(t, server, "姐姐")
	second := createTestMemberCredential(t, server, "弟弟")
	outsider := createTestMemberCredential(t, server, "叔叔")
	now := time.Now().UTC()
	scheduledFor := now.Add(-time.Minute)
	job, err := store.saveScheduledJob(context.Background(), ScheduledJob{FamilyID: defaultFamilyID, Type: scheduledJobFamilyActivity,
		Title: "旧照片故事", Topic: "每个人分享一张童年照片", ScheduledFor: &scheduledFor, MemberIDs: []string{first.Member.ID, second.Member.ID},
		Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.runCoreJobs(context.Background(), now)
	if err != nil || result.JobsTriggered != 1 || result.NotificationsCreated != 2 {
		t.Fatalf("run=%+v err=%v", result, err)
	}
	if len(job.MemberIDs) != 2 {
		t.Fatalf("members not persisted: %+v", job)
	}

	firstThreads := memberJSON[struct {
		Threads []ActivityThread `json:"threads"`
	}](t, server, first.AccessToken, http.MethodGet, "/api/v1/me/activity-threads", nil, "application/json", http.StatusOK)
	if len(firstThreads.Threads) != 1 || len(firstThreads.Threads[0].MemberIDs) != 2 {
		t.Fatalf("unexpected first threads: %+v", firstThreads.Threads)
	}
	threadID := firstThreads.Threads[0].ID
	outsideThreads := memberJSON[struct {
		Threads []ActivityThread `json:"threads"`
	}](t, server, outsider.AccessToken, http.MethodGet, "/api/v1/me/activity-threads", nil, "application/json", http.StatusOK)
	if len(outsideThreads.Threads) != 0 {
		t.Fatalf("outsider saw threads: %+v", outsideThreads.Threads)
	}

	textPost := memberJSON[ActivityPost](t, server, first.AccessToken, http.MethodPost, "/api/v1/me/activity-threads/"+threadID+"/posts", bytes.NewBufferString(`{"text":"这是我小学时的照片。"}`), "application/json", http.StatusCreated)
	if textPost.Type != "text" || textPost.MemberID != first.Member.ID {
		t.Fatalf("unexpected text post: %+v", textPost)
	}

	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("media", "memory.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	_ = writer.WriteField("text", "童年的夏天")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	mediaPost := memberJSON[ActivityPost](t, server, second.AccessToken, http.MethodPost, "/api/v1/me/activity-threads/"+threadID+"/media", body, writer.FormDataContentType(), http.StatusCreated)
	if mediaPost.Type != "image" || mediaPost.MediaURL == "" {
		t.Fatalf("unexpected media post: %+v", mediaPost)
	}

	secondThreads := memberJSON[struct {
		Threads []ActivityThread `json:"threads"`
	}](t, server, second.AccessToken, http.MethodGet, "/api/v1/me/activity-threads", nil, "application/json", http.StatusOK)
	if len(secondThreads.Threads) != 1 || len(secondThreads.Threads[0].Posts) != 2 {
		t.Fatalf("participant did not see posts: %+v", secondThreads.Threads)
	}
	mediaResponse := memberRequest(t, server, first.AccessToken, http.MethodGet, mediaPost.MediaURL, nil, "")
	defer mediaResponse.Body.Close()
	if mediaResponse.StatusCode != http.StatusOK {
		t.Fatalf("participant media status=%d", mediaResponse.StatusCode)
	}
	outsideMedia := memberRequest(t, server, outsider.AccessToken, http.MethodGet, mediaPost.MediaURL, nil, "")
	outsideMedia.Body.Close()
	if outsideMedia.StatusCode != http.StatusNotFound {
		t.Fatalf("outsider media status=%d", outsideMedia.StatusCode)
	}

	result, err = store.runCoreJobs(context.Background(), now.Add(time.Minute))
	if err != nil || result.JobsTriggered != 0 {
		t.Fatalf("duplicate run=%+v err=%v", result, err)
	}
}

func memberJSON[T any](t *testing.T, server *httptest.Server, token, method, path string, body io.Reader, contentType string, want int) T {
	t.Helper()
	response := memberRequest(t, server, token, method, path, body, contentType)
	defer response.Body.Close()
	if response.StatusCode != want {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s status=%d body=%s", method, path, response.StatusCode, data)
	}
	var value T
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
