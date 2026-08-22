package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type audioProcessor interface {
	Process(ctx context.Context, audio []byte, mimeType string) (AudioResult, error)
}

type dailySummarizer interface {
	Summarize(ctx context.Context, updates []Update, members []Member) (string, error)
}

type sharePolicyEvaluator interface {
	EvaluateShare(ctx context.Context, text, prompt string) (ShareDecision, error)
}

type ShareDecision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

func newAudioProcessorFromEnv() (audioProcessor, error) {
	if envOr("AI_MODE", "gemini") == "stub" {
		return stubAudioProcessor{}, nil
	}
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return nil, errors.New("GEMINI_API_KEY is required when AI_MODE=gemini")
	}
	return &geminiAudioProcessor{
		apiKey: key,
		model:  envOr("GEMINI_MODEL", "gemini-3.7-flash"),
		client: &http.Client{Timeout: 75 * time.Second},
	}, nil
}

type stubAudioProcessor struct{}

func (stubAudioProcessor) Process(_ context.Context, _ []byte, _ string) (AudioResult, error) {
	return AudioResult{
		Transcript: "老张已经去医院检查了，医生说没有什么大问题。我们昨天还一起下棋了。",
		Summary:    "老张已经去医院检查，医生认为问题不大。爸爸昨天还和老张一起下棋。",
	}, nil
}

func (stubAudioProcessor) Summarize(_ context.Context, updates []Update, members []Member) (string, error) {
	return localDailySummary(updates, members), nil
}

func (stubAudioProcessor) EvaluateShare(_ context.Context, _ string, prompt string) (ShareDecision, error) {
	return ShareDecision{Allowed: strings.TrimSpace(prompt) != "", Reason: "stub policy evaluation"}, nil
}

type geminiAudioProcessor struct {
	apiKey string
	model  string
	client *http.Client
}

