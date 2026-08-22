package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const geminiGenerateBaseURL = "https://generativelanguage.googleapis.com"

type geminiGeneratePart struct {
	Text       string                    `json:"text,omitempty"`
	InlineData *geminiGenerateInlineData `json:"inlineData,omitempty"`
}

type geminiGenerateInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiGeneratePart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func geminiJSONGenerationConfig(schema map[string]any) map[string]any {
	return map[string]any{
		"responseMimeType":   "application/json",
		"responseJsonSchema": schema,
	}
}

func (g *geminiAudioProcessor) generateContent(ctx context.Context, operation, model string, parts []geminiGeneratePart, generationConfig map[string]any, responseLimit int64) (geminiGenerateResponse, error) {
	payload := map[string]any{
		"contents": []any{map[string]any{
			"role":  "user",
			"parts": parts,
		}},
	}
	if generationConfig != nil {
		payload["generationConfig"] = generationConfig
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return geminiGenerateResponse{}, err
	}
	baseURL := strings.TrimRight(g.baseURL, "/")
	if baseURL == "" {
		baseURL = geminiGenerateBaseURL
	}
	endpoint := baseURL + "/v1beta/models/" + url.PathEscape(model) + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return geminiGenerateResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return geminiGenerateResponse{}, fmt.Errorf("%s request: %w", operation, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	if err != nil {
		return geminiGenerateResponse{}, err
	}
	var result geminiGenerateResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return geminiGenerateResponse{}, fmt.Errorf("decode %s response: %w", operation, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			message = strings.TrimSpace(result.Error.Status)
		}
		if message == "" {
			return geminiGenerateResponse{}, fmt.Errorf("%s returned status %d", operation, resp.StatusCode)
		}
		return geminiGenerateResponse{}, fmt.Errorf("%s returned status %d: %s", operation, resp.StatusCode, message)
	}
	return result, nil
}

func geminiResponseText(response geminiGenerateResponse) string {
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		}
	}
	return ""
}

func (g *geminiAudioProcessor) generateJSON(ctx context.Context, operation, model string, parts []geminiGeneratePart, schema map[string]any, target any, responseLimit int64) error {
	response, err := g.generateContent(ctx, operation, model, parts, geminiJSONGenerationConfig(schema), responseLimit)
	if err != nil {
		return err
	}
	text := geminiResponseText(response)
	if text == "" {
		return fmt.Errorf("%s returned no text", operation)
	}
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("decode %s structured output: %w", operation, err)
	}
	return nil
}
