package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type judgmentMemberContextKey struct{}

type optionalMemberAuthenticator interface {
	authenticateMember(*http.Request) (Member, bool)
}

func (a *app) registerJudgmentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/judgment-prompts", a.judgmentAuthorized(a.listMyJudgmentPrompts))
	mux.HandleFunc("POST /api/v1/me/judgment-prompts", a.judgmentAuthorized(a.createMyJudgmentPrompt))
	mux.HandleFunc("POST /api/v1/me/thoughts", a.judgmentAuthorized(a.createThoughtJudgment))
	mux.HandleFunc("GET /api/v1/me/thoughts/{id}/judgment", a.judgmentAuthorized(a.getThoughtJudgment))
	mux.HandleFunc("POST /api/v1/me/thoughts/{id}/share", a.judgmentAuthorized(a.shareJudgedThought))
}

func (a *app) judgmentAuthorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authenticator, ok := any(a).(optionalMemberAuthenticator); ok {
			if member, authenticated := authenticator.authenticateMember(r); authenticated {
				next(w, r.WithContext(context.WithValue(r.Context(), judgmentMemberContextKey{}, member)))
				return
			}
		}
		if r.Header.Get("X-Family-Token") == a.apiToken {
			member, err := a.store.judgmentMemberByID(r.Context(), strings.TrimSpace(r.Header.Get("X-Member-ID")))
			if err == nil {
				next(w, r.WithContext(context.WithValue(r.Context(), judgmentMemberContextKey{}, member)))
				return
			}
		}
		writeError(w, http.StatusUnauthorized, "成员访问凭证无效")
	}
}

func judgmentMemberFromContext(ctx context.Context) Member {
	member, _ := ctx.Value(judgmentMemberContextKey{}).(Member)
	return member
}

func (a *app) listMyJudgmentPrompts(w http.ResponseWriter, r *http.Request) {
	member := judgmentMemberFromContext(r.Context())
	prompts, err := a.store.listJudgmentPrompts(r.Context(), member.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取 Prompt")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompts": prompts})
}

func (a *app) createMyJudgmentPrompt(w http.ResponseWriter, r *http.Request) {
	member := judgmentMemberFromContext(r.Context())
	var input struct {
		Name        string `json:"name"`
		Instruction string `json:"instruction"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Prompt 格式不正确")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Instruction = strings.TrimSpace(input.Instruction)
	if input.Name == "" || len([]rune(input.Name)) > 80 || input.Instruction == "" || len([]rune(input.Instruction)) > 4000 {
		writeError(w, http.StatusBadRequest, "Prompt 名称或内容不正确")
		return
	}
	now := time.Now().UTC()
	prompt := JudgmentPrompt{ID: newID(), MemberID: member.ID, Name: input.Name, Instruction: input.Instruction, CreatedAt: now, UpdatedAt: now}
	if err := a.store.createJudgmentPrompt(r.Context(), prompt); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存 Prompt")
		return
	}
	writeJSON(w, http.StatusCreated, prompt)
}

func (a *app) createThoughtJudgment(w http.ResponseWriter, r *http.Request) {
	member := judgmentMemberFromContext(r.Context())
	var input struct {
		Text     string `json:"text"`
		PromptID string `json:"promptId"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "想法格式不正确")
		return
	}
	input.Text = strings.TrimSpace(input.Text)
	input.PromptID = strings.TrimSpace(input.PromptID)
	if input.Text == "" || len([]rune(input.Text)) > 4000 || input.PromptID == "" {
		writeError(w, http.StatusBadRequest, "想法不能为空且不能超过 4000 个字")
		return
	}
	prompt, err := a.store.getJudgmentPrompt(r.Context(), input.PromptID, member.ID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "没有找到这个 Prompt")
		} else {
			writeError(w, http.StatusInternalServerError, "暂时无法读取 Prompt")
		}
		return
	}
	now := time.Now().UTC()
	update := Update{ID: newID(), FamilyID: member.FamilyID, MemberID: member.ID, Type: "text", Text: input.Text,
		Visibility: "private", Source: "member_thought", CreatedAt: now}
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存想法")
		return
	}
	if err := a.store.createUpdate(r.Context(), update, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存想法")
		return
	}
	judge, ok := a.ai.(thoughtJudge)
	if !ok {
		writeJSON(w, http.StatusBadGateway, map[string]any{"update": update, "error": "想法已私密保存，但 AI 暂时不支持判断"})
		return
	}
	result, err := judge.Judge(r.Context(), input.Text, prompt.Instruction)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"update": update, "error": "想法已私密保存，但 AI 判断暂时失败"})
		return
	}
	evaluation := JudgmentEvaluation{ID: newID(), UpdateID: update.ID, PromptID: prompt.ID, MemberID: member.ID,
		PromptSnapshot: prompt.Instruction, Model: judgmentModel(a.ai), Decision: result.Decision, OrganizedText: result.OrganizedText,
		Reason: result.Reason, SensitiveFlags: result.SensitiveFlags, CreatedAt: time.Now().UTC()}
	if err := a.store.createJudgmentEvaluation(r.Context(), evaluation); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"update": update, "error": "想法已私密保存，但暂时无法保存 AI 判断"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"update": update, "judgment": evaluation})
}

func (a *app) getThoughtJudgment(w http.ResponseWriter, r *http.Request) {
	member := judgmentMemberFromContext(r.Context())
	evaluation, err := a.store.judgmentEvaluationForUpdate(r.Context(), r.PathValue("id"), member.ID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "没有找到这条想法的判断")
		} else {
			writeError(w, http.StatusInternalServerError, "暂时无法读取判断")
		}
		return
	}
	writeJSON(w, http.StatusOK, evaluation)
}

func (a *app) shareJudgedThought(w http.ResponseWriter, r *http.Request) {
	member := judgmentMemberFromContext(r.Context())
	updateID := r.PathValue("id")
	if _, err := a.store.judgmentEvaluationForUpdate(r.Context(), updateID, member.ID); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusConflict, "这条想法还没有完成 AI 判断")
		} else {
			writeError(w, http.StatusInternalServerError, "暂时无法读取判断")
		}
		return
	}
	update, err := a.store.updateForMember(r.Context(), updateID, member.ID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "没有找到这条想法")
		} else {
			writeError(w, http.StatusInternalServerError, "暂时无法读取想法")
		}
		return
	}
	if update.Visibility != "private" {
		writeError(w, http.StatusConflict, "这条想法已经分享")
		return
	}
	update, err = a.store.shareJudgedUpdate(r.Context(), updateID, member.ID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法分享想法")
		return
	}
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		writeError(w, http.StatusInternalServerError, "想法已经分享，但家庭文件投影暂时写入失败")
		return
	}
	writeJSON(w, http.StatusOK, update)
}
