package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQuestionAnswerPublishReplyLoop(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	mediaDir := filepath.Join(temp, "media")
	if err := ensureDir(mediaDir); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, mediaDir, "test-token").routes())
	t.Cleanup(server.Close)

	question := requestJSON[Question](t, server.Client(), http.MethodPost, server.URL+"/api/v1/questions", map[string]string{
		"familyId": "family-1", "askedBy": "洋宇", "askedTo": "爸爸", "text": "老张后来去医院了吗？",
	}, http.StatusCreated)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("answeredBy", "爸爸")
	part, err := writer.CreateFormFile("audio", "answer.m4a")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("fake audio bytes"))
	_ = writer.Close()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/questions/"+question.ID+"/answer", &body)
	req.Header.Set("X-Admin-Token", "test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("answer status = %d", resp.StatusCode)
	}
	var answer Answer
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if answer.Transcript == "" || answer.AISummary == "" || answer.Status != "ready" {
		t.Fatalf("unexpected answer: %+v", answer)
	}

	requestJSON[Answer](t, server.Client(), http.MethodPost, server.URL+"/api/v1/answers/"+answer.ID+"/publish", map[string]string{}, http.StatusOK)
	requestJSON[Reply](t, server.Client(), http.MethodPost, server.URL+"/api/v1/answers/"+answer.ID+"/replies", map[string]string{
		"authorId": "洋宇", "text": "那就好，你们昨天谁赢了？",
	}, http.StatusCreated)

	feed := requestJSON[struct {
		Questions []Question `json:"questions"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/questions", nil, http.StatusOK)
	if len(feed.Questions) != 1 || feed.Questions[0].Answer == nil || len(feed.Questions[0].Replies) != 1 {
		t.Fatalf("unexpected feed: %+v", feed)
	}
}

func TestRequiresMemberLoginOrAdminToken(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, temp, "secret").routes())
	t.Cleanup(server.Close)
	resp, err := server.Client().Get(server.URL + "/api/v1/questions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	legacyReq, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/questions", nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyReq.Header.Set("X-Family-Token", "secret")
	legacyResp, err := server.Client().Do(legacyReq)
	if err != nil {
		t.Fatal(err)
	}
	defer legacyResp.Body.Close()
	if legacyResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("legacy family token status = %d, want %d", legacyResp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRootIdentifiesAPIWithoutServingWebApp(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, temp, "secret").routes())
	t.Cleanup(server.Close)

	resp, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("API root status = %d, body = %q", resp.StatusCode, body)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("API root content type = %q", contentType)
	}
	if strings.Contains(strings.ToLower(string(body)), "<!doctype html>") || !strings.Contains(string(body), `"service":"family-daily-api"`) {
		t.Fatalf("API root served unexpected body = %q", body)
	}
}

func TestDraftCanBeDeletedAndRecordedAgain(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	mediaDir := filepath.Join(temp, "media")
	if err := ensureDir(mediaDir); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, mediaDir, "test-token").routes())
	t.Cleanup(server.Close)

	question := requestJSON[Question](t, server.Client(), http.MethodPost, server.URL+"/api/v1/questions", map[string]string{
		"familyId": "family-1", "askedBy": "洋宇", "askedTo": "爸爸", "text": "今天过得怎么样？",
	}, http.StatusCreated)
	answer := uploadTestAnswer(t, server.Client(), server.URL, question.ID)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/answers/"+answer.ID+"/archive", strings.NewReader("{}"))
	req.Header.Set("X-Admin-Token", "test-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	history := requestJSON[AnswerHistory](t, server.Client(), http.MethodGet, server.URL+"/api/v1/questions/"+question.ID+"/history", nil, http.StatusOK)
	if history.Current != nil || len(history.Archived) != 1 || history.Archived[0].ID != answer.ID {
		t.Fatalf("unexpected archived history: %+v", history)
	}
	if len(history.Events) < 3 {
		t.Fatalf("expected an append-only audit trail, got %+v", history.Events)
	}
	archivedAudio := filepath.Join(mediaDir, filepath.Base(strings.TrimPrefix(history.Archived[0].AudioURL, "/media/")))
	if _, err := os.Stat(archivedAudio); err != nil {
		t.Fatalf("archived audio was not retained locally: %v", err)
	}
	second := uploadTestAnswer(t, server.Client(), server.URL, question.ID)
	if second.ID == answer.ID {
		t.Fatal("expected a new answer id")
	}
}

type persistenceCheckingProcessor struct {
	store      *store
	questionID string
	checked    bool
}

func (p *persistenceCheckingProcessor) Process(ctx context.Context, _ []byte, _ string) (AudioResult, error) {
	answer, err := p.store.answerForQuestion(ctx, p.questionID)
	if err != nil {
		return AudioResult{}, err
	}
	if answer.Status != "processing" {
		return AudioResult{}, fmt.Errorf("answer status before AI call = %s", answer.Status)
	}
	p.checked = true
	return AudioResult{Transcript: "本地优先", Summary: "录音先保存在本地。"}, nil
}

func TestAudioIsPersistedBeforeAIProcessing(t *testing.T) {
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	mediaDir := filepath.Join(temp, "media")
	if err := ensureDir(mediaDir); err != nil {
		t.Fatal(err)
	}
	question := Question{ID: "question-local-first", FamilyID: "family-1", AskedBy: "洋宇", AskedTo: "爸爸", Text: "今天怎么样？", Status: "pending", CreatedAt: time.Now().UTC(), Replies: []Reply{}}
	if err := store.createQuestion(context.Background(), question); err != nil {
		t.Fatal(err)
	}
	processor := &persistenceCheckingProcessor{store: store, questionID: question.ID}
	server := httptest.NewServer(newApp(store, processor, mediaDir, "test-token").routes())
	t.Cleanup(server.Close)
	uploadTestAnswer(t, server.Client(), server.URL, question.ID)
	if !processor.checked {
		t.Fatal("AI processor did not observe the locally persisted answer")
	}
}

func uploadTestAnswer(t *testing.T, client *http.Client, serverURL, questionID string) Answer {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("answeredBy", "爸爸")
	part, err := writer.CreateFormFile("audio", "answer.m4a")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("fake audio bytes"))
	_ = writer.Close()
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/questions/"+questionID+"/answer", &body)
	req.Header.Set("X-Admin-Token", "test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("answer status = %d", resp.StatusCode)
	}
	var answer Answer
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	return answer
}

func requestJSON[T any](t *testing.T, client *http.Client, method, url string, input any, wantStatus int) T {
	t.Helper()
	var body *strings.Reader
	if input == nil {
		body = strings.NewReader("")
	} else {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Token", "test-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d", method, url, resp.StatusCode, wantStatus)
	}
	var output T
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		t.Fatal(err)
	}
	return output
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o750)
}
