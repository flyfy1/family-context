package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
)

type testMediaProcessor struct {
	stubAudioProcessor
	analysis MediaAnalysis
	err      error
}

func (p testMediaProcessor) AnalyzeMedia(_ context.Context, _ []byte, _ string, _ string) (MediaAnalysis, error) {
	if p.err != nil {
		return MediaAnalysis{}, p.err
	}
	return p.analysis, nil
}

func TestMediaImportReviewIsolationAndShare(t *testing.T) {
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	processor := testMediaProcessor{analysis: MediaAnalysis{Summary: "孩子在公园骑自行车。", SuggestedCaption: "今天在公园学骑车", People: "一位孩子",
		Activities: []string{"骑自行车"}, SuggestedVisibility: "family", Reason: "符合成员规则"}}
	server := httptest.NewServer(newApp(store, processor, filepath.Join(temp, "media"), "admin-token").routes())
	t.Cleanup(server.Close)
	first := createTestMemberCredential(t, server, "爸爸")
	second := createTestMemberCredential(t, server, "妈妈")
	requestMemberJSON[MemberSettings](t, server.Client(), http.MethodPut, server.URL+"/api/v1/me/share-policy", map[string]string{
		"shareMode": "review", "sharePrompt": "可以分享普通家庭活动。",
	}, first.AccessToken, http.StatusOK)

	unauthorized, _ := http.Post(server.URL+"/api/v1/me/media-imports", "multipart/form-data", bytes.NewReader(nil))
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}

	item := uploadTestMediaImport(t, server, first.AccessToken, "ride.png", "image/png", []byte("\x89PNG\r\n\x1a\nprivate image"))
	if item.AnalysisStatus != "ready" || item.Analysis == nil || item.Analysis.SuggestedVisibility != "family" || item.ShareDecision != "pending" || item.UpdateID != "" {
		t.Fatalf("unexpected review import: %+v", item)
	}
	repeated := uploadTestMediaImport(t, server, first.AccessToken, "ride.png", "image/png", []byte("\x89PNG\r\n\x1a\ndifferent retry body"))
	if repeated.ID != item.ID || repeated.SHA256 != item.SHA256 {
		t.Fatalf("clientMediaId retry created a duplicate: %+v", repeated)
	}
	if _, err := os.Stat(filepath.Join(temp, "spaces", "members", first.Member.ID, "media", filepath.Base(item.MediaURL))); err != nil {
		t.Fatalf("private original missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(temp, "spaces", "members", first.Member.ID, "imports", item.ID+".json")); err != nil {
		t.Fatalf("import metadata missing: %v", err)
	}

	cross := memberRequest(t, server, second.AccessToken, http.MethodGet, "/api/v1/me/media-imports/"+item.ID, nil, "application/json")
	cross.Body.Close()
	if cross.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-member import status = %d", cross.StatusCode)
	}
	fileCross := memberRequest(t, server, second.AccessToken, http.MethodGet, item.MediaURL, nil, "")
	fileCross.Body.Close()
	if fileCross.StatusCode != http.StatusUnauthorized {
		t.Fatalf("cross-member file status = %d", fileCross.StatusCode)
	}

	shared := requestMemberJSON[MediaImport](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/media-imports/"+item.ID+"/decision", map[string]string{
		"visibility": "family", "caption": "第一次学会骑自行车。",
	}, first.AccessToken, http.StatusOK)
	if shared.ShareDecision != "family" || shared.UpdateID == "" {
		t.Fatalf("unexpected shared import: %+v", shared)
	}
	again := requestMemberJSON[MediaImport](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/media-imports/"+item.ID+"/decision", map[string]string{
		"visibility": "family", "caption": "不会重复创建",
	}, first.AccessToken, http.StatusOK)
	if again.UpdateID != shared.UpdateID {
		t.Fatalf("decision was not idempotent: %s != %s", again.UpdateID, shared.UpdateID)
	}
	updates := requestMemberJSON[struct {
		Updates []Update `json:"updates"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/me/updates", nil, first.AccessToken, http.StatusOK)
	if len(updates.Updates) != 1 || updates.Updates[0].Type != "image" || updates.Updates[0].MediaURL == "" {
		t.Fatalf("unexpected published update: %+v", updates.Updates)
	}
}

func TestMediaImportAutoShareAndFailureStayLocal(t *testing.T) {
	t.Run("explicit auto policy shares safe video", func(t *testing.T) {
		temp := t.TempDir()
		store, err := openStore(filepath.Join(temp, "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
		processor := testMediaProcessor{analysis: MediaAnalysis{Summary: "一家人在吃晚饭。", SuggestedCaption: "一起吃晚饭", Activities: []string{"晚餐"}, SuggestedVisibility: "family", Reason: "符合规则"}}
		server := httptest.NewServer(newApp(store, processor, filepath.Join(temp, "media"), "admin-token").routes())
		t.Cleanup(server.Close)
		credential := createTestMemberCredential(t, server, "奶奶")
		requestMemberJSON[MemberSettings](t, server.Client(), http.MethodPut, server.URL+"/api/v1/me/share-policy", map[string]string{"shareMode": "auto", "sharePrompt": "普通家庭活动可以自动分享。"}, credential.AccessToken, http.StatusOK)
		item := uploadTestMediaImport(t, server, credential.AccessToken, "dinner.mp4", "video/mp4", []byte("small video"))
		if item.MediaType != "video" || item.ShareDecision != "family" || item.UpdateID == "" {
			t.Fatalf("auto share failed: %+v", item)
		}
	})

	t.Run("analysis failure keeps original private", func(t *testing.T) {
		temp := t.TempDir()
		store, err := openStore(filepath.Join(temp, "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
		server := httptest.NewServer(newApp(store, testMediaProcessor{err: errors.New("provider unavailable")}, filepath.Join(temp, "media"), "admin-token").routes())
		t.Cleanup(server.Close)
		credential := createTestMemberCredential(t, server, "爷爷")
		item := uploadTestMediaImport(t, server, credential.AccessToken, "garden.jpg", "image/jpeg", []byte("\xff\xd8\xffprivate photo"))
		if item.AnalysisStatus != "failed" || item.ShareDecision != "pending" || item.UpdateID != "" || item.AnalysisError == "" {
			t.Fatalf("failed import was not private: %+v", item)
		}
		if _, err := os.Stat(filepath.Join(temp, "spaces", "members", credential.Member.ID, "media", filepath.Base(item.MediaURL))); err != nil {
			t.Fatal(err)
		}
	})
}

func createTestMemberCredential(t *testing.T, server *httptest.Server, name string) MemberCredential {
	t.Helper()
	return requestAdminJSON[MemberCredential](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/members", map[string]string{
		"familyId": defaultFamilyID, "name": name, "role": "member", "color": "#54706A",
	}, http.StatusCreated)
}

func uploadTestMediaImport(t *testing.T, server *httptest.Server, token, filename, mimeType string, data []byte) MediaImport {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("capturedAt", "2026-08-22T12:00:00+08:00")
	_ = writer.WriteField("deviceId", "android-test")
	_ = writer.WriteField("clientMediaId", filename+"-20260822")
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="media"; filename="`+filename+`"`)
	header.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = writer.Close()
	resp := memberRequest(t, server, token, http.MethodPost, "/api/v1/me/media-imports", &body, writer.FormDataContentType())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("media import status = %d", resp.StatusCode)
	}
	var item MediaImport
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	return item
}

func memberRequest(t *testing.T, server *httptest.Server, token, method, path string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
