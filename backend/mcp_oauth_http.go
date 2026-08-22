package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	mcpOAuthAccessLifetime  = time.Hour
	mcpOAuthRefreshLifetime = 60 * 24 * time.Hour
	mcpOAuthCodeLifetime    = 5 * time.Minute
)

var mcpOAuthConsentTemplate = template.Must(template.New("mcp-oauth-consent").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connect Family Daily</title><style>
body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f4eee7;color:#302923;font:15px system-ui,sans-serif}.card{width:min(92vw,480px);box-sizing:border-box;padding:30px;border:1px solid #dfd2c7;border-radius:20px;background:#fffdf9;box-shadow:0 18px 55px #5f46341c}h1{margin:0 0 8px;font-family:Georgia,serif;font-size:30px}p{color:#73685f;line-height:1.6}.client{padding:14px;border-radius:12px;background:#f8f2eb}.error{padding:10px 12px;border-radius:10px;color:#7d2f20;background:#f9e3da}label{display:grid;gap:7px;margin:18px 0 8px;font-weight:700}input{min-height:44px;padding:0 12px;border:1px solid #d8ccc1;border-radius:10px;font:inherit}button{width:100%;min-height:46px;margin-top:14px;border:0;border-radius:10px;color:white;background:#a94f37;font-weight:800}.boundary{font-size:12px}code{word-break:break-all}
</style></head><body><main class="card"><h1>Connect Family Daily</h1><p class="client"><strong>{{.ClientName}}</strong> wants access to one family member's private MCP context.</p>{{if .Error}}<p class="error">{{.Error}}</p>{{end}}<form method="post" action="/oauth/authorize">{{range $key, $value := .Hidden}}<input type="hidden" name="{{$key}}" value="{{$value}}">{{end}}<label>Member or MCP session token<input type="password" name="member_access_token" autocomplete="current-password" required autofocus></label><button type="submit">Allow this member's context</button></form><p class="boundary">The client can only use <code>{{.Resource}}</code>. It cannot access another member, the storage root, or a shell.</p></main></body></html>`))

func (a *app) mcpOAuthProtectedResource(w http.ResponseWriter, r *http.Request) {
	resource := publicBaseURL(r)
	path := strings.TrimPrefix(r.PathValue("path"), "/")
	if path != "" {
		resource += "/" + path
	}
	if _, ok := memberIDFromMCPResource(r, resource); path != "" && !ok {
		writeOAuthError(w, http.StatusNotFound, "invalid_target", "unknown MCP resource")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource": resource, "authorization_servers": []string{publicBaseURL(r)},
		"scopes_supported": []string{"mcp:context"}, "bearer_methods_supported": []string{"header"},
	})
}

func (a *app) mcpOAuthAuthorizationMetadata(w http.ResponseWriter, r *http.Request) {
	base := publicBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token",
		"registration_endpoint": base + "/oauth/register", "response_types_supported": []string{"code"},
		"grant_types_supported":            []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported": []string{"mcp:context", "offline_access"},
	})
}

func (a *app) mcpOAuthRegister(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		GrantTypes              []string `json:"grant_types"`
		ResponseTypes           []string `json:"response_types"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	if err := decoder.Decode(&input); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "invalid registration document")
		return
	}
	input.ClientName = strings.TrimSpace(input.ClientName)
	if input.ClientName == "" || len([]rune(input.ClientName)) > 100 || len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > 10 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client_name and redirect_uris are required")
		return
	}
	for _, redirect := range input.RedirectURIs {
		if !validOAuthRedirectURI(redirect) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect URI must be HTTPS or loopback HTTP")
			return
		}
	}
	if len(input.GrantTypes) > 0 && (!slices.Contains(input.GrantTypes, "authorization_code") || !slices.Contains(input.GrantTypes, "refresh_token")) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "authorization_code and refresh_token grants are required")
		return
	}
	if len(input.ResponseTypes) > 0 && !slices.Contains(input.ResponseTypes, "code") {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "code response type is required")
		return
	}
	if input.TokenEndpointAuthMethod != "" && input.TokenEndpointAuthMethod != "none" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "only public clients are supported")
		return
	}
	client := MCPOAuthClient{ID: "fdmcp_client_" + newID(), Name: input.ClientName, RedirectURIs: input.RedirectURIs, CreatedAt: time.Now().UTC()}
	if err := a.store.createMCPOAuthClient(r.Context(), client); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "client registration failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": client.ID, "client_id_issued_at": client.CreatedAt.Unix(), "client_name": client.Name,
		"redirect_uris": client.RedirectURIs, "grant_types": []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
}

