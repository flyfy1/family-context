package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxAudioBytes = 18 << 20

//go:embed web
var embeddedWeb embed.FS

type app struct {
	store      *store
	ai         audioProcessor
	summarizer dailySummarizer
	mediaDir   string
	spacesRoot string
	apiToken   string
}

func newApp(store *store, ai audioProcessor, mediaDir, apiToken string) *app {
	spacesRoot, err := prepareSpacesRoot(filepath.Dir(mediaDir))
	if err != nil {
		panic(err)
	}
	summarizer, ok := ai.(dailySummarizer)
	if !ok {
		summarizer = stubAudioProcessor{}
	}
	return &app{store: store, ai: ai, summarizer: summarizer, mediaDir: mediaDir, spacesRoot: spacesRoot, apiToken: apiToken}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.Handle("GET /media/", a.authorize(http.StripPrefix("/media/", http.FileServer(http.Dir(a.mediaDir)))))
	mux.HandleFunc("GET /api/v1/questions", a.authorized(a.listQuestions))
	mux.HandleFunc("POST /api/v1/questions", a.authorized(a.createQuestion))
	mux.HandleFunc("GET /api/v1/questions/{id}/history", a.authorized(a.answerHistory))
	mux.HandleFunc("POST /api/v1/questions/{id}/answer", a.authorized(a.createAnswer))
	mux.HandleFunc("POST /api/v1/answers/{id}/publish", a.authorized(a.publishAnswer))
	mux.HandleFunc("POST /api/v1/answers/{id}/archive", a.authorized(a.archiveDraftAnswer))
	mux.HandleFunc("POST /api/v1/answers/{id}/replies", a.authorized(a.createReply))
	mux.HandleFunc("GET /api/v1/members", a.authorized(a.listMembers))
	mux.HandleFunc("POST /api/v1/members", a.authorized(a.createMember))
	mux.HandleFunc("GET /api/v1/updates", a.authorized(a.listUpdates))
	mux.HandleFunc("POST /api/v1/updates", a.authorized(a.createTextUpdate))
	mux.HandleFunc("POST /api/v1/updates/voice", a.authorized(a.createVoiceUpdate))
	mux.HandleFunc("GET /api/v1/daily-summaries/latest", a.authorized(a.latestDailySummary))
	mux.HandleFunc("POST /api/v1/daily-summaries/generate", a.authorized(a.generateDailySummary))
	a.registerJudgmentRoutes(mux)
	mux.HandleFunc("GET /space-files/{path...}", a.authorized(a.serveSpaceFile))
	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(webRoot)))
	return a.cors(a.recoverPanic(a.logRequests(mux)))
}

func (a *app) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && a.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Family-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) originAllowed(origin string) bool {
	for _, allowed := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	if !strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		return origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173"
	}
	return false
}

func (a *app) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Family-Token") != a.apiToken {
			writeError(w, http.StatusUnauthorized, "无权访问这个家庭")
			return
		}
		next(w, r)
	}
}

