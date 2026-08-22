package main

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *app) createBedtimeStory(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FamilyID    string `json:"familyId"`
		ChildID     string `json:"childId"`
		AudienceAge int    `json:"audienceAge"`
		Days        int    `json:"days"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "睡前故事请求格式不正确")
		return
	}
	input.FamilyID = strings.TrimSpace(input.FamilyID)
	input.ChildID = strings.TrimSpace(input.ChildID)
	if input.FamilyID == "" {
		input.FamilyID = defaultFamilyID
	}
	if input.AudienceAge == 0 {
		input.AudienceAge = 6
	}
	if input.Days == 0 {
		input.Days = 7
	}
	if input.ChildID == "" || input.AudienceAge < 3 || input.AudienceAge > 12 || input.Days < 1 || input.Days > 30 {
		writeError(w, http.StatusBadRequest, "请选择孩子；年龄须为 3 到 12 岁，时间范围须为 1 到 30 天")
		return
	}
	child, err := a.store.getMember(r.Context(), input.ChildID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && child.FamilyID != input.FamilyID) {
		writeError(w, http.StatusBadRequest, "没有找到这个家庭中的孩子")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取孩子信息")
		return
	}
	updates, err := a.store.sharedUpdatesSince(r.Context(), input.FamilyID, time.Now().UTC().Add(-time.Duration(input.Days)*24*time.Hour), 40)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭 Context")
		return
	}
	if len(updates) == 0 {
		writeError(w, http.StatusConflict, "这段时间还没有家庭可见的内容可以写成故事")
		return
	}
	members, err := a.store.listMembers(r.Context(), input.FamilyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭成员")
		return
	}
	draft, err := a.storyAI.GenerateBedtimeStory(r.Context(), child, input.AudienceAge, updates, members)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI 暂时无法生成睡前故事")
		return
	}
	draft.Title = strings.TrimSpace(draft.Title)
	draft.Content = strings.TrimSpace(draft.Content)
	if draft.Title == "" || len([]rune(draft.Title)) > 100 || draft.Content == "" || len([]rune(draft.Content)) > 5000 {
		writeError(w, http.StatusBadGateway, "AI 返回的故事格式不完整")
		return
	}
	validSources := make(map[string]bool, len(updates))
	for _, update := range updates {
		validSources[update.ID] = true
	}
	seen := make(map[string]bool)
	sources := make([]string, 0, len(draft.SourceUpdateIDs))
	for _, id := range draft.SourceUpdateIDs {
		if !validSources[id] {
			writeError(w, http.StatusBadGateway, "AI 返回了不属于家庭 Context 的来源")
			return
		}
		if !seen[id] {
			seen[id] = true
			sources = append(sources, id)
		}
	}
	if len(sources) == 0 {
		writeError(w, http.StatusBadGateway, "故事没有可回溯的家庭来源")
		return
	}
	now := time.Now().UTC()
	story := BedtimeStory{ID: newID(), FamilyID: input.FamilyID, ChildID: child.ID, ChildName: child.Name, AudienceAge: input.AudienceAge,
		Title: draft.Title, Content: draft.Content, SourceUpdateIDs: sources, Voice: bedtimeStoryVoice(), Status: "text_ready", CreatedAt: now, UpdatedAt: now}
	if err := a.store.createBedtimeStory(r.Context(), story); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存睡前故事")
		return
	}
	if err := persistBedtimeStoryToSpace(a.spacesRoot, story); err != nil {
		writeError(w, http.StatusInternalServerError, "故事已保存到索引，但暂时无法写入本地文件")
		return
	}
	wav, synthErr := a.tts.SynthesizeSpeech(r.Context(), story.Content, story.Voice)
	story.UpdatedAt = time.Now().UTC()
	audioFile := ""
	if synthErr != nil {
		story.Status = "audio_failed"
		story.ErrorMessage = "故事文本已保存；Gemini 暂时无法生成音频"
	} else {
		story.Status = "ready"
		audioFile = story.ID + ".wav"
		if err := writeFileAtomically(filepath.Join(a.spacesRoot, "shared", "stories"), audioFile, wav); err != nil {
			story.Status = "audio_failed"
			story.ErrorMessage = "故事文本已保存；音频暂时无法写入本地存储"
			audioFile = ""
		}
	}
	if err := a.store.finishBedtimeStoryAudio(r.Context(), story, audioFile); err != nil {
		writeError(w, http.StatusInternalServerError, "故事已生成，但暂时无法更新音频状态")
		return
	}
	if audioFile != "" {
		story.AudioURL = "/api/v1/bedtime-stories/" + story.ID + "/audio"
	}
	if err := persistBedtimeStoryToSpace(a.spacesRoot, story); err != nil {
		writeError(w, http.StatusInternalServerError, "故事已保存到索引，但暂时无法更新本地文件")
		return
	}
	writeJSON(w, http.StatusCreated, story)
}

func (a *app) listBedtimeStories(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	stories, err := a.store.listBedtimeStories(r.Context(), familyID, strings.TrimSpace(r.URL.Query().Get("childId")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取睡前故事")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bedtimeStories": stories})
}

func (a *app) getBedtimeStory(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	story, err := a.store.getBedtimeStory(r.Context(), r.PathValue("id"), familyID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个睡前故事")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取睡前故事")
		return
	}
	writeJSON(w, http.StatusOK, story)
}

func (a *app) serveBedtimeStoryAudio(w http.ResponseWriter, r *http.Request) {
	story, err := a.store.getBedtimeStory(r.Context(), r.PathValue("id"), defaultFamilyID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && story.Status != "ready") {
		writeError(w, http.StatusNotFound, "没有找到这个故事音频")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取故事音频")
		return
	}
	path := filepath.Join(a.spacesRoot, "shared", "stories", story.ID+".wav")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "没有找到这个故事音频")
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Content-Disposition", `inline; filename="bedtime-story-`+story.ID+`.wav"`)
	http.ServeFile(w, r, path)
}
