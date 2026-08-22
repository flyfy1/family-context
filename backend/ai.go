package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

type audioProcessor interface {
	Process(ctx context.Context, audio []byte, mimeType string) (AudioResult, error)
}

type dailySummarizer interface {
	Summarize(ctx context.Context, updates []Update, members []Member, language string) (string, error)
}

type sharePolicyEvaluator interface {
	EvaluateShare(ctx context.Context, text, prompt string) (ShareDecision, error)
}

type mediaAnalyzer interface {
	AnalyzeMedia(ctx context.Context, media []byte, mimeType, sharePrompt string, recipientCandidates []Member) (MediaAnalysis, error)
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
		model:  envOr("GEMINI_MODEL", "gemini-flash-latest"),
		client: &http.Client{Timeout: 150 * time.Second},
	}, nil
}

type stubAudioProcessor struct{}

func (stubAudioProcessor) Process(_ context.Context, _ []byte, _ string) (AudioResult, error) {
	return AudioResult{
		Transcript: "老张已经去医院检查了，医生说没有什么大问题。我们昨天还一起下棋了。",
		Summary:    "老张已经去医院检查，医生认为问题不大。爸爸昨天还和老张一起下棋。",
	}, nil
}

func (stubAudioProcessor) Summarize(_ context.Context, updates []Update, members []Member, language string) (string, error) {
	return localDailySummary(updates, members, language), nil
}

func (stubAudioProcessor) EvaluateShare(_ context.Context, _ string, prompt string) (ShareDecision, error) {
	return ShareDecision{Allowed: strings.TrimSpace(prompt) != "", Reason: "stub policy evaluation"}, nil
}

func (stubAudioProcessor) AnalyzeMedia(_ context.Context, _ []byte, mimeType, sharePrompt string, recipientCandidates []Member) (MediaAnalysis, error) {
	mediaType := "照片"
	if strings.HasPrefix(mimeType, "video/") {
		mediaType = "视频"
	}
	visibility := "private"
	if strings.TrimSpace(sharePrompt) != "" {
		visibility = "family"
	}
	recipients := make([]MediaShareRecipient, 0, len(recipientCandidates))
	if visibility == "family" {
		for _, member := range recipientCandidates {
			recipients = append(recipients, MediaShareRecipient{MemberID: member.ID, Name: member.Name})
		}
	}
	return MediaAnalysis{Summary: "记录了一段家庭生活" + mediaType + "。", SuggestedCaption: "今天的生活片段", Activities: []string{"家庭生活"},
		SuggestedVisibility: visibility, SuggestedRecipients: recipients, RecipientReason: "stub recipient suggestion", Reason: "stub media analysis"}, nil
}

func (g *geminiAudioProcessor) AnalyzeMedia(ctx context.Context, media []byte, mimeType, sharePrompt string, recipientCandidates []Member) (MediaAnalysis, error) {
	candidates := make([]map[string]string, 0, len(recipientCandidates))
	for _, member := range recipientCandidates {
		candidates = append(candidates, map[string]string{"memberId": member.ID, "name": member.Name, "role": member.Role})
	}
	candidatesJSON, err := json.Marshal(candidates)
	if err != nil {
		return MediaAnalysis{}, err
	}
	prompt := `分析这段家庭私人媒体，并输出简体中文 JSON。忠于可观察内容，不识别人脸身份，不推断姓名、健康、种族、宗教、政治、精确住址等敏感属性。people 只能用概括描述（例如“两位成年人和一个孩子”）。summary 和 suggestedCaption 要自然简短；activities 是可观察活动。containsSensitive 在出现证件、医疗资料、裸露、精确地址等不适合默认家庭分享的内容时为 true。

根据成员的分享规则判断 suggestedVisibility（private 或 family），并从服务端给出的候选家庭成员中选择 suggestedRecipients。只能复制候选列表里的 memberId，不能创造成员、识别人脸或推断图片中的人物是谁。recipientReason 简短解释为什么这些家庭成员可能关心。规则没有明确允许、内容敏感、无法判断或没有合适收件人时，必须返回 private 和空收件人列表。你的结果只是建议，不会授予访问权限或执行分享。

成员分享规则：
` + strings.TrimSpace(sharePrompt) + `

候选家庭成员 JSON：
` + string(candidatesJSON)
	schema := map[string]any{
		"type": "object", "properties": map[string]any{
			"summary": map[string]any{"type": "string"}, "suggestedCaption": map[string]any{"type": "string"},
			"people": map[string]any{"type": "string"}, "activities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"containsSensitive": map[string]any{"type": "boolean"}, "suggestedVisibility": map[string]any{"type": "string", "enum": []string{"private", "family"}},
			"suggestedRecipients": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"memberId": map[string]any{"type": "string"}}, "required": []string{"memberId"}}},
			"recipientReason":     map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"},
		}, "required": []string{"summary", "suggestedCaption", "activities", "containsSensitive", "suggestedVisibility", "suggestedRecipients", "recipientReason", "reason"},
	}
	parts := []geminiGeneratePart{
		{Text: prompt},
		{InlineData: &geminiGenerateInlineData{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(media)}},
	}
	var analysis MediaAnalysis
	if err := g.generateJSON(ctx, "gemini media analysis", g.model, parts, schema, &analysis, 2<<20); err != nil {
		return MediaAnalysis{}, err
	}
	if analysis.Summary == "" || analysis.SuggestedCaption == "" || !validVisibility(analysis.SuggestedVisibility) || analysis.RecipientReason == "" || analysis.Reason == "" {
		return MediaAnalysis{}, errors.New("gemini media analysis returned incomplete output")
	}
	return analysis, nil
}

