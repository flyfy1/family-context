package main

import (
	"net/http"
	"strings"
	"time"
)

const memberMCPSessionLifetime = 45 * 24 * time.Hour

type MemberMCPSessionCredential struct {
	Session     MemberMCPSession `json:"session"`
	AccessToken string           `json:"accessToken"`
	ServerURL   string           `json:"serverUrl"`
}

func (a *app) memberListMCPSessions(w http.ResponseWriter, r *http.Request) {
	a.listMCPSessions(w, r, memberFromContext(r.Context()))
}

func (a *app) memberCreateMCPSession(w http.ResponseWriter, r *http.Request) {
	a.createMCPSession(w, r, memberFromContext(r.Context()))
}

func (a *app) memberRevokeMCPSession(w http.ResponseWriter, r *http.Request) {
	a.revokeMCPSession(w, r, memberFromContext(r.Context()))
}

func (a *app) adminListMemberMCPSessions(w http.ResponseWriter, r *http.Request) {
	member, err := a.store.getMember(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "没有找到这个成员")
		return
	}
	a.listMCPSessions(w, r, member)
}

func (a *app) adminCreateMemberMCPSession(w http.ResponseWriter, r *http.Request) {
	member, err := a.store.getMember(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "没有找到这个成员")
		return
	}
	a.createMCPSession(w, r, member)
}

func (a *app) adminRevokeMemberMCPSession(w http.ResponseWriter, r *http.Request) {
	member, err := a.store.getMember(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "没有找到这个成员")
		return
	}
	a.revokeMCPSession(w, r, member)
}

func (a *app) listMCPSessions(w http.ResponseWriter, r *http.Request, member Member) {
	sessions, err := a.store.listMemberMCPSessions(r.Context(), member.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取 MCP 会话")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "serverUrl": mcpServerURL(r, member.ID)})
}

func (a *app) createMCPSession(w http.ResponseWriter, r *http.Request, member Member) {
	var input struct {
		Label string `json:"label"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "MCP 会话格式不正确")
		return
	}
	input.Label = strings.TrimSpace(input.Label)
	if input.Label == "" || len([]rune(input.Label)) > 80 {
		writeError(w, http.StatusBadRequest, "MCP 会话名称不能为空且不能超过 80 个字")
		return
	}
	now := time.Now().UTC()
	token := newMCPAccessToken()
	session := MemberMCPSession{ID: newID(), MemberID: member.ID, Label: input.Label, CreatedAt: now, ExpiresAt: now.Add(memberMCPSessionLifetime)}
	if err := a.store.createMemberMCPSession(r.Context(), session, hashToken(token)); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法创建 MCP 会话")
		return
	}
	_ = appendAudit(r.Context(), a.store.db, "mcp.access_session_created", "member", member.ID,
		map[string]any{"sessionId": session.ID, "label": session.Label, "expiresAt": session.ExpiresAt}, now)
	writeJSON(w, http.StatusCreated, MemberMCPSessionCredential{Session: session, AccessToken: token, ServerURL: mcpServerURL(r, member.ID)})
}

func (a *app) revokeMCPSession(w http.ResponseWriter, r *http.Request, member Member) {
	now := time.Now().UTC()
	revoked, err := a.store.revokeMemberMCPSession(r.Context(), member.ID, r.PathValue("sessionId"), now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法撤销 MCP 会话")
		return
	}
	if !revoked {
		writeError(w, http.StatusNotFound, errMCPSessionNotFound.Error())
		return
	}
	a.invalidateMCPSessions(member.ID)
	_ = appendAudit(r.Context(), a.store.db, "mcp.access_session_revoked", "member", member.ID,
		map[string]any{"sessionId": r.PathValue("sessionId")}, now)
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) authenticateMCPMember(r *http.Request) (Member, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return Member{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return Member{}, false
	}
	if strings.HasPrefix(token, "fdmcp_") {
		member, err := a.store.memberByMCPSessionTokenHash(r.Context(), hashToken(token), time.Now().UTC())
		return member, err == nil
	}
	return a.authenticateMember(r)
}

func newMCPAccessToken() string { return "fdmcp_" + newID() + newID() }

func mcpServerURL(r *http.Request, memberID string) string {
	base := strings.TrimRight(strings.TrimSpace(envOr("PUBLIC_BASE_URL", "")), "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		} else if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		}
		base = scheme + "://" + r.Host
	}
	return base + "/mcp/members/" + memberID
}
