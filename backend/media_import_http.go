package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) memberCreateMediaImport(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaImportBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "媒体过大或上传格式不正确")
		return
	}
	defer r.MultipartForm.RemoveAll()
	deviceID := strings.TrimSpace(r.FormValue("deviceId"))
	clientMediaID := strings.TrimSpace(r.FormValue("clientMediaId"))
	if len([]rune(deviceID)) > 200 || len([]rune(clientMediaID)) > 200 {
		writeError(w, http.StatusBadRequest, "deviceId 和 clientMediaId 不能超过 200 个字")
		return
	}
	if clientMediaID != "" && deviceID == "" {
		writeError(w, http.StatusBadRequest, "使用 clientMediaId 时必须同时提供 deviceId")
		return
	}
	if clientMediaID != "" {
		if existing, err := a.store.mediaImportByClientID(r.Context(), member.ID, deviceID, clientMediaID); err == nil {
			writeJSON(w, http.StatusOK, existing)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "暂时无法检查同步记录")
			return
		}
	}
	var capturedAt *time.Time
	if raw := strings.TrimSpace(r.FormValue("capturedAt")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "capturedAt 必须是 RFC3339 时间")
			return
		}
		value = value.UTC()
		capturedAt = &value
	}
	file, header, err := r.FormFile("media")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择照片或视频")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMediaImportBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxMediaImportBytes {
		writeError(w, http.StatusBadRequest, "媒体为空或超过 100MB")
		return
	}
	mimeType := strings.TrimSpace(strings.Split(header.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = strings.Split(http.DetectContentType(data), ";")[0]
	}
	mediaType, extension, ok := importMediaType(mimeType)
	if !ok {
		writeError(w, http.StatusBadRequest, "只支持 JPEG、PNG、WebP、GIF、MP4、MOV 或 WebM")
		return
	}
	originalName := filepath.Base(strings.TrimSpace(header.Filename))
	if originalName == "." || originalName == "" {
		originalName = "mobile-upload" + extension
	}
	if len([]rune(originalName)) > 255 {
		writeError(w, http.StatusBadRequest, "原始文件名不能超过 255 个字")
		return
	}
	now := time.Now().UTC()
	digest := sha256.Sum256(data)
	item := MediaImport{ID: newID(), FamilyID: member.FamilyID, MemberID: member.ID, MediaType: mediaType, MimeType: mimeType,
		OriginalName: originalName, CapturedAt: capturedAt, DeviceID: deviceID, ClientMediaID: clientMediaID, SHA256: hex.EncodeToString(digest[:]),
		AnalysisStatus: "processing", ShareDecision: "pending", CreatedAt: now, UpdatedAt: now}
	mediaFile := item.ID + extension
	for _, dir := range []string{filepath.Join(a.spacesRoot, "members", member.ID, "media"), filepath.Join(a.spacesRoot, "members", member.ID, "imports")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			writeError(w, http.StatusInternalServerError, "暂时无法准备私人 Space")
			return
		}
	}
	if err := writeFileAtomically(filepath.Join(a.spacesRoot, "members", member.ID, "media"), mediaFile, data); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法把媒体保存到私人 Space")
		return
	}
	item.MediaURL = "/space-files/members/" + member.ID + "/media/" + mediaFile
	if err := a.store.createMediaImport(r.Context(), item, mediaFile); err != nil {
		writeError(w, http.StatusInternalServerError, "媒体已落盘，但暂时无法更新本地索引")
		return
	}
	if err := persistMediaImportToSpace(a.spacesRoot, item); err != nil {
		writeError(w, http.StatusInternalServerError, "媒体已保存，但暂时无法写入导入元数据")
		return
	}

	settings, settingsErr := a.store.getMemberSettings(r.Context(), member.ID)
	var recipientCandidates []Member
	if settingsErr != nil {
		item.AnalysisStatus = "failed"
		item.AnalysisError = "暂时无法读取分享策略"
	} else if len(data) > maxInlineAnalysisBytes {
		item.AnalysisStatus = "skipped_too_large"
		item.AnalysisError = "媒体已安全保存；当前版本只自动分析 14MB 以内的文件"
	} else {
		members, membersErr := a.store.listMembers(r.Context(), member.FamilyID)
		if membersErr != nil {
			item.AnalysisStatus = "failed"
			item.AnalysisError = "媒体已安全保存；暂时无法读取家庭收件人"
		} else {
			for _, candidate := range members {
				if candidate.ID != member.ID {
					recipientCandidates = append(recipientCandidates, candidate)
				}
			}
			analysis, analyzeErr := a.mediaAI.AnalyzeMedia(r.Context(), data, mimeType, settings.SharePrompt, recipientCandidates)
			if analyzeErr != nil {
				item.AnalysisStatus = "failed"
				item.AnalysisError = "媒体已安全保存；AI 暂时无法完成分析"
			} else {
				analysis = validateMediaRecipientSuggestions(analysis, recipientCandidates)
				item.AnalysisStatus = "ready"
				item.Analysis = &analysis
			}
		}
	}
	item.UpdatedAt = time.Now().UTC()
	if err := a.store.saveMediaImportAnalysis(r.Context(), item); err != nil {
		writeError(w, http.StatusInternalServerError, "媒体已保存，但暂时无法保存分析结果")
		return
	}
	if err := persistMediaImportToSpace(a.spacesRoot, item); err != nil {
		writeError(w, http.StatusInternalServerError, "分析已保存到索引，但暂时无法写入导入元数据")
		return
	}
	if settingsErr == nil && settings.ShareMode == "auto" && item.Analysis != nil && !item.Analysis.ContainsSensitive && item.Analysis.SuggestedVisibility == "family" && suggestionsCoverCandidates(item.Analysis.SuggestedRecipients, recipientCandidates) {
		if shared, err := a.shareMediaImport(r, item, item.Analysis.SuggestedCaption, mediaFile); err == nil {
			item = shared
		}
	}
	writeJSON(w, http.StatusCreated, item)
}

