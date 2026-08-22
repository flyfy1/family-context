package main

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) adminListMembers(w http.ResponseWriter, r *http.Request) {
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

func (a *app) adminCreateMember(w http.ResponseWriter, r *http.Request) {
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
	if input.Name == "" || len([]rune(input.Name)) > 30 || !validMemberRole(input.Role) {
		writeError(w, http.StatusBadRequest, "成员称呼或角色不正确")
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
	token := newAccessToken()
	if err := a.store.setMemberTokenHash(r.Context(), member.ID, hashToken(token), time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "成员已创建，但暂时无法生成访问令牌")
		return
	}
	writeJSON(w, http.StatusCreated, MemberCredential{Member: member, AccessToken: token})
}

func (a *app) adminUpdateMember(w http.ResponseWriter, r *http.Request) {
	member, err := a.store.getMember(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个家庭成员")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭成员")
		return
	}
	var input struct {
		Name    string `json:"name"`
		Role    string `json:"role"`
		IsAdmin bool   `json:"isAdmin"`
		Color   string `json:"color"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "成员信息格式不正确")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 30 || !validMemberRole(input.Role) || !hexColorPattern.MatchString(input.Color) {
		writeError(w, http.StatusBadRequest, "成员称呼、角色或颜色不正确")
		return
	}
	member.Name, member.Role, member.IsAdmin, member.Color = input.Name, input.Role, input.IsAdmin, input.Color
	if err := a.store.updateMember(r.Context(), member, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法更新成员")
		return
	}
	if err := createMemberSpace(a.spacesRoot, member); err != nil {
		writeError(w, http.StatusInternalServerError, "成员已更新，但文件资料暂时无法同步")
		return
	}
	writeJSON(w, http.StatusOK, member)
}

func (a *app) adminRotateMemberToken(w http.ResponseWriter, r *http.Request) {
	member, err := a.store.getMember(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个家庭成员")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭成员")
		return
	}
	token := newAccessToken()
	if err := a.store.setMemberTokenHash(r.Context(), member.ID, hashToken(token), time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法更新成员令牌")
		return
	}
	a.invalidateMCPSessions(member.ID)
	writeJSON(w, http.StatusOK, MemberCredential{Member: member, AccessToken: token})
}

func (a *app) adminListUpdates(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	updates, err := a.store.listAllUpdates(r.Context(), familyID, strings.TrimSpace(r.URL.Query().Get("memberId")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭数据")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updates": updates})
}

func (a *app) adminUpdateVisibility(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Visibility string `json:"visibility"`
	}
	if err := readJSON(r, &input); err != nil || !validVisibility(input.Visibility) {
		writeError(w, http.StatusBadRequest, "可见范围不正确")
		return
	}
	update, err := a.store.updateVisibility(r.Context(), r.PathValue("id"), input.Visibility, time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这条动态")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法更新可见范围")
		return
	}
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		writeError(w, http.StatusInternalServerError, "可见范围已更新，但文件投影暂时无法同步")
		return
	}
	if update.Visibility == "private" {
		_ = os.Remove(filepath.Join(a.spacesRoot, "shared", "updates", update.ID+".json"))
	}
	writeJSON(w, http.StatusOK, update)
}
