package main

import (
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const maxImageBytes = 25 << 20

func (a *app) getMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, memberFromContext(r.Context()))
}

func (a *app) memberDismissAttention(w http.ResponseWriter, r *http.Request) {
	actor := memberFromContext(r.Context())
	err := a.store.dismissMemberAttention(r.Context(), actor.FamilyID, r.PathValue("id"), actor.ID, time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到需要关注的老人")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法取消关注状态")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) memberListUpdates(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	updates, err := a.store.listUpdates(r.Context(), member.FamilyID, member.ID, "mine")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取个人 Space")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updates": updates})
}

func (a *app) memberCreateTextUpdate(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	var input struct {
		Text       string `json:"text"`
		Visibility string `json:"visibility"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "动态格式不正确")
		return
	}
	input.Text = strings.TrimSpace(input.Text)
	if input.Text == "" || len([]rune(input.Text)) > 2000 || !validVisibility(input.Visibility) {
		writeError(w, http.StatusBadRequest, "动态内容或可见范围不正确")
		return
	}
	update := Update{ID: newID(), FamilyID: member.FamilyID, MemberID: member.ID, Type: "text", Text: input.Text,
		Visibility: input.Visibility, Source: "member_api", CreatedAt: time.Now().UTC()}
	if err := a.persistNewUpdate(r, update, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存动态")
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

func (a *app) memberCreateImageUpdate(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxImageBytes); err != nil {
		writeError(w, http.StatusBadRequest, "图片过大或上传格式不正确")
		return
	}
	visibility := strings.TrimSpace(r.FormValue("visibility"))
	caption := strings.TrimSpace(r.FormValue("text"))
	if !validVisibility(visibility) || len([]rune(caption)) > 2000 {
		writeError(w, http.StatusBadRequest, "图片说明或可见范围不正确")
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
	mediaDir := filepath.Join(a.spacesRoot, "members", member.ID, "media")
	if err := writeFileAtomically(mediaDir, fileName, data); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存图片")
		return
	}
	if caption == "" {
		caption = "分享了一张照片"
	}
	update := Update{ID: updateID, FamilyID: member.FamilyID, MemberID: member.ID, Type: "image", Text: caption,
		Visibility: visibility, MediaURL: "/space-files/members/" + member.ID + "/media/" + fileName, Source: "member_image_api", CreatedAt: time.Now().UTC()}
	if err := a.persistNewUpdate(r, update, fileName); err != nil {
		writeError(w, http.StatusInternalServerError, "图片已保存，但暂时无法更新索引")
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

func (a *app) getSharePolicy(w http.ResponseWriter, r *http.Request) {
	member := judgmentMemberFromContext(r.Context())
	settings, err := a.store.getMemberSettings(r.Context(), member.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取分享策略")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *app) saveSharePolicy(w http.ResponseWriter, r *http.Request) {
	member := judgmentMemberFromContext(r.Context())
	var input struct {
		ShareMode   string `json:"shareMode"`
		SharePrompt string `json:"sharePrompt"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "分享策略格式不正确")
		return
	}
	input.SharePrompt = strings.TrimSpace(input.SharePrompt)
	if input.ShareMode != "manual" && input.ShareMode != "review" && input.ShareMode != "auto" {
		writeError(w, http.StatusBadRequest, "分享模式必须是手动、审核或自动")
		return
	}
	if len([]rune(input.SharePrompt)) > 4000 || (input.ShareMode == "auto" && input.SharePrompt == "") {
		writeError(w, http.StatusBadRequest, "自动分享必须配置 Prompt，且不能超过 4000 个字")
		return
	}
	settings := MemberSettings{MemberID: member.ID, ShareMode: input.ShareMode, SharePrompt: input.SharePrompt, UpdatedAt: time.Now().UTC()}
	if err := persistMemberSettings(a.spacesRoot, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法写入成员 Space")
		return
	}
	if err := a.store.saveMemberSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存分享策略")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *app) persistNewUpdate(r *http.Request, update Update, mediaFile string) error {
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		return err
	}
	return a.store.createUpdate(r.Context(), update, mediaFile)
}

func imageExtension(mimeType string) (string, bool) {
	switch mimeType {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	default:
		if extensions, _ := mime.ExtensionsByType(mimeType); len(extensions) > 0 && strings.HasPrefix(mimeType, "image/") {
			return "", false
		}
		return "", false
	}
}
