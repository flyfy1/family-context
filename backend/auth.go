package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

type memberContextKey struct{}

func (a *app) adminAuthorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !secureEqual(r.Header.Get("X-Admin-Token"), a.adminToken) {
			writeError(w, http.StatusUnauthorized, "需要家庭管理员权限")
			return
		}
		next(w, r)
	}
}

func (a *app) memberAuthorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member, ok := a.authenticateMember(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="family-daily-member"`)
			writeError(w, http.StatusUnauthorized, "成员访问令牌无效")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), memberContextKey{}, member)))
	}
}

func (a *app) authenticateMember(r *http.Request) (Member, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return Member{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return Member{}, false
	}
	tokenHash := hashToken(token)
	if strings.HasPrefix(token, "fds_") {
		member, err := a.store.memberByWebSessionTokenHash(r.Context(), tokenHash, time.Now().UTC())
		return member, err == nil
	}
	member, err := a.store.memberByTokenHash(r.Context(), tokenHash)
	return member, err == nil
}

func memberFromContext(ctx context.Context) Member {
	member, _ := ctx.Value(memberContextKey{}).(Member)
	return member
}

func newAccessToken() string { return "fdm_" + newID() + newID() }

func newWebSessionToken() string { return "fds_" + newID() + newID() }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func secureEqual(left, right string) bool {
	if len(left) != len(right) || left == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
