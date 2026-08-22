package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const mcpProtocolVersion = "2025-11-25"

var contextFileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,119}\.(md|txt|json)$`)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (a *app) mcpEndpoint(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && !a.originAllowed(origin) {
		writeMCPError(w, nil, http.StatusForbidden, -32000, "Origin is not allowed")
		return
	}
	member, ok := a.authenticateMCPMember(r)
	if !ok || member.ID != r.PathValue("id") {
		metadataURL := publicBaseURL(r) + "/.well-known/oauth-protected-resource/mcp/members/" + r.PathValue("id")
		w.Header().Set("WWW-Authenticate", `Bearer realm="family-daily-mcp", resource_metadata="`+metadataURL+`", scope="mcp:context"`)
		writeMCPError(w, nil, http.StatusUnauthorized, -32001, "Invalid member access token")
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Allow", "POST, DELETE")
		writeMCPError(w, nil, http.StatusMethodNotAllowed, -32601, "SSE stream is not supported")
		return
	}
	if r.Method == http.MethodDelete {
		sessionID := r.Header.Get("Mcp-Session-Id")
		a.mcpMu.Lock()
		if a.mcpSessions[sessionID] == member.ID {
			delete(a.mcpSessions, sessionID)
		}
		a.mcpMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeMCPError(w, nil, http.StatusUnsupportedMediaType, -32600, "Content-Type must be application/json")
		return
	}
	var request mcpRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil || request.JSONRPC != "2.0" || request.Method == "" {
		writeMCPError(w, request.ID, http.StatusBadRequest, -32600, "Invalid JSON-RPC request")
		return
	}
	if request.Method == "initialize" {
		sessionID := newID()
		a.mcpMu.Lock()
		a.mcpSessions[sessionID] = member.ID
		a.mcpMu.Unlock()
		w.Header().Set("Mcp-Session-Id", sessionID)
		writeMCPResult(w, request.ID, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo": map[string]any{"name": "family-daily-member-space", "version": "0.2.0",
				"description": "Member-scoped local context and update tools"},
		})
		return
	}
	if !a.validMCPSession(r.Header.Get("Mcp-Session-Id"), member.ID) {
		writeMCPError(w, request.ID, http.StatusBadRequest, -32002, "Missing or invalid MCP session")
		return
	}
	if request.Method == "notifications/initialized" && len(request.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch request.Method {
	case "ping":
		writeMCPResult(w, request.ID, map[string]any{})
	case "tools/list":
		writeMCPResult(w, request.ID, map[string]any{"tools": memberMCPTools()})
	case "tools/call":
		a.callMCPTool(w, r, request, member)
	default:
		writeMCPError(w, request.ID, http.StatusOK, -32601, "Method not found")
	}
}

func (a *app) invalidateMCPSessions(memberID string) {
	a.mcpMu.Lock()
	defer a.mcpMu.Unlock()
	for sessionID, ownerID := range a.mcpSessions {
		if ownerID == memberID {
			delete(a.mcpSessions, sessionID)
		}
	}
}

func (a *app) validMCPSession(sessionID, memberID string) bool {
	if sessionID == "" {
		return false
	}
	a.mcpMu.Lock()
	defer a.mcpMu.Unlock()
	return a.mcpSessions[sessionID] == memberID
}

func memberMCPTools() []map[string]any {
	return []map[string]any{
		{"name": "list_updates", "description": "List updates in this member's private Space", "inputSchema": objectSchema(map[string]any{}, []string{})},
		{"name": "get_share_policy", "description": "Read this member's automatic sharing policy and prompt", "inputSchema": objectSchema(map[string]any{}, []string{})},
		{"name": "list_context_files", "description": "List files in this member's isolated context directory", "inputSchema": objectSchema(map[string]any{}, []string{})},
		{"name": "read_context_file", "description": "Read one UTF-8 context file owned by this member", "inputSchema": objectSchema(map[string]any{"name": map[string]any{"type": "string"}}, []string{"name"})},
		{"name": "write_context_file", "description": "Write one UTF-8 .md, .txt, or .json file in this member's context directory", "inputSchema": objectSchema(map[string]any{"name": map[string]any{"type": "string"}, "content": map[string]any{"type": "string", "maxLength": 524288}}, []string{"name", "content"})},
		{"name": "create_update", "description": "Create a text update for this member. Family visibility is allowed only when the member enabled auto sharing", "inputSchema": objectSchema(map[string]any{
			"text": map[string]any{"type": "string", "maxLength": 2000}, "visibility": map[string]any{"type": "string", "enum": []string{"private", "family"}},
		}, []string{"text", "visibility"})},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func (a *app) callMCPTool(w http.ResponseWriter, r *http.Request, request mcpRequest, member Member) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
		writeMCPError(w, request.ID, http.StatusOK, -32602, "Invalid tool parameters")
		return
	}
	var result any
	var err error
	switch params.Name {
	case "list_updates":
		result, err = a.store.listUpdates(r.Context(), member.FamilyID, member.ID, "mine")
	case "get_share_policy":
		result, err = a.store.getMemberSettings(r.Context(), member.ID)
	case "list_context_files":
		result, err = a.listMemberContextFiles(member.ID)
	case "read_context_file":
		result, err = a.readMemberContextFile(member.ID, stringArgument(params.Arguments, "name"))
	case "write_context_file":
		result, err = a.writeMemberContextFile(r, member, stringArgument(params.Arguments, "name"), stringArgument(params.Arguments, "content"))
	case "create_update":
		result, err = a.createMCPUpdate(r, member, stringArgument(params.Arguments, "text"), stringArgument(params.Arguments, "visibility"))
	default:
		writeMCPError(w, request.ID, http.StatusOK, -32602, "Unknown tool")
		return
	}
	if err != nil {
		writeMCPResult(w, request.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": err.Error()}}, "isError": true})
		return
	}
	data, _ := json.Marshal(result)
	writeMCPResult(w, request.ID, map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(data)}},
		"structuredContent": map[string]any{"result": result},
	})
}

func (a *app) listMemberContextFiles(memberID string) ([]map[string]any, error) {
	dir := filepath.Join(a.spacesRoot, "members", memberID, "context")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]map[string]any, 0)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || !contextFileNamePattern.MatchString(entry.Name()) {
			continue
		}
		files = append(files, map[string]any{"name": entry.Name(), "size": info.Size(), "modifiedAt": info.ModTime().UTC().Format(time.RFC3339)})
	}
	return files, nil
}

func (a *app) readMemberContextFile(memberID, name string) (map[string]any, error) {
	path, err := a.memberContextPath(memberID, name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 512<<10 {
		return nil, errors.New("context file exceeds 512KB")
	}
	return map[string]any{"name": name, "content": string(data)}, nil
}

func (a *app) writeMemberContextFile(r *http.Request, member Member, name, content string) (map[string]any, error) {
	path, err := a.memberContextPath(member.ID, name)
	if err != nil {
		return nil, err
	}
	if len([]byte(content)) > 512<<10 {
		return nil, errors.New("context file exceeds 512KB")
	}
	if err := writeFileAtomically(filepath.Dir(path), filepath.Base(path), []byte(content)); err != nil {
		return nil, err
	}
	_ = appendAudit(r.Context(), a.store.db, "mcp.context_file_written", "member", member.ID, map[string]any{"name": name, "size": len([]byte(content))}, time.Now().UTC())
	return map[string]any{"name": name, "size": len([]byte(content))}, nil
}

func (a *app) createMCPUpdate(r *http.Request, member Member, text, visibility string) (Update, error) {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 2000 || !validVisibility(visibility) {
		return Update{}, errors.New("text or visibility is invalid")
	}
	settings, err := a.store.getMemberSettings(r.Context(), member.ID)
	if err != nil {
		return Update{}, err
	}
	if visibility == "family" && settings.ShareMode != "auto" {
		return Update{}, errors.New("family sharing requires auto mode; create a private update for manual review")
	}
	if visibility == "family" {
		decision, err := a.sharePolicy.EvaluateShare(r.Context(), text, settings.SharePrompt)
		if err != nil {
			return Update{}, errors.New("share policy evaluation failed; content was not shared")
		}
		if !decision.Allowed {
			return Update{}, errors.New("share policy rejected this content: " + decision.Reason)
		}
	}
	update := Update{ID: newID(), FamilyID: member.FamilyID, MemberID: member.ID, Type: "text", Text: text, Visibility: visibility, Source: "member_mcp", CreatedAt: time.Now().UTC()}
	if err := persistUpdateToSpace(a.spacesRoot, update); err != nil {
		return Update{}, err
	}
	if err := a.store.createUpdate(r.Context(), update, ""); err != nil {
		return Update{}, err
	}
	return update, nil
}

func (a *app) memberContextPath(memberID, name string) (string, error) {
	if !contextFileNamePattern.MatchString(name) {
		return "", errors.New("file name must be a flat .md, .txt, or .json name")
	}
	dir := filepath.Join(a.spacesRoot, "members", memberID, "context")
	return filepath.Join(dir, name), nil
}

func stringArgument(arguments map[string]any, name string) string {
	value, _ := arguments[name].(string)
	return value
}

func writeMCPResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mcpResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeMCPError(w http.ResponseWriter, id json.RawMessage, status, code int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: code, Message: message}})
}