func (a *app) mcpOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		a.renderMCPOAuthConsent(w, r, r.URL.Query(), "")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid authorization request")
		return
	}
	values := r.PostForm
	client, resourceMemberID, errMessage := a.validateMCPOAuthAuthorization(r, values)
	if errMessage != "" {
		a.renderMCPOAuthConsentWithClient(w, values, client, errMessage)
		return
	}
	token := strings.TrimSpace(values.Get("member_access_token"))
	member, err := a.memberForOAuthConsentToken(r, token)
	if err != nil || member.ID != resourceMemberID {
		a.renderMCPOAuthConsentWithClient(w, values, client, "The member token does not match this MCP endpoint.")
		return
	}
	now := time.Now().UTC()
	plainCode := "fdmcp_code_" + newID() + newID()
	code := MCPOAuthCode{ClientID: client.ID, MemberID: member.ID, RedirectURI: values.Get("redirect_uri"), CodeChallenge: values.Get("code_challenge"), Resource: values.Get("resource"), Scope: normalizeMCPOAuthScope(values.Get("scope")), ExpiresAt: now.Add(mcpOAuthCodeLifetime)}
	if err := a.store.createMCPOAuthCode(r.Context(), hashToken(plainCode), code); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "authorization failed")
		return
	}
	redirect, _ := url.Parse(code.RedirectURI)
	query := redirect.Query()
	query.Set("code", plainCode)
	if state := values.Get("state"); state != "" {
		query.Set("state", state)
	}
	redirect.RawQuery = query.Encode()
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (a *app) memberForOAuthConsentToken(r *http.Request, token string) (Member, error) {
	if strings.HasPrefix(token, "fdmcp_") {
		return a.store.memberByMCPSessionTokenHash(r.Context(), hashToken(token), time.Now().UTC())
	}
	return a.store.memberByTokenHash(r.Context(), hashToken(token))
}

func (a *app) renderMCPOAuthConsent(w http.ResponseWriter, r *http.Request, values url.Values, requestError string) {
	client, _, validationError := a.validateMCPOAuthAuthorization(r, values)
	if requestError == "" {
		requestError = validationError
	}
	a.renderMCPOAuthConsentWithClient(w, values, client, requestError)
}

func (a *app) renderMCPOAuthConsentWithClient(w http.ResponseWriter, values url.Values, client MCPOAuthClient, requestError string) {
	if requestError != "" && client.ID == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", requestError)
		return
	}
	hidden := map[string]string{}
	for _, key := range []string{"response_type", "client_id", "redirect_uri", "code_challenge", "code_challenge_method", "resource", "scope", "state"} {
		hidden[key] = values.Get(key)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	status := http.StatusOK
	if requestError != "" {
		status = http.StatusBadRequest
	}
	w.WriteHeader(status)
	_ = mcpOAuthConsentTemplate.Execute(w, map[string]any{"ClientName": client.Name, "Resource": values.Get("resource"), "Hidden": hidden, "Error": requestError})
}

func (a *app) validateMCPOAuthAuthorization(r *http.Request, values url.Values) (MCPOAuthClient, string, string) {
	if values.Get("response_type") != "code" || values.Get("code_challenge_method") != "S256" || values.Get("code_challenge") == "" {
		return MCPOAuthClient{}, "", "Authorization requires response_type=code and PKCE S256."
	}
	client, err := a.store.getMCPOAuthClient(r.Context(), values.Get("client_id"))
	if err != nil || !slices.Contains(client.RedirectURIs, values.Get("redirect_uri")) {
		return MCPOAuthClient{}, "", "Unknown client or redirect URI."
	}
	memberID, ok := memberIDFromMCPResource(r, values.Get("resource"))
	if !ok {
		return client, "", "The resource must be this server's member MCP endpoint."
	}
	if _, ok := validMCPOAuthScope(values.Get("scope")); !ok {
		return client, "", "Unsupported OAuth scope."
	}
	return client, memberID, ""
}