func (a *app) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Family-Token") != a.apiToken {
			writeError(w, http.StatusUnauthorized, "无权访问这个家庭")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) createQuestion(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FamilyID string `json:"familyId"`
		AskedBy  string `json:"askedBy"`
		AskedTo  string `json:"askedTo"`
		Text     string `json:"text"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "问题格式不正确")
		return
	}
	input.FamilyID = strings.TrimSpace(input.FamilyID)
	input.AskedBy = strings.TrimSpace(input.AskedBy)
	input.AskedTo = strings.TrimSpace(input.AskedTo)
	input.Text = strings.TrimSpace(input.Text)
	if input.FamilyID == "" || input.AskedBy == "" || input.AskedTo == "" || input.Text == "" {
		writeError(w, http.StatusBadRequest, "家庭、提问人、回答人和问题不能为空")
		return
	}
	if len([]rune(input.Text)) > 300 {
		writeError(w, http.StatusBadRequest, "问题不能超过 300 个字")
		return
	}
	question := Question{
		ID: newID(), FamilyID: input.FamilyID, AskedBy: input.AskedBy,
		AskedTo: input.AskedTo, Text: input.Text, Status: "pending",
		CreatedAt: time.Now().UTC(), Replies: []Reply{},
	}
	if err := a.store.createQuestion(r.Context(), question); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存问题")
		return
	}
	writeJSON(w, http.StatusCreated, question)
}

func (a *app) listQuestions(w http.ResponseWriter, r *http.Request) {
	questions, err := a.store.listQuestions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭动态")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"questions": questions})
}

func (a *app) createAnswer(w http.ResponseWriter, r *http.Request) {
	questionID := r.PathValue("id")
	exists, err := a.store.questionExists(r.Context(), questionID)
	if err != nil || !exists {
		writeError(w, http.StatusNotFound, "没有找到这个问题")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAudioBytes); err != nil {
		writeError(w, http.StatusBadRequest, "录音过大或上传格式不正确")
		return
	}
	answeredBy := strings.TrimSpace(r.FormValue("answeredBy"))
	if answeredBy == "" {
		writeError(w, http.StatusBadRequest, "回答人不能为空")
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
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(audio)
	}
	if mimeType == "audio/x-m4a" {
		mimeType = "audio/mp4"
	}
	if !strings.HasPrefix(mimeType, "audio/") {
		writeError(w, http.StatusBadRequest, "只支持音频文件")
		return
	}

	answerID := newID()
	extension := extensionForMime(mimeType)
	fileName := answerID + extension
	if err := writeFileAtomically(a.mediaDir, fileName, audio); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存录音")
		return
	}

	answer := Answer{
		ID: answerID, QuestionID: questionID, AnsweredBy: answeredBy,
		AudioURL: "/media/" + fileName, Status: "processing", CreatedAt: time.Now().UTC(),
	}
	if err := a.store.createAnswer(r.Context(), answer, fileName); err != nil {
		_ = os.Remove(filepath.Join(a.mediaDir, fileName))
		writeError(w, http.StatusConflict, "这个问题已经有回答")
		return
	}
	result, processErr := a.ai.Process(r.Context(), audio, mimeType)
	if processErr != nil {
		answer.Status = "processing_failed"
		answer.ErrorMessage = "AI 暂时没有完成整理，原始录音已经安全保存"
	} else {
		answer.Transcript = result.Transcript
		answer.AISummary = result.Summary
		answer.Status = "ready"
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.store.completeAnswer(persistCtx, answer); err != nil {
		writeError(w, http.StatusInternalServerError, "录音已保存，但处理状态暂时无法更新")
		return
	}
	if processErr != nil {
		log.Printf("audio processing failed for answer %s: %v", answer.ID, processErr)
		writeJSON(w, http.StatusBadGateway, answer)
		return
	}
	writeJSON(w, http.StatusCreated, answer)
}

func (a *app) publishAnswer(w http.ResponseWriter, r *http.Request) {
	answer, err := a.store.publishAnswer(r.Context(), r.PathValue("id"), time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusConflict, "回答尚未整理完成或已经分享")
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

func (a *app) archiveDraftAnswer(w http.ResponseWriter, r *http.Request) {
	_, err := a.store.archiveDraftAnswer(r.Context(), r.PathValue("id"), time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusConflict, "只能归档尚未分享的回答")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) answerHistory(w http.ResponseWriter, r *http.Request) {
	history, err := a.store.answerHistory(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取本地历史")
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (a *app) createReply(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AuthorID string `json:"authorId"`
		Text     string `json:"text"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "回复格式不正确")
		return
	}
	input.AuthorID = strings.TrimSpace(input.AuthorID)
	input.Text = strings.TrimSpace(input.Text)
	if input.AuthorID == "" || input.Text == "" || len([]rune(input.Text)) > 500 {
		writeError(w, http.StatusBadRequest, "回复不能为空且不能超过 500 个字")
		return
	}
	reply := Reply{ID: newID(), AnswerID: r.PathValue("id"), AuthorID: input.AuthorID, Text: input.Text, CreatedAt: time.Now().UTC()}
	if err := a.store.createReply(r.Context(), reply); err != nil {
		writeError(w, http.StatusConflict, "只能回复已经分享的回答")
		return
	}
	writeJSON(w, http.StatusCreated, reply)
}

func (a *app) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}

func (a *app) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic: %v", recovered)
				writeError(w, http.StatusInternalServerError, "服务暂时不可用")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func readJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}

func extensionForMime(mimeType string) string {
	if extensions, _ := mime.ExtensionsByType(mimeType); len(extensions) > 0 {
		return extensions[0]
	}
	switch mimeType {
	case "audio/mp4", "audio/aac":
		return ".m4a"
	default:
		return ".audio"
	}
}
