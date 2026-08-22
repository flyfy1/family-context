package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGeminiBedtimeStoryAndTTSLive(t *testing.T) {
	if os.Getenv("RUN_GEMINI_LIVE") != "1" {
		t.Skip("set RUN_GEMINI_LIVE=1 to call Gemini")
	}
	if err := loadDotEnv("../.env", ".env"); err != nil {
		t.Fatal(err)
	}
	processor, err := newAudioProcessorFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	storyGenerator, ok := processor.(bedtimeStoryGenerator)
	if !ok {
		t.Fatal("configured AI processor does not support bedtime stories")
	}
	tts, ok := processor.(speechSynthesizer)
	if !ok {
		t.Fatal("configured AI processor does not support TTS")
	}
	now := time.Now().UTC()
	child := Member{ID: "child-live", FamilyID: defaultFamilyID, Name: "小星", Role: "child", CreatedAt: now}
	parent := Member{ID: "parent-live", FamilyID: defaultFamilyID, Name: "妈妈", Role: "member", CreatedAt: now}
	updates := []Update{{ID: "update-live", FamilyID: defaultFamilyID, MemberID: parent.ID, Type: "text", Text: "今天妈妈和小星一起在阳台给薄荷浇水，发现长出了一片新叶子。", Visibility: "family", CreatedAt: now}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	draft, err := storyGenerator.GenerateBedtimeStory(ctx, child, 6, updates, []Member{child, parent}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Title == "" || draft.Content == "" || len(draft.SourceUpdateIDs) != 1 || draft.SourceUpdateIDs[0] != updates[0].ID {
		t.Fatalf("incomplete live story: %+v", draft)
	}
	wav, err := tts.SynthesizeSpeech(ctx, draft.Content, bedtimeStoryVoice(), "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("invalid live TTS WAV: %d bytes", len(wav))
	}
}