func (a *app) mcpOAuthToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid token request")
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		a.exchangeMCPOAuthCode(w, r)
	case "refresh_token":
		a.refreshMCPOAuthToken(w, r)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant type")
	}
}

func (a *app) exchangeMCPOAuthCode(w http.ResponseWriter, r *http.Request) {
	code, err := a.store.consumeMCPOAuthCode(r.Context(), hashToken(r.Form.Get("code")), r.Form.Get("client_id"), r.Form.Get("redirect_uri"), r.Form.Get("resource"), time.Now().UTC())
	if err != nil || !verifyPKCES256(r.Form.Get("code_verifier"), code.CodeChallenge) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid")
		return
	}
	a.issueMCPOAuthTokens(w, r, MCPOAuthRefreshGrant{ClientID: code.ClientID, MemberID: code.MemberID, Resource: code.Resource, Scope: code.Scope, ExpiresAt: time.Now().UTC().Add(mcpOAuthRefreshLifetime)})
}

func (a *app) refreshMCPOAuthToken(w http.ResponseWriter, r *http.Request) {
	newRefresh := "fdmcp_refresh_" + newID() + newID()
	grant, err := a.store.rotateMCPOAuthRefreshToken(r.Context(), hashToken(r.Form.Get("refresh_token")), hashToken(newRefresh), r.Form.Get("client_id"), r.Form.Get("resource"), time.Now().UTC())
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid")
		return
	}
	a.issueMCPOAuthAccessToken(w, r, grant, newRefresh)
}

func (a *app) issueMCPOAuthTokens(w http.ResponseWriter, r *http.Request, grant MCPOAuthRefreshGrant) {
	refresh := ""
	if strings.Contains(" "+grant.Scope+" ", " offline_access ") {
		refresh = "fdmcp_refresh_" + newID() + newID()
		if err := a.store.createMCPOAuthRefreshToken(r.Context(), hashToken(refresh), grant); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "token issuance failed")
			return
		}
	}
	a.issueMCPOAuthAccessToken(w, r, grant, refresh)
}

func (a *app) issueMCPOAuthAccessToken(w http.ResponseWriter, r *http.Request, grant MCPOAuthRefreshGrant, refresh string) {
	now := time.Now().UTC()
	client, _ := a.store.getMCPOAuthClient(r.Context(), grant.ClientID)
	access := newMCPAccessToken()
	session := MemberMCPSession{ID: newID(), MemberID: grant.MemberID, Label: "OAuth · " + client.Name, CreatedAt: now, ExpiresAt: now.Add(mcpOAuthAccessLifetime)}
	if err := a.store.createMemberMCPSession(r.Context(), session, hashToken(access)); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "access token issuance failed")
		return
	}
	response := map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(mcpOAuthAccessLifetime.Seconds()), "scope": grant.Scope}
	if refresh != "" {
		response["refresh_token"] = refresh
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

func memberIDFromMCPResource(r *http.Request, resource string) (string, bool) {
	parsed, err := url.Parse(resource)
	if err != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return "", false
	}
	base, _ := url.Parse(publicBaseURL(r))
	if !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) || !strings.HasPrefix(parsed.Path, "/mcp/members/") {
		return "", false
	}
	memberID := strings.TrimPrefix(parsed.Path, "/mcp/members/")
	return memberID, memberID != "" && !strings.Contains(memberID, "/")
}

func validOAuthRedirectURI(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Fragment != "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	host := parsed.Hostname()
	return parsed.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback())
}

func validProductionPublicBaseURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && (parsed.Path == "" || parsed.Path == "/")
}

func validMCPOAuthScope(scope string) (string, bool) {
	if strings.TrimSpace(scope) == "" {
		return "mcp:context", true
	}
	seen := map[string]bool{}
	for _, item := range strings.Fields(scope) {
		if item != "mcp:context" && item != "offline_access" {
			return "", false
		}
		seen[item] = true
	}
	items := []string{"mcp:context"}
	if seen["offline_access"] {
		items = append(items, "offline_access")
	}
	return strings.Join(items, " "), true
}

func normalizeMCPOAuthScope(scope string) string {
	normalized, _ := validMCPOAuthScope(scope)
	return normalized
}

func verifyPKCES256(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return secureEqual(base64.RawURLEncoding.EncodeToString(sum[:]), challenge)
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": description})
}