func validateMediaRecipientSuggestions(analysis MediaAnalysis, candidates []Member) MediaAnalysis {
	allowed := make(map[string]Member, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate.ID] = candidate
	}
	seen := make(map[string]bool, len(analysis.SuggestedRecipients))
	validated := make([]MediaShareRecipient, 0, len(analysis.SuggestedRecipients))
	for _, suggestion := range analysis.SuggestedRecipients {
		candidate, ok := allowed[strings.TrimSpace(suggestion.MemberID)]
		if !ok || seen[candidate.ID] {
			continue
		}
		seen[candidate.ID] = true
		validated = append(validated, MediaShareRecipient{MemberID: candidate.ID, Name: candidate.Name})
	}
	analysis.SuggestedRecipients = validated
	if analysis.ContainsSensitive || analysis.SuggestedVisibility != "family" || len(validated) == 0 {
		analysis.SuggestedVisibility = "private"
		analysis.SuggestedRecipients = []MediaShareRecipient{}
	}
	return analysis
}

func suggestionsCoverCandidates(suggestions []MediaShareRecipient, candidates []Member) bool {
	if len(candidates) == 0 || len(suggestions) != len(candidates) {
		return false
	}
	wanted := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		wanted[candidate.ID] = true
	}
	for _, suggestion := range suggestions {
		if !wanted[suggestion.MemberID] {
			return false
		}
	}
	return true
}

func (a *app) memberListMediaImports(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	items, err := a.store.listMediaImports(r.Context(), member.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取媒体导入记录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mediaImports": items})
}

func (a *app) memberGetMediaImport(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	item, err := a.store.getMediaImport(r.Context(), r.PathValue("id"), member.ID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这条媒体导入记录")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取媒体导入记录")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *app) memberDecideMediaImport(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	var input struct {
		Visibility string `json:"visibility"`
		Caption    string `json:"caption"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "分享决定格式不正确")
		return
	}
	input.Visibility = strings.TrimSpace(input.Visibility)
	input.Caption = strings.TrimSpace(input.Caption)
	if !validVisibility(input.Visibility) || len([]rune(input.Caption)) > 2000 {
		writeError(w, http.StatusBadRequest, "可见范围或说明不正确")
		return
	}
	item, err := a.store.getMediaImport(r.Context(), r.PathValue("id"), member.ID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这条媒体导入记录")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取媒体导入记录")
		return
	}
	if input.Visibility == "private" {
		item, err = a.store.markMediaImportPrivate(r.Context(), item.ID, member.ID, time.Now().UTC())
	} else {
		item, err = a.shareMediaImport(r, item, input.Caption, filepath.Base(item.MediaURL))
	}
	if err != nil {
		writeError(w, http.StatusConflict, "暂时无法应用这个分享决定")
		return
	}
	_ = persistMediaImportToSpace(a.spacesRoot, item)
	writeJSON(w, http.StatusOK, item)
}

func (a *app) shareMediaImport(r *http.Request, item MediaImport, caption, mediaFile string) (MediaImport, error) {
	if item.UpdateID != "" {
		return item, nil
	}
	caption = strings.TrimSpace(caption)
	if caption == "" && item.Analysis != nil {
		caption = strings.TrimSpace(item.Analysis.SuggestedCaption)
	}
	if caption == "" {
		if item.MediaType == "video" {
			caption = "分享了一段视频"
		} else {
			caption = "分享了一张照片"
		}
	}
	now := time.Now().UTC()
	update := Update{ID: newID(), FamilyID: item.FamilyID, MemberID: item.MemberID, Type: item.MediaType, Text: caption, Visibility: "family",
		MediaURL: item.MediaURL, Source: "mobile_media_import", CreatedAt: now}
	if item.Analysis != nil {
		update.AISummary = item.Analysis.Summary
	}
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		return MediaImport{}, err
	}
	shared, err := a.store.shareMediaImport(r.Context(), item.ID, item.MemberID, update, mediaFile, now)
	if err != nil {
		return MediaImport{}, err
	}
	_ = persistMediaImportToSpace(a.spacesRoot, shared)
	return shared, nil
}

func importMediaType(mimeType string) (mediaType, extension string, ok bool) {
	if ext, imageOK := imageExtension(mimeType); imageOK {
		return "image", ext, true
	}
	switch mimeType {
	case "video/mp4":
		return "video", ".mp4", true
	case "video/quicktime":
		return "video", ".mov", true
	case "video/webm":
		return "video", ".webm", true
	default:
		return "", "", false
	}
}
