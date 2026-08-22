package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type recordingBedtimeProcessor struct {
	stubAudioProcessor
	seen      []Update
	languages []string
	synthErr  error
}

func (p *recordingBedtimeProcessor) GenerateBedtimeStory(_ context.Context, child Member, _ int, updates []Update, _ []Member, language string) (BedtimeStoryDraft, error) {
	p.seen = append([]Update(nil), updates...)
	p.languages = append(p.languages, "story:"+language)
	sources := make([]string, 0, len(updates))
	for _, update := range updates {
		sources = append(sources, update.ID)
	}
	return BedtimeStoryDraft{Title: child.Name + "的月光晚安", Content: "月光轻轻照进窗户，把家人今天分享的快乐变成了一颗温暖的小星星。晚安。", SourceUpdateIDs: sources}, nil
}

func (p *recordingBedtimeProcessor) SynthesizeSpeech(_ context.Context, _ string, _ string, language string) ([]byte, error) {
	p.languages = append(p.languages, "tts:"+language)
	if p.synthErr != nil {
		return nil, p.synthErr
	}
	return wrapPCMAsWAV([]byte{0, 0, 1, 0, 0, 0, 1, 0}, 24000, 1, 16), nil
}

func TestBedtimeStoryUsesOnlySharedContextAndPersistsAudio(t *testing.T) {
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	processor := &recordingBedtimeProcessor{}
	application := newApp(store, processor, filepath.Join(temp, "media"), "test-token")
	child := Member{ID: "child-1", FamilyID: defaultFamilyID, Name: "瓜瓜", Role: "child", Color: "#54706A", CreatedAt: time.Now().UTC()}
	parent := Member{ID: "parent-1", FamilyID: defaultFamilyID, Name: "爸爸", Role: "member", Color: "#AD4C34", CreatedAt: time.Now().UTC().Add(time.Second)}
	for _, member := range []Member{child, parent} {
		if err := createMemberSpace(application.spacesRoot, member); err != nil {
			t.Fatal(err)
		}
		if err := store.createMember(context.Background(), member); err != nil {
			t.Fatal(err)
		}
	}
	shared := Update{ID: "shared-update", FamilyID: defaultFamilyID, MemberID: parent.ID, Type: "text", Text: "今天一家人在公园看到了晚霞。", Visibility: "family", Source: "test", CreatedAt: time.Now().UTC()}
	private := Update{ID: "private-update", FamilyID: defaultFamilyID, MemberID: parent.ID, Type: "text", Text: "这是不能进入故事的私人内容。", Visibility: "private", Source: "test", CreatedAt: time.Now().UTC()}
	for _, update := range []Update{shared, private} {
		if err := store.createUpdate(context.Background(), update, ""); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(application.routes())
	t.Cleanup(server.Close)

	story := requestJSON[BedtimeStory](t, server.Client(), http.MethodPost, server.URL+"/api/v1/bedtime-stories", map[string]any{
		"familyId": defaultFamilyID, "childId": child.ID, "audienceAge": 6, "days": 7, "language": "zh",
	}, http.StatusCreated)
	if story.Status != "ready" || story.AudioURL == "" || len(story.SourceUpdateIDs) != 1 || story.SourceUpdateIDs[0] != shared.ID {
		t.Fatalf("unexpected bedtime story: %+v", story)
	}
	if len(processor.seen) != 1 || processor.seen[0].ID != shared.ID {
		t.Fatalf("private context reached generator: %+v", processor.seen)
	}
	if story.Language != "zh" || len(processor.languages) != 2 || processor.languages[0] != "story:zh" || processor.languages[1] != "tts:zh" {
		t.Fatalf("language was not propagated: story=%+v calls=%v", story, processor.languages)
	}
	metadataPath := filepath.Join(temp, "spaces", "shared", "stories", story.ID+".json")
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("story metadata missing: %v", err)
	}
	audioReq, _ := http.NewRequest(http.MethodGet, server.URL+story.AudioURL, nil)
	audioReq.Header.Set("X-Admin-Token", "test-token")
	audioResp, err := server.Client().Do(audioReq)
	if err != nil {
		t.Fatal(err)
	}
	audio, _ := io.ReadAll(audioResp.Body)
	audioResp.Body.Close()
	if audioResp.StatusCode != http.StatusOK || len(audio) < 44 || string(audio[:4]) != "RIFF" {
		t.Fatalf("invalid story audio: status=%d bytes=%d", audioResp.StatusCode, len(audio))
	}
	unauthorized, _ := http.Get(server.URL + story.AudioURL)
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized audio status=%d", unauthorized.StatusCode)
	}
}

func TestBedtimeStoryKeepsTextWhenTTSFails(t *testing.T) {
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	processor := &recordingBedtimeProcessor{synthErr: errors.New("tts unavailable")}
	application := newApp(store, processor, filepath.Join(temp, "media"), "test-token")
	child := Member{ID: "child-2", FamilyID: defaultFamilyID, Name: "小雨", Role: "child", Color: "#54706A", CreatedAt: time.Now().UTC()}
	if err := createMemberSpace(application.spacesRoot, child); err != nil {
		t.Fatal(err)
	}
	if err := store.createMember(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	update := Update{ID: "family-update", FamilyID: defaultFamilyID, MemberID: child.ID, Type: "text", Text: "今天搭好了一座积木城堡。", Visibility: "family", Source: "test", CreatedAt: time.Now().UTC()}
	if err := store.createUpdate(context.Background(), update, ""); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(application.routes())
	t.Cleanup(server.Close)
	story := requestJSON[BedtimeStory](t, server.Client(), http.MethodPost, server.URL+"/api/v1/bedtime-stories", map[string]any{"childId": child.ID}, http.StatusCreated)
	if story.Language != "en" {
		t.Fatalf("default language = %q, want en", story.Language)
	}
	if story.Status != "audio_failed" || story.Content == "" || story.AudioURL != "" || story.ErrorMessage == "" {
		t.Fatalf("story text was not retained: %+v", story)
	}
	stored, err := store.getBedtimeStory(context.Background(), story.ID, defaultFamilyID)
	if err != nil || stored.Status != "audio_failed" || stored.Content == "" {
		t.Fatalf("stored failed story = %+v, %v", stored, err)
	}
	processor.synthErr = nil
	retried := requestJSON[BedtimeStory](t, server.Client(), http.MethodPost, server.URL+"/api/v1/bedtime-stories/"+story.ID+"/audio", map[string]any{}, http.StatusOK)
	if retried.Status != "ready" || retried.AudioURL == "" || retried.ErrorMessage != "" {
		t.Fatalf("TTS retry failed: %+v", retried)
	}
}
