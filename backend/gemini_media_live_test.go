package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"
)

func TestGeminiMediaAnalysisLive(t *testing.T) {
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
	analyzer, ok := processor.(mediaAnalyzer)
	if !ok {
		t.Fatal("configured AI processor does not support media analysis")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 96, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 96; x++ {
			if x < 48 {
				canvas.Set(x, y, color.RGBA{R: 92, G: 153, B: 112, A: 255})
			} else {
				canvas.Set(x, y, color.RGBA{R: 242, G: 188, B: 92, A: 255})
			}
		}
	}
	var media bytes.Buffer
	if err := png.Encode(&media, canvas); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	analysis, err := analyzer.AnalyzeMedia(ctx, media.Bytes(), "image/png", "普通、无敏感信息的家庭图片可以分享给爷爷。", []Member{{ID: "grandpa", Name: "爷爷", Role: "member"}})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Summary == "" || analysis.SuggestedCaption == "" || !validVisibility(analysis.SuggestedVisibility) || analysis.RecipientReason == "" || analysis.Reason == "" {
		t.Fatalf("incomplete analysis: %+v", analysis)
	}
}
