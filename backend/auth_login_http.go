package main

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const memberWebSessionLifetime = 30 * 24 * time.Hour
const memberLoginWindow = 10 * time.Minute
const memberLoginMaxFailures = 8

var memberUsernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("family-daily-dummy-password"), bcrypt.DefaultCost)

type loginAttempt struct {
	Failures  int
	StartedAt time.Time
}

func normalizeMemberUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (a *app) adminSetMemberLogin(w http.ResponseWriter, r *http.Request) {
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
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "登录信息格式不正确")
		return
	}
	username := normalizeMemberUsername(input.Username)
	if !memberUsernamePattern.MatchString(username) {
		writeError(w, http.StatusBadRequest, "用户名需为 3–32 位小写字母、数字、点、下划线或连字符")
		return
	}
	if len(input.Password) < 10 || len(input.Password) > 128 {
		writeError(w, http.StatusBadRequest, "密码长度需为 10–128 个字符")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法安全保存密码")
		return
	}
	if err := a.store.saveMemberLogin(r.Context(), member.ID, username, passwordHash, time.Now().UTC()); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "这个用户名已经被使用")
			return
		}
		writeError(w, http.StatusInternalServerError, "暂时无法保存登录信息")
		return
	}
	writeJSON(w, http.StatusOK, MemberLoginStatus{MemberID: member.ID, Username: username, HasLogin: true})
}

func (a *app) memberLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "登录信息格式不正确")
		return
	}
	username := normalizeMemberUsername(input.Username)
	if !a.memberLoginAllowed(username, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "登录尝试过多，请稍后再试")
		return
	}
	member, passwordHash, err := a.store.memberAndPasswordHashByUsername(r.Context(), username)
	if err != nil {
		passwordHash = dummyPasswordHash
	}
	passwordErr := bcrypt.CompareHashAndPassword(passwordHash, []byte(input.Password))
	if err != nil || passwordErr != nil {
		a.recordMemberLoginFailure(username, time.Now().UTC())
		writeError(w, http.StatusUnauthorized, "用户名或密码不正确")
		return
	}
	a.clearMemberLoginFailures(username)
	now := time.Now().UTC()
	expiresAt := now.Add(memberWebSessionLifetime)
	token := newWebSessionToken()
	if err := a.store.createMemberWebSession(r.Context(), member.ID, hashToken(token), now, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法创建登录会话")
		return
	}
	writeJSON(w, http.StatusOK, MemberLoginCredential{Member: member, AccessToken: token, ExpiresAt: expiresAt})
}

func (a *app) memberLoginAllowed(username string, now time.Time) bool {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	attempt, ok := a.loginTries[username]
	if !ok || now.Sub(attempt.StartedAt) >= memberLoginWindow {
		delete(a.loginTries, username)
		return true
	}
	return attempt.Failures < memberLoginMaxFailures
}

func (a *app) recordMemberLoginFailure(username string, now time.Time) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	attempt, ok := a.loginTries[username]
	if !ok || now.Sub(attempt.StartedAt) >= memberLoginWindow {
		attempt = loginAttempt{StartedAt: now}
	}
	attempt.Failures++
	a.loginTries[username] = attempt
}

func (a *app) clearMemberLoginFailures(username string) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	delete(a.loginTries, username)
}

func (a *app) memberLogout(w http.ResponseWriter, r *http.Request) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if strings.HasPrefix(token, "fds_") {
		if err := a.store.revokeMemberWebSession(r.Context(), hashToken(token), time.Now().UTC()); err != nil {
			writeError(w, http.StatusInternalServerError, "暂时无法退出登录")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
