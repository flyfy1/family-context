package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type bedtimeStoryGenerator interface {
	GenerateBedtimeStory(ctx context.Context, child Member, audienceAge int, updates []Update, members []Member) (BedtimeStoryDraft, error)
}

type speechSynthesizer interface {
	SynthesizeSpeech(ctx context.Context, text, voice string) ([]byte, error)
}

func (stubAudioProcessor) GenerateBedtimeStory(_ context.Context, child Member, _ int, updates []Update, _ []Member) (BedtimeStoryDraft, error) {
	sources := make([]string, 0, len(updates))
	for _, update := range updates {
		sources = append(sources, update.ID)
	}
	return BedtimeStoryDraft{Title: child.Name + "和家里的星光", Content: "夜晚到了，家里今天发生的温暖小事变成了窗边的星光。大家互相惦记，也把快乐带回了家。晚安，明天又会是新的一天。", SourceUpdateIDs: sources}, nil
}

func (stubAudioProcessor) SynthesizeSpeech(_ context.Context, _ string, _ string) ([]byte, error) {
	return wrapPCMAsWAV([]byte{0, 0, 0, 0, 0, 0, 0, 0}, 24000, 1, 16), nil
}

func (g *geminiAudioProcessor) GenerateBedtimeStory(ctx context.Context, child Member, audienceAge int, updates []Update, members []Member) (BedtimeStoryDraft, error) {
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
		items = append(items, map[string]string{"id": update.ID, "member": names[update.MemberID], "date": update.CreatedAt.Format("2006-01-02"), "text": text})
	}
	contextJSON, err := json.Marshal(items)
	if err != nil {
		return BedtimeStoryDraft{}, err
	}
	prompt := fmt.Sprintf(`你是一位谨慎、温柔的家庭睡前故事作者。请根据下面仅包含家庭可见动态的 JSON，为 %s（约 %d 岁）写一篇 2 到 4 分钟的简体中文睡前故事。

要求：
- 只使用输入中明确存在的事实，不添加旅行、对话、健康结果、人物关系或其他事件。
- 可以用月光、星星、小动物等轻柔想象作为叙事连接，但不能把想象说成真实家庭事件。
- 语言温暖、平静、适龄，不制造危险、羞耻、比较或焦虑，不提供医疗建议。
- 不提“动态”“数据”“AI”等产品词。
- 可以省略不适合睡前讲的细节，但不能使用任何私人内容。
- sourceUpdateIds 只列出故事实际采用的输入 id，至少一个，不得编造 id。
- 输出 JSON：title、content、sourceUpdateIds。

家庭可见内容：
%s`, child.Name, audienceAge, string(contextJSON))
	payload := map[string]any{
		"model": g.model, "store": false, "input": []any{map[string]any{"type": "text", "text": prompt}},
		"response_format": []any{map[string]any{"type": "text", "mime_type": "application/json", "schema": map[string]any{
			"type": "object", "properties": map[string]any{
				"title": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
				"sourceUpdateIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			}, "required": []string{"title", "content", "sourceUpdateIds"},
		}}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return BedtimeStoryDraft{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/interactions", bytes.NewReader(body))
	if err != nil {
		return BedtimeStoryDraft{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return BedtimeStoryDraft{}, fmt.Errorf("gemini bedtime story request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return BedtimeStoryDraft{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BedtimeStoryDraft{}, fmt.Errorf("gemini returned status %d", resp.StatusCode)
	}
	var interaction geminiInteractionResponse
	if err := json.Unmarshal(responseBody, &interaction); err != nil {
		return BedtimeStoryDraft{}, err
	}
	for i := len(interaction.Steps) - 1; i >= 0; i-- {
		for _, content := range interaction.Steps[i].Content {
			if content.Type != "text" || content.Text == "" {
				continue
			}
			var draft BedtimeStoryDraft
			if err := json.Unmarshal([]byte(content.Text), &draft); err == nil && strings.TrimSpace(draft.Title) != "" && strings.TrimSpace(draft.Content) != "" && len(draft.SourceUpdateIDs) > 0 {
				return draft, nil
			}
		}
	}
	return BedtimeStoryDraft{}, errors.New("gemini returned no bedtime story")
}

func (g *geminiAudioProcessor) SynthesizeSpeech(ctx context.Context, text, voice string) ([]byte, error) {
	model := envOr("GEMINI_TTS_MODEL", "gemini-3.1-flash-tts-preview")
	if strings.TrimSpace(voice) == "" {
		voice = envOr("GEMINI_TTS_VOICE", "Kore")
	}
	payload := map[string]any{
		"model": model, "store": false,
		"input":             "请用温暖、舒缓、自然的普通话，以睡前讲故事的节奏朗读下面全文。不要增加或删改任何内容。\n\n" + text,
		"response_format":   map[string]any{"type": "audio"},
		"generation_config": map[string]any{"speech_config": []any{map[string]any{"voice": voice}}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/interactions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini TTS request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini TTS returned status %d", resp.StatusCode)
	}
	var interaction struct {
		OutputAudio struct {
			Data     string `json:"data"`
			MimeType string `json:"mime_type"`
		} `json:"output_audio"`
		Steps []struct {
			Content []struct {
				Type     string `json:"type"`
				Data     string `json:"data"`
				MimeType string `json:"mime_type"`
			} `json:"content"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(responseBody, &interaction); err != nil {
		return nil, err
	}
	encoded, mimeType := interaction.OutputAudio.Data, interaction.OutputAudio.MimeType
	if encoded == "" {
		for i := len(interaction.Steps) - 1; i >= 0 && encoded == ""; i-- {
			for _, content := range interaction.Steps[i].Content {
				if content.Type == "audio" && content.Data != "" {
					encoded, mimeType = content.Data, content.MimeType
					break
				}
			}
		}
	}
	if encoded == "" {
		return nil, errors.New("gemini TTS returned no audio")
	}
	audio, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode Gemini TTS audio: %w", err)
	}
	if len(audio) >= 12 && string(audio[:4]) == "RIFF" && string(audio[8:12]) == "WAVE" {
		return audio, nil
	}
	rate := 24000
	if strings.Contains(mimeType, "rate=") {
		var parsed int
		if _, err := fmt.Sscanf(mimeType[strings.Index(mimeType, "rate="):], "rate=%d", &parsed); err == nil && parsed > 0 {
			rate = parsed
		}
	}
	return wrapPCMAsWAV(audio, rate, 1, 16), nil
}

func wrapPCMAsWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	dataSize := uint32(len(pcm))
	byteRate := uint32(sampleRate * channels * bitsPerSample / 8)
	blockAlign := uint16(channels * bitsPerSample / 8)
	var output bytes.Buffer
	output.WriteString("RIFF")
	_ = binary.Write(&output, binary.LittleEndian, uint32(36)+dataSize)
	output.WriteString("WAVEfmt ")
	_ = binary.Write(&output, binary.LittleEndian, uint32(16))
	_ = binary.Write(&output, binary.LittleEndian, uint16(1))
	_ = binary.Write(&output, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&output, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&output, binary.LittleEndian, byteRate)
	_ = binary.Write(&output, binary.LittleEndian, blockAlign)
	_ = binary.Write(&output, binary.LittleEndian, uint16(bitsPerSample))
	output.WriteString("data")
	_ = binary.Write(&output, binary.LittleEndian, dataSize)
	output.Write(pcm)
	return output.Bytes()
}

func bedtimeStoryVoice() string { return envOr("GEMINI_TTS_VOICE", "Kore") }
