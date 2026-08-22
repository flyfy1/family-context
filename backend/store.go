package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type store struct {
	db *sql.DB
}

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrateCoreJobs(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) Close() error { return s.db.Close() }

func (s *store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = FULL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS questions (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  asked_by TEXT NOT NULL,
  asked_to TEXT NOT NULL,
  text TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS answers (
  id TEXT PRIMARY KEY,
  question_id TEXT NOT NULL UNIQUE REFERENCES questions(id) ON DELETE CASCADE,
  answered_by TEXT NOT NULL,
  audio_file TEXT NOT NULL,
  transcript TEXT NOT NULL DEFAULT '',
  ai_summary TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  shared_at TEXT
);

CREATE TABLE IF NOT EXISTS replies (
  id TEXT PRIMARY KEY,
  answer_id TEXT NOT NULL REFERENCES answers(id) ON DELETE CASCADE,
  author_id TEXT NOT NULL,
  text TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS archived_answers (
  id TEXT PRIMARY KEY,
  question_id TEXT NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
  answered_by TEXT NOT NULL,
  audio_file TEXT NOT NULL,
  transcript TEXT NOT NULL DEFAULT '',
  ai_summary TEXT NOT NULL DEFAULT '',
  original_status TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  shared_at TEXT,
  archived_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS members (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  name TEXT NOT NULL,
  role TEXT NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  color TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS updates (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  text TEXT NOT NULL,
  visibility TEXT NOT NULL,
  audio_file TEXT NOT NULL DEFAULT '',
  transcript TEXT NOT NULL DEFAULT '',
  ai_summary TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS daily_summaries (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  summary_date TEXT NOT NULL,
  content TEXT NOT NULL,
  update_count INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS member_tokens (
  member_id TEXT PRIMARY KEY REFERENCES members(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS member_settings (
  member_id TEXT PRIMARY KEY REFERENCES members(id) ON DELETE CASCADE,
  share_mode TEXT NOT NULL DEFAULT 'manual',
  share_prompt TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS media_imports (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  media_type TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  original_name TEXT NOT NULL,
  media_file TEXT NOT NULL,
  captured_at TEXT,
  device_id TEXT NOT NULL DEFAULT '',
  client_media_id TEXT NOT NULL DEFAULT '',
  sha256 TEXT NOT NULL,
  analysis_status TEXT NOT NULL,
  analysis_json TEXT NOT NULL DEFAULT '',
  analysis_error TEXT NOT NULL DEFAULT '',
  share_decision TEXT NOT NULL DEFAULT 'pending',
  update_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS bedtime_stories (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  child_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  child_name TEXT NOT NULL,
  audience_age INTEGER NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  source_update_ids_json TEXT NOT NULL,
  voice TEXT NOT NULL,
  audio_file TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_questions_created_at ON questions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_replies_answer_id ON replies(answer_id, created_at);
CREATE INDEX IF NOT EXISTS idx_archived_answers_question_id ON archived_answers(question_id, archived_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_entity ON audit_events(entity_type, entity_id, created_at);
CREATE INDEX IF NOT EXISTS idx_members_family ON members(family_id, created_at);
CREATE INDEX IF NOT EXISTS idx_updates_family_created ON updates(family_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_updates_member_created ON updates(member_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_summaries_family_date ON daily_summaries(family_id, summary_date DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_imports_member_created ON media_imports(member_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_imports_client_id ON media_imports(member_id, device_id, client_media_id) WHERE client_media_id != '';
CREATE INDEX IF NOT EXISTS idx_bedtime_stories_family_created ON bedtime_stories(family_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bedtime_stories_child_created ON bedtime_stories(child_id, created_at DESC);
`)
	if err != nil {
		return err
	}
	var hasAdminColumn int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('members') WHERE name = 'is_admin'`).Scan(&hasAdminColumn); err != nil {
		return err
	}
	if hasAdminColumn == 0 {
		_, err = s.db.ExecContext(ctx, `ALTER TABLE members ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0`)
	}
	return err
}

func (s *store) createQuestion(ctx context.Context, q Question) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO questions(id, family_id, asked_by, asked_to, text, status, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		q.ID, q.FamilyID, q.AskedBy, q.AskedTo, q.Text, q.Status, q.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "question.created", "question", q.ID, q, q.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) questionExists(ctx context.Context, id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM questions WHERE id = ?`, id).Scan(&count)
	return count == 1, err
}

func (s *store) createAnswer(ctx context.Context, a Answer, audioFile string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO answers(id, question_id, answered_by, audio_file, transcript, ai_summary, status, error_message, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.QuestionID, a.AnsweredBy, audioFile, a.Transcript, a.AISummary, a.Status, a.ErrorMessage, a.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE questions SET status = ? WHERE id = ?`, "answered", a.QuestionID); err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "answer.recorded", "answer", a.ID, map[string]any{
		"questionId": a.QuestionID, "answeredBy": a.AnsweredBy, "audioFile": audioFile, "status": a.Status,
	}, a.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) completeAnswer(ctx context.Context, a Answer) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE answers SET transcript = ?, ai_summary = ?, status = ?, error_message = ? WHERE id = ? AND status = ?`,
		a.Transcript, a.AISummary, a.Status, a.ErrorMessage, a.ID, "processing")
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	eventType := "answer.processed"
	if a.Status == "processing_failed" {
		eventType = "answer.processing_failed"
	}
	if err := appendAudit(ctx, tx, eventType, "answer", a.ID, a, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) publishAnswer(ctx context.Context, id string, sharedAt time.Time) (Answer, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Answer{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE answers SET status = ?, shared_at = ? WHERE id = ? AND status = ?`,
		"shared", sharedAt.Format(time.RFC3339Nano), id, "ready")
	if err != nil {
		return Answer{}, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return Answer{}, sql.ErrNoRows
	}
	answer, err := getAnswerWith(ctx, tx, id)
	if err != nil {
		return Answer{}, err
	}
	if err := appendAudit(ctx, tx, "answer.shared", "answer", id, answer, sharedAt); err != nil {
		return Answer{}, err
	}
	if err := tx.Commit(); err != nil {
		return Answer{}, err
	}
	return answer, nil
}

func (s *store) archiveDraftAnswer(ctx context.Context, id string, archivedAt time.Time) (Answer, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Answer{}, err
	}
	defer tx.Rollback()
	answer, err := getAnswerWith(ctx, tx, id)
	if err != nil {
		return Answer{}, err
	}
	if answer.Status == "shared" {
		return Answer{}, errors.New("shared answers cannot be archived through the draft endpoint")
	}
	var audioFile string
	if err := tx.QueryRowContext(ctx, `SELECT audio_file FROM answers WHERE id = ?`, id).Scan(&audioFile); err != nil {
		return Answer{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO archived_answers(id, question_id, answered_by, audio_file, transcript, ai_summary, original_status, error_message, created_at, shared_at, archived_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, answer.ID, answer.QuestionID, answer.AnsweredBy, audioFile, answer.Transcript,
		answer.AISummary, answer.Status, answer.ErrorMessage, answer.CreatedAt.Format(time.RFC3339Nano), nil, archivedAt.Format(time.RFC3339Nano))
	if err != nil {
		return Answer{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM answers WHERE id = ?`, id); err != nil {
		return Answer{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE questions SET status = ? WHERE id = ?`, "pending", answer.QuestionID); err != nil {
		return Answer{}, err
	}
	answer.ArchivedAt = &archivedAt
	if err := appendAudit(ctx, tx, "answer.archived_for_rerecord", "answer", id, answer, archivedAt); err != nil {
		return Answer{}, err
	}
	if err := tx.Commit(); err != nil {
		return Answer{}, err
	}
	return answer, nil
}

func (s *store) getAnswer(ctx context.Context, id string) (Answer, error) {
	return getAnswerWith(ctx, s.db, id)
}

func (s *store) createReply(ctx context.Context, reply Reply) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM answers WHERE id = ?`, reply.AnswerID).Scan(&status); err != nil {
		return err
	}
	if status != "shared" {
		return errors.New("answer is not shared")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO replies(id, answer_id, author_id, text, created_at) VALUES(?, ?, ?, ?, ?)`,
		reply.ID, reply.AnswerID, reply.AuthorID, reply.Text, reply.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "reply.created", "answer", reply.AnswerID, reply, reply.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) listQuestions(ctx context.Context) ([]Question, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, family_id, asked_by, asked_to, text, status, created_at FROM questions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	questions := make([]Question, 0)
	for rows.Next() {
		var q Question
		var createdAt string
		if err := rows.Scan(&q.ID, &q.FamilyID, &q.AskedBy, &q.AskedTo, &q.Text, &q.Status, &createdAt); err != nil {
			return nil, err
		}
		q.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		q.Replies = []Reply{}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range questions {
		answer, err := s.answerForQuestion(ctx, questions[i].ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			questions[i].Answer = &answer
			replies, err := s.repliesForAnswer(ctx, answer.ID)
			if err != nil {
				return nil, err
			}
			questions[i].Replies = replies
		}
	}
	return questions, nil
}

func (s *store) answerForQuestion(ctx context.Context, questionID string) (Answer, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, question_id, answered_by, audio_file, transcript, ai_summary, status, error_message, created_at, shared_at FROM answers WHERE question_id = ?`, questionID)
	return scanAnswer(row)
}

func (s *store) answerHistory(ctx context.Context, questionID string) (AnswerHistory, error) {
	history := AnswerHistory{Archived: []Answer{}, Events: []AuditEvent{}}
	current, err := s.answerForQuestion(ctx, questionID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return history, err
	}
	if err == nil {
		history.Current = &current
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, question_id, answered_by, audio_file, transcript, ai_summary, original_status, error_message, created_at, shared_at, archived_at FROM archived_answers WHERE question_id = ? ORDER BY archived_at DESC`, questionID)
	if err != nil {
		return history, err
	}
	for rows.Next() {
		answer, err := scanArchivedAnswer(rows)
		if err != nil {
			rows.Close()
			return history, err
		}
		history.Archived = append(history.Archived, answer)
	}
	if err := rows.Close(); err != nil {
		return history, err
	}
	eventRows, err := s.db.QueryContext(ctx, `SELECT id, event_type, entity_type, entity_id, payload_json, created_at FROM audit_events
		WHERE (entity_type = 'question' AND entity_id = ?)
		   OR (entity_type = 'answer' AND entity_id IN (
		       SELECT id FROM answers WHERE question_id = ?
		       UNION SELECT id FROM archived_answers WHERE question_id = ?
		   ))
		ORDER BY created_at`, questionID, questionID, questionID)
	if err != nil {
		return history, err
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var event AuditEvent
		var createdAt, payload string
		if err := eventRows.Scan(&event.ID, &event.EventType, &event.EntityType, &event.EntityID, &payload, &createdAt); err != nil {
			return history, err
		}
		event.Payload = json.RawMessage(payload)
		event.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return history, err
		}
		history.Events = append(history.Events, event)
	}
	return history, eventRows.Err()
}

func (s *store) repliesForAnswer(ctx context.Context, answerID string) ([]Reply, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, answer_id, author_id, text, created_at FROM replies WHERE answer_id = ? ORDER BY created_at`, answerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	replies := make([]Reply, 0)
	for rows.Next() {
		var reply Reply
		var createdAt string
		if err := rows.Scan(&reply.ID, &reply.AnswerID, &reply.AuthorID, &reply.Text, &createdAt); err != nil {
			return nil, err
		}
		reply.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		replies = append(replies, reply)
	}
	return replies, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

type queryExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type execExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func getAnswerWith(ctx context.Context, executor queryExecutor, id string) (Answer, error) {
	row := executor.QueryRowContext(ctx, `SELECT id, question_id, answered_by, audio_file, transcript, ai_summary, status, error_message, created_at, shared_at FROM answers WHERE id = ?`, id)
	return scanAnswer(row)
}

func appendAudit(ctx context.Context, executor execExecutor, eventType, entityType, entityID string, payload any, createdAt time.Time) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO audit_events(id, event_type, entity_type, entity_id, payload_json, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		newID(), eventType, entityType, entityID, string(data), createdAt.Format(time.RFC3339Nano))
	return err
}

func scanAnswer(row rowScanner) (Answer, error) {
	var answer Answer
	var audioFile, createdAt string
	var sharedAt sql.NullString
	if err := row.Scan(&answer.ID, &answer.QuestionID, &answer.AnsweredBy, &audioFile, &answer.Transcript, &answer.AISummary, &answer.Status, &answer.ErrorMessage, &createdAt, &sharedAt); err != nil {
		return Answer{}, err
	}
	answer.AudioURL = "/media/" + audioFile
	var err error
	answer.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Answer{}, fmt.Errorf("parse answer created_at: %w", err)
	}
	if sharedAt.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, sharedAt.String)
		if parseErr != nil {
			return Answer{}, fmt.Errorf("parse answer shared_at: %w", parseErr)
		}
		answer.SharedAt = &value
	}
	return answer, nil
}

func scanArchivedAnswer(row rowScanner) (Answer, error) {
	var answer Answer
	var audioFile, createdAt, archivedAt string
	var sharedAt sql.NullString
	if err := row.Scan(&answer.ID, &answer.QuestionID, &answer.AnsweredBy, &audioFile, &answer.Transcript, &answer.AISummary, &answer.Status, &answer.ErrorMessage, &createdAt, &sharedAt, &archivedAt); err != nil {
		return Answer{}, err
	}
	answer.AudioURL = "/media/" + audioFile
	var err error
	answer.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Answer{}, err
	}
	archived, err := time.Parse(time.RFC3339Nano, archivedAt)
	if err != nil {
		return Answer{}, err
	}
	answer.ArchivedAt = &archived
	if sharedAt.Valid {
		shared, err := time.Parse(time.RFC3339Nano, sharedAt.String)
		if err != nil {
			return Answer{}, err
		}
		answer.SharedAt = &shared
	}
	return answer, nil
}
