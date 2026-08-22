package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGeminiCoreGenerateContentLive(t *testing.T) {
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
	gemini, ok := processor.(*geminiAudioProcessor)
	if !ok {
		t.Fatalf("configured AI processor is %T, want Gemini", processor)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("audio process", func(t *testing.T) {
		if _, err := exec.LookPath("say"); err != nil {
			t.Skip("macOS say command is required for the live speech fixture")
		}
		audioPath := filepath.Join(t.TempDir(), "family-daily-live-test.aiff")
		if output, err := exec.Command("say", "-v", "Tingting", "-o", audioPath, "今天喝了一杯茶，天气很好。测试结束。").CombinedOutput(); err != nil {
			t.Fatalf("create live speech fixture: %v: %s", err, output)
		}
		audio, err := os.ReadFile(audioPath)
		if err != nil {
			t.Fatal(err)
		}
		result, err := gemini.Process(ctx, audio, "audio/aiff")
		if err != nil {
			t.Fatal(err)
		}
		if result.Transcript == "" || result.Summary == "" {
			t.Fatalf("incomplete audio result: %+v", result)
		}
	})

	now := time.Now().UTC()
	member := Member{ID: "member-live", FamilyID: defaultFamilyID, Name: "测试成员", Role: "member", CreatedAt: now}
	update := Update{ID: "update-live", FamilyID: defaultFamilyID, MemberID: member.ID, Type: "text", Text: "今天喝了一杯茶。", Visibility: "family", CreatedAt: now}

	t.Run("daily summary", func(t *testing.T) {
		summary, err := gemini.Summarize(ctx, []Update{update}, []Member{member}, "zh")
		if err != nil {
			t.Fatal(err)
		}
		if summary == "" {
			t.Fatal("empty daily summary")
		}
	})

	t.Run("share evaluation", func(t *testing.T) {
		decision, err := gemini.EvaluateShare(ctx, "测试内容", "普通测试内容可以分享")
		if err != nil {
			t.Fatal(err)
		}
		if decision.Reason == "" {
			t.Fatalf("incomplete share decision: %+v", decision)
		}
	})

	t.Run("judgment", func(t *testing.T) {
		result, err := gemini.Judge(ctx, "今天喝了一杯茶。", "整理普通生活记录")
		if err != nil {
			t.Fatal(err)
		}
		if !validJudgmentDecision(result.Decision) || result.OrganizedText == "" || result.Reason == "" {
			t.Fatalf("incomplete judgment: %+v", result)
		}
	})
}