type geminiInteractionResponse struct {
	Status string `json:"status"`
	Steps  []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"steps"`
}

func (g *geminiAudioProcessor) Process(ctx context.Context, audio []byte, mimeType string) (AudioResult, error) {
	payload := map[string]any{
		"model": g.model,
		"store": false,
		"input": []any{
			map[string]any{"type": "text", "text": `请处理这段家庭语音。输出简体中文 JSON：
1. transcript：尽可能忠实的逐字转写，保留人名、金额、日期和不确定语气。
2. summary：一到两句简短、自然的整理结果。
不要补充录音中没有的事实，不要做健康诊断，不要把猜测改成确定事实。`},
			map[string]any{"type": "audio", "data": base64.StdEncoding.EncodeToString(audio), "mime_type": mimeType},
		},
		"response_format": []any{map[string]any{
			"type":      "text",
			"mime_type": "application/json",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"transcript": map[string]any{"type": "string"},
					"summary":    map[string]any{"type": "string"},
				},
				"required": []string{"transcript", "summary"},
			},
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AudioResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/interactions", bytes.NewReader(body))
	if err != nil {
		return AudioResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return AudioResult{}, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return AudioResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AudioResult{}, fmt.Errorf("gemini returned status %d", resp.StatusCode)
	}

	var interaction geminiInteractionResponse
	if err := json.Unmarshal(responseBody, &interaction); err != nil {
		return AudioResult{}, fmt.Errorf("decode gemini response: %w", err)
	}
	for i := len(interaction.Steps) - 1; i >= 0; i-- {
		if interaction.Steps[i].Type != "model_output" {
			continue
		}
		for _, content := range interaction.Steps[i].Content {
			if content.Type != "text" || content.Text == "" {
				continue
			}
			var result AudioResult
			if err := json.Unmarshal([]byte(content.Text), &result); err != nil {
				return AudioResult{}, fmt.Errorf("decode gemini structured output: %w", err)
			}
			if result.Transcript == "" || result.Summary == "" {
				return AudioResult{}, errors.New("gemini returned empty transcript or summary")
			}
			return result, nil
		}
	}
	return AudioResult{}, fmt.Errorf("gemini interaction ended with status %q and no text output", interaction.Status)
}

func (g *geminiAudioProcessor) Summarize(ctx context.Context, updates []Update, members []Member) (string, error) {
	names := make(map[string]string, len(members))
	for _, member := range members {
		names[member.ID] = member.Name
	}
	items := make([]map[string]string, 0, len(updates))
	for _, update := range updates {
		text := update.Text
		if update.AISummary != "" {
			text = update.AISummary
		}
		items = append(items, map[string]string{"member": names[update.MemberID], "text": text})
	}
	input, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"model": g.model,
		"store": false,
		"input": []any{map[string]any{"type": "text", "text": `根据下面的家庭成员公开动态生成一份简体中文家庭日报。要求：温暖、简短、忠于原文；按成员自然组织；不要添加事实、建议、健康判断或隐私推断。输出 JSON，字段 summary。\n\n` + string(input)}},
		"response_format": []any{map[string]any{
			"type": "text", "mime_type": "application/json",
			"schema": map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string"}}, "required": []string{"summary"}},
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/interactions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gemini returned status %d", resp.StatusCode)
	}
	var interaction geminiInteractionResponse
	if err := json.Unmarshal(responseBody, &interaction); err != nil {
		return "", err
	}
	for i := len(interaction.Steps) - 1; i >= 0; i-- {
		for _, content := range interaction.Steps[i].Content {
			if content.Type != "text" {
				continue
			}
			var result struct {
				Summary string `json:"summary"`
			}
			if err := json.Unmarshal([]byte(content.Text), &result); err == nil && result.Summary != "" {
				return result.Summary, nil
			}
		}
	}
	return "", errors.New("gemini returned no daily summary")
}

func (g *geminiAudioProcessor) EvaluateShare(ctx context.Context, text, prompt string) (ShareDecision, error) {
	payload := map[string]any{
		"model": g.model,
		"store": false,
		"input": []any{map[string]any{"type": "text", "text": `你是家庭内容分享策略执行器。根据成员自己配置的规则，判断候选内容是否允许自动分享给整个家庭。规则没有明确允许时必须拒绝；不要补充事实。输出 JSON。\n\n成员规则：\n` + prompt + `\n\n候选内容：\n` + text}},
		"response_format": []any{map[string]any{
			"type": "text", "mime_type": "application/json",
			"schema": map[string]any{"type": "object", "properties": map[string]any{
				"allowed": map[string]any{"type": "boolean"}, "reason": map[string]any{"type": "string"},
			}, "required": []string{"allowed", "reason"}},
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ShareDecision{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/interactions", bytes.NewReader(body))
	if err != nil {
		return ShareDecision{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return ShareDecision{}, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return ShareDecision{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ShareDecision{}, fmt.Errorf("gemini returned status %d", resp.StatusCode)
	}
	var interaction geminiInteractionResponse
	if err := json.Unmarshal(responseBody, &interaction); err != nil {
		return ShareDecision{}, err
	}
	for i := len(interaction.Steps) - 1; i >= 0; i-- {
		for _, content := range interaction.Steps[i].Content {
			if content.Type != "text" {
				continue
			}
			var decision ShareDecision
			if err := json.Unmarshal([]byte(content.Text), &decision); err == nil && decision.Reason != "" {
				return decision, nil
			}
		}
	}
	return ShareDecision{}, errors.New("gemini returned no share decision")
}

func localDailySummary(updates []Update, members []Member) string {
	names := make(map[string]string, len(members))
	for _, member := range members {
		names[member.ID] = member.Name
	}
	var result string
	for _, update := range updates {
		text := update.Text
		if update.AISummary != "" {
			text = update.AISummary
		}
		if result != "" {
			result += "\n\n"
		}
		result += names[update.MemberID] + "：" + text
	}
	return result
}