type geminiAudioProcessor struct {
	apiKey  string
	model   string
	client  *http.Client
	baseURL string
}

func (g *geminiAudioProcessor) Process(ctx context.Context, audio []byte, mimeType string) (AudioResult, error) {
	parts := []geminiGeneratePart{
		{Text: `请处理这段家庭语音。输出简体中文 JSON：
1. transcript：尽可能忠实的逐字转写，保留人名、金额、日期和不确定语气。
2. summary：一到两句简短、自然的整理结果。
不要补充录音中没有的事实，不要做健康诊断，不要把猜测改成确定事实。`},
		{InlineData: &geminiGenerateInlineData{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(audio)}},
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"transcript": map[string]any{"type": "string"},
			"summary":    map[string]any{"type": "string"},
		},
		"required": []string{"transcript", "summary"},
	}
	var result AudioResult
	if err := g.generateJSON(ctx, "gemini audio processing", g.model, parts, schema, &result, 2<<20); err != nil {
		return AudioResult{}, err
	}
	if result.Transcript == "" || result.Summary == "" {
		return AudioResult{}, errors.New("gemini returned empty transcript or summary")
	}
	return result, nil
}

func (g *geminiAudioProcessor) Summarize(ctx context.Context, updates []Update, members []Member, language string) (string, error) {
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
	instruction := "Create a warm, concise family daily summary in English from the public family updates below. Organize it naturally by family member. Stay faithful to the source; do not add facts, advice, health judgments, or privacy inferences. Output JSON with the field summary."
	if language == "zh" {
		instruction = "根据下面的家庭成员公开动态生成一份简体中文家庭日报。要求：温暖、简短、忠于原文；按成员自然组织；不要添加事实、建议、健康判断或隐私推断。输出 JSON，字段 summary。"
	}
	schema := map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string"}}, "required": []string{"summary"}}
	var result struct {
		Summary string `json:"summary"`
	}
	if err := g.generateJSON(ctx, "gemini daily summary", g.model, []geminiGeneratePart{{Text: instruction + "\n\n" + string(input)}}, schema, &result, 2<<20); err != nil {
		return "", err
	}
	if result.Summary == "" {
		return "", errors.New("gemini returned no daily summary")
	}
	return result.Summary, nil
}

func (g *geminiAudioProcessor) EvaluateShare(ctx context.Context, text, prompt string) (ShareDecision, error) {
	requestText := `你是家庭内容分享策略执行器。根据成员自己配置的规则，判断候选内容是否允许自动分享给整个家庭。规则没有明确允许时必须拒绝；不要补充事实。输出 JSON。\n\n成员规则：\n` + prompt + `\n\n候选内容：\n` + text
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"allowed": map[string]any{"type": "boolean"}, "reason": map[string]any{"type": "string"},
	}, "required": []string{"allowed", "reason"}}
	var decision ShareDecision
	if err := g.generateJSON(ctx, "gemini share evaluation", g.model, []geminiGeneratePart{{Text: requestText}}, schema, &decision, 2<<20); err != nil {
		return ShareDecision{}, err
	}
	if decision.Reason == "" {
		return ShareDecision{}, errors.New("gemini returned no share decision")
	}
	return decision, nil
}

func localDailySummary(updates []Update, members []Member, language string) string {
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
		separator := ": "
		if language == "zh" {
			separator = "："
		}
		result += names[update.MemberID] + separator + text
	}
	return result
}
