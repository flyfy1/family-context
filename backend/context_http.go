package main

import (
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const defaultFamilyID = "our-family"

var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validMemberRole(role string) bool {
	return role == "member" || role == "elder" || role == "child"
}

func (a *app) listMembers(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	members, err := a.store.listMembers(r.Context(), familyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭成员")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (a *app) createMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FamilyID string `json:"familyId"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		IsAdmin  bool   `json:"isAdmin"`
		Color    string `json:"color"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "成员信息格式不正确")
		return
	}
	input.FamilyID = strings.TrimSpace(input.FamilyID)
	input.Name = strings.TrimSpace(input.Name)
	input.Role = strings.TrimSpace(input.Role)
	input.Color = strings.TrimSpace(input.Color)
	if input.FamilyID == "" {
		input.FamilyID = defaultFamilyID
	}
	if input.Name == "" || len([]rune(input.Name)) > 30 {
		writeError(w, http.StatusBadRequest, "成员称呼不能为空且不能超过 30 个字")
		return
	}
	if !validMemberRole(input.Role) {
		writeError(w, http.StatusBadRequest, "成员角色只能是普通成员、老人或孩子")
		return
	}
	if !hexColorPattern.MatchString(input.Color) {
		input.Color = "#AD4C34"
	}
	member := Member{ID: newID(), FamilyID: input.FamilyID, Name: input.Name, Role: input.Role, IsAdmin: input.IsAdmin, Color: input.Color, CreatedAt: time.Now().UTC()}
	if err := createMemberSpace(a.spacesRoot, member); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法创建成员文件空间")
		return
	}
	if err := a.store.createMember(r.Context(), member); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存家庭成员")
		return
	}
	writeJSON(w, http.StatusCreated, member)
}

func (a *app) listUpdates(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	memberID := strings.TrimSpace(r.URL.Query().Get("memberId"))
	if scope == "mine" && memberID == "" {
		writeError(w, http.StatusBadRequest, "查看个人空间时必须选择成员")
		return
	}
	if scope != "mine" {
		scope = "family"
	}
	updates, err := a.store.listUpdates(r.Context(), familyID, memberID, scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭动态")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updates": updates})
}

func (a *app) createTextUpdate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FamilyID   string `json:"familyId"`
		MemberID   string `json:"memberId"`
		Text       string `json:"text"`
		Visibility string `json:"visibility"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "动态格式不正确")
		return
	}
	input.FamilyID = strings.TrimSpace(input.FamilyID)
	input.MemberID = strings.TrimSpace(input.MemberID)
	input.Text = strings.TrimSpace(input.Text)
	input.Visibility = strings.TrimSpace(input.Visibility)
	if input.FamilyID == "" {
		input.FamilyID = defaultFamilyID
	}
	if input.Text == "" || len([]rune(input.Text)) > 2000 {
		writeError(w, http.StatusBadRequest, "动态不能为空且不能超过 2000 个字")
		return
	}
	if !validVisibility(input.Visibility) {
		writeError(w, http.StatusBadRequest, "请选择仅自己或家庭可见")
		return
	}
	if exists, err := a.store.memberExists(r.Context(), input.MemberID, input.FamilyID); err != nil || !exists {
		writeError(w, http.StatusBadRequest, "没有找到这个家庭成员")
		return
	}
	update := Update{ID: newID(), FamilyID: input.FamilyID, MemberID: input.MemberID, Type: "text", Text: input.Text, Visibility: input.Visibility, Source: "member", CreatedAt: time.Now().UTC()}
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法写入成员文件空间")
		return
	}
	if err := a.store.createUpdate(r.Context(), update, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存动态")
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

func (a *app) createImageUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxImageBytes); err != nil {
		writeError(w, http.StatusBadRequest, "图片过大或上传格式不正确")
		return
	}
	familyID := strings.TrimSpace(r.FormValue("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	memberID := strings.TrimSpace(r.FormValue("memberId"))
	visibility := strings.TrimSpace(r.FormValue("visibility"))
	caption := strings.TrimSpace(r.FormValue("text"))
	if !validVisibility(visibility) || len([]rune(caption)) > 2000 {
		writeError(w, http.StatusBadRequest, "图片说明或可见范围不正确")
		return
	}
	if exists, err := a.store.memberExists(r.Context(), memberID, familyID); err != nil || !exists {
		writeError(w, http.StatusBadRequest, "没有找到这个家庭成员")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择一张图片")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxImageBytes {
		writeError(w, http.StatusBadRequest, "图片为空或超过 25MB")
		return
	}
	mimeType := strings.Split(header.Header.Get("Content-Type"), ";")[0]
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	extension, ok := imageExtension(mimeType)
	if !ok {
		writeError(w, http.StatusBadRequest, "只支持 JPEG、PNG、WebP 或 GIF 图片")
		return
	}
	updateID := newID()
	fileName := updateID + extension
	mediaDir := filepath.Join(a.spacesRoot, "members", memberID, "media")
	if err := writeFileAtomically(mediaDir, fileName, data); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存图片")
		return
	}
	if caption == "" {
		caption = "分享了一张照片"
	}
	update := Update{ID: updateID, FamilyID: familyID, MemberID: memberID, Type: "image", Text: caption,
		Visibility: visibility, MediaURL: "/space-files/members/" + memberID + "/media/" + fileName, Source: "member_image", CreatedAt: time.Now().UTC()}
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		writeError(w, http.StatusInternalServerError, "图片已保存，但暂时无法写入成员记录")
		return
	}
	if err := a.store.createUpdate(r.Context(), update, fileName); err != nil {
		writeError(w, http.StatusInternalServerError, "图片已保存，但暂时无法更新索引")
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

func (a *app) createVoiceUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAudioBytes); err != nil {
		writeError(w, http.StatusBadRequest, "录音过大或上传格式不正确")
		return
	}
	familyID := strings.TrimSpace(r.FormValue("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	memberID := strings.TrimSpace(r.FormValue("memberId"))
	visibility := strings.TrimSpace(r.FormValue("visibility"))
	if !validVisibility(visibility) {
		writeError(w, http.StatusBadRequest, "请选择仅自己或家庭可见")
		return
	}
	if exists, err := a.store.memberExists(r.Context(), memberID, familyID); err != nil || !exists {
		writeError(w, http.StatusBadRequest, "没有找到这个家庭成员")
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择一段录音")
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(io.LimitReader(file, maxAudioBytes+1))
	if err != nil || len(audio) == 0 || len(audio) > maxAudioBytes {
		writeError(w, http.StatusBadRequest, "录音为空或超过 18MB")
		return
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	if mimeType == "audio/x-m4a" {
		mimeType = "audio/mp4"
	}
	if !strings.HasPrefix(mimeType, "audio/") {
		writeError(w, http.StatusBadRequest, "只支持音频文件")
		return
	}
	updateID := newID()
	fileName := updateID + extensionForMime(mimeType)
	mediaDir := filepath.Join(a.spacesRoot, "members", memberID, "media")
	if err := writeFileAtomically(mediaDir, fileName, audio); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存录音")
		return
	}
	result, processErr := a.ai.Process(r.Context(), audio, mimeType)
	update := Update{ID: updateID, FamilyID: familyID, MemberID: memberID, Type: "voice", Visibility: visibility, Source: "member_voice", CreatedAt: time.Now().UTC()}
	if processErr != nil {
		update.Text = "语音已经保存在本地，AI 暂时没有完成整理。"
		update.Source = "member_voice_processing_failed"
	} else {
		update.Text = result.Summary
		update.Transcript = result.Transcript
		update.AISummary = result.Summary
	}
	update.AudioURL = "/space-files/members/" + memberID + "/media/" + fileName
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		writeError(w, http.StatusInternalServerError, "录音已保存，但暂时无法写入成员记录")
		return
	}
	if err := a.store.createUpdate(r.Context(), update, fileName); err != nil {
		writeError(w, http.StatusInternalServerError, "录音已保存，但暂时无法更新索引")
		return
	}
	if processErr != nil {
		writeJSON(w, http.StatusBadGateway, update)
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

func (a *app) latestDailySummary(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	language, ok := normalizeLanguage(r.URL.Query().Get("language"))
	if !ok {
		writeError(w, http.StatusBadRequest, "language must be en or zh")
		return
	}
	summary, err := a.store.latestDailySummary(r.Context(), familyID, language)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"summary": nil})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭日报")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary})
}

func (a *app) generateDailySummary(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FamilyID string `json:"familyId"`
		Date     string `json:"date"`
		Language string `json:"language"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "日报请求格式不正确")
		return
	}
	if input.FamilyID == "" {
		input.FamilyID = defaultFamilyID
	}
	language, ok := normalizeLanguage(input.Language)
	if !ok {
		writeError(w, http.StatusBadRequest, "language must be en or zh")
		return
	}
	if _, err := time.Parse("2006-01-02", input.Date); err != nil {
		writeError(w, http.StatusBadRequest, "日报日期格式不正确")
		return
	}
	updates, err := a.store.sharedUpdatesForDate(r.Context(), input.FamilyID, input.Date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取今日动态")
		return
	}
	if len(updates) == 0 {
		writeError(w, http.StatusConflict, "今天还没有家庭可见的动态")
		return
	}
	members, err := a.store.listMembers(r.Context(), input.FamilyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭成员")
		return
	}
	content, err := a.summarizer.Summarize(r.Context(), updates, members, language)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI 暂时无法生成家庭日报")
		return
	}
	summary := DailySummary{ID: newID(), FamilyID: input.FamilyID, Date: input.Date, Content: content, Language: language, UpdateCount: len(updates), CreatedAt: time.Now().UTC()}
	if err := persistSummaryToSpace(a.spacesRoot, summary); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法写入家庭日报文件")
		return
	}
	if err := a.store.createDailySummary(r.Context(), summary); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存家庭日报")
		return
	}
	writeJSON(w, http.StatusCreated, summary)
}

func (a *app) serveSpaceFile(w http.ResponseWriter, r *http.Request) {
	requested := filepath.ToSlash(filepath.Clean(r.PathValue("path")))
	parts := strings.Split(requested, "/")
	if len(parts) != 4 || parts[0] != "members" || parts[2] != "media" || parts[1] == "" || parts[3] == "" {
		writeError(w, http.StatusNotFound, "没有找到这个文件")
		return
	}
	allowed := secureEqual(r.Header.Get("X-Family-Token"), a.apiToken) || secureEqual(r.Header.Get("X-Admin-Token"), a.adminToken)
	if !allowed {
		member, ok := a.authenticateMember(r)
		allowed = ok && member.ID == parts[1]
	}
	if !allowed {
		writeError(w, http.StatusUnauthorized, "无权读取这个成员的文件")
		return
	}
	path := filepath.Join(a.spacesRoot, filepath.FromSlash(requested))
	rootPrefix := filepath.Clean(a.spacesRoot) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(path), rootPrefix) {
		writeError(w, http.StatusNotFound, "没有找到这个文件")
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "没有找到这个文件")
		return
	}
	http.ServeFile(w, r, path)
}

func validVisibility(value string) bool {
	return value == "private" || value == "family"
}
