package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiProcessUsesGenerateContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-flash-latest:generateContent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("missing Gemini API key header")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Contents []struct {
				Parts []geminiGeneratePart `json:"parts"`
			} `json:"contents"`
			GenerationConfig map[string]any `json:"generationConfig"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Contents) != 1 || len(payload.Contents[0].Parts) != 2 {
			t.Fatalf("unexpected contents: %+v", payload.Contents)
		}
		inline := payload.Contents[0].Parts[1].InlineData
		if inline == nil || inline.MimeType != "audio/wav" || inline.Data != base64.StdEncoding.EncodeToString([]byte("audio")) {
			t.Fatalf("unexpected inline audio: %+v", inline)
		}
		if payload.GenerationConfig["responseMimeType"] != "application/json" || payload.GenerationConfig["responseJsonSchema"] == nil {
			t.Fatalf("missing structured output config: %+v", payload.GenerationConfig)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"transcript\":\"hello\",\"summary\":\"short\"}"}]}}]}`)
	}))
	defer server.Close()

	processor := &geminiAudioProcessor{
		apiKey:  "test-key",
		model:   "gemini-flash-latest",
		client:  &http.Client{Timeout: time.Second},
		baseURL: server.URL,
	}
	result, err := processor.Process(context.Background(), []byte("audio"), "audio/wav")
	if err != nil {
		t.Fatal(err)
	}
	if result.Transcript != "hello" || result.Summary != "short" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGeminiProcessRetriesWhenTranscriptIsEmpty(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Contents []struct {
				Parts []geminiGeneratePart `json:"parts"`
			} `json:"contents"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Contents) != 1 || len(payload.Contents[0].Parts) != 2 {
			t.Fatalf("unexpected contents: %+v", payload.Contents)
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"transcript\":\"\",\"summary\":\"Tea was good.\"}"}]}}]}`)
			return
		}
		if !strings.Contains(payload.Contents[0].Parts[0].Text, "original spoken language") {
			t.Fatalf("retry prompt does not preserve original language: %q", payload.Contents[0].Parts[0].Text)
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"transcript\":\"I had a cup of tea.\"}"}]}}]}`)
	}))
	defer server.Close()

	processor := &geminiAudioProcessor{apiKey: "test-key", model: "gemini-flash-latest", client: &http.Client{Timeout: time.Second}, baseURL: server.URL}
	result, err := processor.Process(context.Background(), []byte("audio"), "audio/wav")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if result.Transcript != "I had a cup of tea." || result.Summary != "Tea was good." {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGeminiProcessUsesTranscriptWhenSummaryIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"transcript\":\"今天喝了一杯茶。\",\"summary\":\"\"}"}]}}]}`)
	}))
	defer server.Close()

	processor := &geminiAudioProcessor{apiKey: "test-key", model: "gemini-flash-latest", client: &http.Client{Timeout: time.Second}, baseURL: server.URL}
	result, err := processor.Process(context.Background(), []byte("audio"), "audio/wav")
	if err != nil {
		t.Fatal(err)
	}
	if result.Transcript != "今天喝了一杯茶。" || result.Summary != result.Transcript {
		t.Fatalf("unexpected transcript fallback: %+v", result)
	}
}

func TestGeminiTTSUsesGenerateContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-3.1-flash-tts-preview:generateContent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			GenerationConfig struct {
				ResponseModalities []string `json:"responseModalities"`
				SpeechConfig       struct {
					VoiceConfig struct {
						PrebuiltVoiceConfig struct {
							VoiceName string `json:"voiceName"`
						} `json:"prebuiltVoiceConfig"`
					} `json:"voiceConfig"`
				} `json:"speechConfig"`
			} `json:"generationConfig"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.GenerationConfig.ResponseModalities) != 1 || payload.GenerationConfig.ResponseModalities[0] != "AUDIO" {
			t.Fatalf("unexpected modalities: %+v", payload.GenerationConfig.ResponseModalities)
		}
		if payload.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName != "Kore" {
			t.Fatalf("unexpected voice config: %+v", payload.GenerationConfig.SpeechConfig)
		}
		encoded := base64.StdEncoding.EncodeToString([]byte{0, 0, 1, 0})
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/l16; rate=24000; channels=1","data":"`+encoded+`"}}]}}]}`)
	}))
	defer server.Close()

	processor := &geminiAudioProcessor{apiKey: "test-key", model: "gemini-flash-latest", client: &http.Client{Timeout: time.Second}, baseURL: server.URL}
	wav, err := processor.SynthesizeSpeech(context.Background(), "hello", "Kore", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV: %d bytes", len(wav))
	}
}

func TestGeminiGenerateContentErrorIncludesProviderReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"status":"RESOURCE_EXHAUSTED","message":"monthly spending cap exceeded"}}`)
	}))
	defer server.Close()
	processor := &geminiAudioProcessor{apiKey: "test-key", model: "gemini-flash-latest", client: &http.Client{Timeout: time.Second}, baseURL: server.URL}
	_, err := processor.generateContent(context.Background(), "gemini test", processor.model, []geminiGeneratePart{{Text: "hello"}}, nil, 1024)
	if err == nil || !strings.Contains(err.Error(), "monthly spending cap exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGeminiDefaultModelUsesWorkingAlias(t *testing.T) {
	t.Setenv("AI_MODE", "gemini")
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_MODEL", "")
	processor, err := newAudioProcessorFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	gemini, ok := processor.(*geminiAudioProcessor)
	if !ok {
		t.Fatalf("unexpected processor type: %T", processor)
	}
	if gemini.model != "gemini-flash-latest" {
		t.Fatalf("unexpected default model: %s", gemini.model)
	}
}
