package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ActivityThread struct {
	ID              string         `json:"id"`
	FamilyID        string         `json:"familyId"`
	ScheduledJobID  string         `json:"scheduledJobId,omitempty"`
	Title           string         `json:"title"`
	Topic           string         `json:"topic"`
	CreatorMemberID string         `json:"creatorMemberId,omitempty"`
	MemberIDs       []string       `json:"memberIds"`
	Posts           []ActivityPost `json:"posts"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type ActivityPost struct {
	ID         string    `json:"id"`
	ThreadID   string    `json:"threadId"`
	MemberID   string    `json:"memberId"`
	MemberName string    `json:"memberName"`
	Type       string    `json:"type"`
	Text       string    `json:"text,omitempty"`
	MediaURL   string    `json:"mediaUrl,omitempty"`
	MimeType   string    `json:"mimeType,omitempty"`
	Transcript string    `json:"transcript,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (s *store) migrateActivityThreads(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS activity_threads (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  scheduled_job_id TEXT UNIQUE REFERENCES scheduled_jobs(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  topic TEXT NOT NULL,
  creator_member_id TEXT REFERENCES members(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS activity_thread_members (
  thread_id TEXT NOT NULL REFERENCES activity_threads(id) ON DELETE CASCADE,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  PRIMARY KEY(thread_id, member_id)
);
CREATE TABLE IF NOT EXISTS activity_posts (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL REFERENCES activity_threads(id) ON DELETE CASCADE,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  post_type TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  media_file TEXT NOT NULL DEFAULT '',
  mime_type TEXT NOT NULL DEFAULT '',
  transcript TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_activity_thread_member ON activity_thread_members(member_id, thread_id);
CREATE INDEX IF NOT EXISTS idx_activity_posts_thread ON activity_posts(thread_id, created_at);
`)
	if err != nil {
		return err
	}
	var hasTranscript int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('activity_posts') WHERE name = 'transcript'`).Scan(&hasTranscript); err != nil {
		return err
	}
	if hasTranscript == 0 {
		_, err = s.db.ExecContext(ctx, `ALTER TABLE activity_posts ADD COLUMN transcript TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

func (s *store) activityParticipants(ctx context.Context, job ScheduledJob) ([]string, error) {
	if len(job.MemberIDs) > 0 {
		return job.MemberIDs, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM members WHERE family_id = ? ORDER BY created_at`, job.FamilyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *store) ensureScheduledActivityThread(ctx context.Context, job ScheduledJob, now time.Time) (bool, error) {
	participants, err := s.activityParticipants(ctx, job)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	threadID := newID()
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO activity_threads(id, family_id, scheduled_job_id, title, topic, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		threadID, job.FamilyID, job.ID, job.Title, job.Topic, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	created, _ := result.RowsAffected()
	if created == 0 {
		return false, nil
	}
	for _, memberID := range participants {
		if _, err := tx.ExecContext(ctx, `INSERT INTO activity_thread_members(thread_id, member_id) VALUES(?, ?)`, threadID, memberID); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func (s *store) createActivityThread(ctx context.Context, familyID, creatorID, title, topic string, memberIDs []string, now time.Time) (ActivityThread, error) {
	if strings.TrimSpace(title) == "" || len([]rune(title)) > 80 || strings.TrimSpace(topic) == "" || len([]rune(topic)) > 300 || len(memberIDs) == 0 {
		return ActivityThread{}, errors.New("invalid activity thread")
	}
	if err := s.validateFamilyMemberIDs(ctx, familyID, memberIDs); err != nil {
		return ActivityThread{}, err
	}
	foundCreator := false
	for _, id := range memberIDs {
		if id == creatorID {
			foundCreator = true
		}
	}
	if !foundCreator {
		return ActivityThread{}, errors.New("creator must participate")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ActivityThread{}, err
	}
	defer tx.Rollback()
	id := newID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO activity_threads(id, family_id, title, topic, creator_member_id, created_at) VALUES(?, ?, ?, ?, ?, ?)`, id, familyID, strings.TrimSpace(title), strings.TrimSpace(topic), creatorID, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return ActivityThread{}, err
	}
	for _, memberID := range memberIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO activity_thread_members(thread_id, member_id) VALUES(?, ?)`, id, memberID); err != nil {
			return ActivityThread{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ActivityThread{}, err
	}
	return s.getActivityThread(ctx, id, creatorID)
}

func (s *store) listActivityThreads(ctx context.Context, memberID string) ([]ActivityThread, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id FROM activity_threads t JOIN activity_thread_members tm ON tm.thread_id = t.id WHERE tm.member_id = ? ORDER BY t.created_at DESC`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	threads := make([]ActivityThread, 0, len(ids))
	for _, id := range ids {
		thread, err := s.getActivityThread(ctx, id, memberID)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, nil
}

func (s *store) getActivityThread(ctx context.Context, id, memberID string) (ActivityThread, error) {
	var thread ActivityThread
	var scheduledJobID, creatorID sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT t.id, t.family_id, t.scheduled_job_id, t.title, t.topic, t.creator_member_id, t.created_at
		FROM activity_threads t JOIN activity_thread_members tm ON tm.thread_id = t.id WHERE t.id = ? AND tm.member_id = ?`, id, memberID).
		Scan(&thread.ID, &thread.FamilyID, &scheduledJobID, &thread.Title, &thread.Topic, &creatorID, &createdAt)
	if err != nil {
		return ActivityThread{}, err
	}
	thread.ScheduledJobID, thread.CreatorMemberID = scheduledJobID.String, creatorID.String
	thread.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ActivityThread{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT member_id FROM activity_thread_members WHERE thread_id = ? ORDER BY rowid`, id)
	if err != nil {
		return ActivityThread{}, err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return ActivityThread{}, err
		}
		thread.MemberIDs = append(thread.MemberIDs, value)
	}
	if err := rows.Close(); err != nil {
		return ActivityThread{}, err
	}
	postRows, err := s.db.QueryContext(ctx, `SELECT p.id, p.thread_id, p.member_id, m.name, p.post_type, p.body, p.media_file, p.mime_type, p.transcript, p.created_at
		FROM activity_posts p JOIN members m ON m.id = p.member_id WHERE p.thread_id = ? ORDER BY p.created_at`, id)
	if err != nil {
		return ActivityThread{}, err
	}
	defer postRows.Close()
	thread.Posts = make([]ActivityPost, 0)
	for postRows.Next() {
		var post ActivityPost
		var mediaFile, postAt string
		if err := postRows.Scan(&post.ID, &post.ThreadID, &post.MemberID, &post.MemberName, &post.Type, &post.Text, &mediaFile, &post.MimeType, &post.Transcript, &postAt); err != nil {
			return ActivityThread{}, err
		}
		post.CreatedAt, err = time.Parse(time.RFC3339Nano, postAt)
		if err != nil {
			return ActivityThread{}, err
		}
		if mediaFile != "" {
			post.MediaURL = "/api/v1/me/activity-threads/" + id + "/posts/" + post.ID + "/media"
		}
		thread.Posts = append(thread.Posts, post)
	}
	return thread, postRows.Err()
}

func (s *store) createActivityPost(ctx context.Context, threadID, memberID, postType, body, mediaFile, mimeType, transcript string, now time.Time) (ActivityPost, error) {
	if _, err := s.getActivityThread(ctx, threadID, memberID); err != nil {
		return ActivityPost{}, err
	}
	body = strings.TrimSpace(body)
	if len([]rune(body)) > 2000 || (body == "" && mediaFile == "") {
		return ActivityPost{}, errors.New("empty activity post")
	}
	id := newID()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO activity_posts(id, thread_id, member_id, post_type, body, media_file, mime_type, transcript, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, threadID, memberID, postType, body, mediaFile, mimeType, transcript, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return ActivityPost{}, err
	}
	thread, err := s.getActivityThread(ctx, threadID, memberID)
	if err != nil {
		return ActivityPost{}, err
	}
	for _, post := range thread.Posts {
		if post.ID == id {
			return post, nil
		}
	}
	return ActivityPost{}, sql.ErrNoRows
}

func (a *app) memberListActivityThreads(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	threads, err := a.store.listActivityThreads(r.Context(), member.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭活动")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": threads})
}

func (a *app) memberCreateActivityThread(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	var input struct {
		Title     string   `json:"title"`
		Topic     string   `json:"topic"`
		MemberIDs []string `json:"memberIds"`
	}
	if readJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "活动格式不正确")
		return
	}
	thread, err := a.store.createActivityThread(r.Context(), member.FamilyID, member.ID, input.Title, input.Topic, input.MemberIDs, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "活动成员或内容不正确")
		return
	}
	writeJSON(w, http.StatusCreated, thread)
}

func (a *app) memberCreateActivityTextPost(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	var input struct {
		Text string `json:"text"`
	}
	if readJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "内容格式不正确")
		return
	}
	post, err := a.store.createActivityPost(r.Context(), r.PathValue("id"), member.ID, "text", input.Text, "", "", "", time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个活动")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "内容不能为空")
		return
	}
	writeJSON(w, http.StatusCreated, post)
}

func activityMedia(file multipart.File, header *multipart.FileHeader) ([]byte, string, string, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxMediaImportBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxMediaImportBytes {
		return nil, "", "", errors.New("invalid media")
	}
	mimeType := http.DetectContentType(data)
	ext := ""
	switch mimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	case "video/mp4":
		ext = ".mp4"
	case "video/webm":
		ext = ".webm"
	case "video/quicktime":
		ext = ".mov"
	default:
		if strings.HasSuffix(strings.ToLower(header.Filename), ".mov") {
			mimeType, ext = "video/quicktime", ".mov"
		} else {
			return nil, "", "", errors.New("unsupported media")
		}
	}
	return data, mimeType, ext, nil
}

func (a *app) memberCreateActivityMediaPost(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	if err := r.ParseMultipartForm(maxMediaImportBytes); err != nil {
		writeError(w, http.StatusBadRequest, "媒体格式不正确")
		return
	}
	file, header, err := r.FormFile("media")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择图片或视频")
		return
	}
	defer file.Close()
	data, mimeType, ext, err := activityMedia(file, header)
	if err != nil {
		writeError(w, http.StatusBadRequest, "仅支持常见图片和视频，文件不能超过 100MB")
		return
	}
	threadID, postID := r.PathValue("id"), newID()
	if _, err := a.store.getActivityThread(r.Context(), threadID, member.ID); err != nil {
		writeError(w, http.StatusNotFound, "没有找到这个活动")
		return
	}
	fileName := postID + ext
	dir := filepath.Join(a.spacesRoot, "members", member.ID, "activities", threadID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法准备活动空间")
		return
	}
	if err := writeFileAtomically(dir, fileName, data); err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存媒体")
		return
	}
	postType := "image"
	transcript := ""
	if strings.HasPrefix(mimeType, "video/") {
		postType = "video"
		if transcriber, ok := a.ai.(mediaTranscriber); ok && len(data) <= maxInlineAnalysisBytes {
			if value, transcribeErr := transcriber.Transcribe(r.Context(), data, mimeType); transcribeErr == nil {
				transcript = value
			} else {
				log.Printf("video transcription failed for activity post %s: %v", postID, transcribeErr)
			}
		}
	}
	post, err := a.store.createActivityPost(r.Context(), threadID, member.ID, postType, r.FormValue("text"), fileName, mimeType, transcript, time.Now().UTC())
	if err != nil {
		_ = os.Remove(filepath.Join(dir, fileName))
		writeError(w, http.StatusBadRequest, "暂时无法发布内容")
		return
	}
	writeJSON(w, http.StatusCreated, post)
}

func (a *app) memberServeActivityMedia(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	threadID, postID := r.PathValue("id"), r.PathValue("postId")
	if _, err := a.store.getActivityThread(r.Context(), threadID, member.ID); err != nil {
		writeError(w, http.StatusNotFound, "没有找到这个活动")
		return
	}
	var ownerID, fileName string
	err := a.store.db.QueryRowContext(r.Context(), `SELECT member_id, media_file FROM activity_posts WHERE id = ? AND thread_id = ? AND media_file != ''`, postID, threadID).Scan(&ownerID, &fileName)
	if err != nil || filepath.Base(fileName) != fileName {
		writeError(w, http.StatusNotFound, "没有找到这个文件")
		return
	}
	http.ServeFile(w, r, filepath.Join(a.spacesRoot, "members", ownerID, "activities", threadID, fileName))
}
