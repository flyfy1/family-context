package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type JudgmentPrompt struct {
	ID          string    `json:"id"`
	MemberID    string    `json:"memberId"`
	Name        string    `json:"name"`
	Instruction string    `json:"instruction"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type JudgmentEvaluation struct {
	ID             string    `json:"id"`
	UpdateID       string    `json:"updateId"`
	PromptID       string    `json:"promptId"`
	MemberID       string    `json:"memberId"`
	PromptSnapshot string    `json:"promptSnapshot"`
	Model          string    `json:"model"`
	Decision       string    `json:"decision"`
	OrganizedText  string    `json:"organizedText"`
	Reason         string    `json:"reason"`
	SensitiveFlags []string  `json:"sensitiveFlags"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (s *store) ensureJudgmentSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS judgment_prompts (
  id TEXT PRIMARY KEY,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  instruction TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS judgment_evaluations (
  id TEXT PRIMARY KEY,
  update_id TEXT NOT NULL REFERENCES updates(id) ON DELETE CASCADE,
  prompt_id TEXT NOT NULL REFERENCES judgment_prompts(id) ON DELETE RESTRICT,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  prompt_snapshot TEXT NOT NULL,
  model TEXT NOT NULL,
  decision TEXT NOT NULL,
  organized_text TEXT NOT NULL,
  reason TEXT NOT NULL,
  sensitive_flags_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_judgment_prompts_member ON judgment_prompts(member_id, created_at);
CREATE INDEX IF NOT EXISTS idx_judgment_evaluations_update ON judgment_evaluations(update_id, created_at DESC);
`)
	return err
}

func (s *store) createJudgmentPrompt(ctx context.Context, prompt JudgmentPrompt) error {
	if err := s.ensureJudgmentSchema(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO judgment_prompts(id, member_id, name, instruction, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`,
		prompt.ID, prompt.MemberID, prompt.Name, prompt.Instruction, prompt.CreatedAt.Format(time.RFC3339Nano), prompt.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "judgment_prompt.created", "judgment_prompt", prompt.ID, map[string]any{
		"memberId": prompt.MemberID, "name": prompt.Name,
	}, prompt.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) listJudgmentPrompts(ctx context.Context, memberID string) ([]JudgmentPrompt, error) {
	if err := s.ensureJudgmentSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, member_id, name, instruction, created_at, updated_at FROM judgment_prompts WHERE member_id = ? ORDER BY created_at`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prompts := make([]JudgmentPrompt, 0)
	for rows.Next() {
		prompt, err := scanJudgmentPrompt(rows)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, prompt)
	}
	return prompts, rows.Err()
}

func (s *store) getJudgmentPrompt(ctx context.Context, id, memberID string) (JudgmentPrompt, error) {
	if err := s.ensureJudgmentSchema(ctx); err != nil {
		return JudgmentPrompt{}, err
	}
	return scanJudgmentPrompt(s.db.QueryRowContext(ctx, `SELECT id, member_id, name, instruction, created_at, updated_at FROM judgment_prompts WHERE id = ? AND member_id = ?`, id, memberID))
}

func scanJudgmentPrompt(row rowScanner) (JudgmentPrompt, error) {
	var prompt JudgmentPrompt
	var createdAt, updatedAt string
	if err := row.Scan(&prompt.ID, &prompt.MemberID, &prompt.Name, &prompt.Instruction, &createdAt, &updatedAt); err != nil {
		return JudgmentPrompt{}, err
	}
	var err error
	prompt.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return JudgmentPrompt{}, err
	}
	prompt.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	return prompt, err
}

func (s *store) createJudgmentEvaluation(ctx context.Context, evaluation JudgmentEvaluation) error {
	if err := s.ensureJudgmentSchema(ctx); err != nil {
		return err
	}
	flags, err := json.Marshal(evaluation.SensitiveFlags)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO judgment_evaluations(id, update_id, prompt_id, member_id, prompt_snapshot, model, decision, organized_text, reason, sensitive_flags_json, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, evaluation.ID, evaluation.UpdateID, evaluation.PromptID, evaluation.MemberID,
		evaluation.PromptSnapshot, evaluation.Model, evaluation.Decision, evaluation.OrganizedText, evaluation.Reason, string(flags), evaluation.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, "judgment.completed", "update", evaluation.UpdateID, map[string]any{
		"evaluationId": evaluation.ID, "promptId": evaluation.PromptID, "decision": evaluation.Decision,
	}, evaluation.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) judgmentEvaluationForUpdate(ctx context.Context, updateID, memberID string) (JudgmentEvaluation, error) {
	if err := s.ensureJudgmentSchema(ctx); err != nil {
		return JudgmentEvaluation{}, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, update_id, prompt_id, member_id, prompt_snapshot, model, decision, organized_text, reason, sensitive_flags_json, created_at
		FROM judgment_evaluations WHERE update_id = ? AND member_id = ? ORDER BY created_at DESC LIMIT 1`, updateID, memberID)
	var evaluation JudgmentEvaluation
	var flags, createdAt string
	if err := row.Scan(&evaluation.ID, &evaluation.UpdateID, &evaluation.PromptID, &evaluation.MemberID, &evaluation.PromptSnapshot,
		&evaluation.Model, &evaluation.Decision, &evaluation.OrganizedText, &evaluation.Reason, &flags, &createdAt); err != nil {
		return JudgmentEvaluation{}, err
	}
	if err := json.Unmarshal([]byte(flags), &evaluation.SensitiveFlags); err != nil {
		return JudgmentEvaluation{}, err
	}
	var err error
	evaluation.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return evaluation, err
}

func (s *store) updateForMember(ctx context.Context, updateID, memberID string) (Update, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at
		FROM updates WHERE id = ? AND member_id = ?`, updateID, memberID)
	return scanUpdate(row)
}

func (s *store) judgmentMemberByID(ctx context.Context, memberID string) (Member, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, family_id, name, role, is_admin, needs_attention, color, created_at FROM members WHERE id = ?`, memberID)
	var member Member
	var createdAt string
	if err := row.Scan(&member.ID, &member.FamilyID, &member.Name, &member.Role, &member.IsAdmin, &member.NeedsAttention, &member.Color, &createdAt); err != nil {
		return Member{}, err
	}
	var err error
	member.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return member, err
}

func (s *store) shareJudgedUpdate(ctx context.Context, updateID, memberID string, changedAt time.Time) (Update, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Update{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE updates SET visibility = 'family' WHERE id = ? AND member_id = ? AND visibility = 'private'`, updateID, memberID)
	if err != nil {
		return Update{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return Update{}, sql.ErrNoRows
	}
	row := tx.QueryRowContext(ctx, `SELECT id, family_id, member_id, type, text, visibility, audio_file, transcript, ai_summary, source, created_at FROM updates WHERE id = ?`, updateID)
	update, err := scanUpdate(row)
	if err != nil {
		return Update{}, err
	}
	if err := appendAudit(ctx, tx, "thought.shared_after_judgment", "update", updateID, map[string]any{
		"memberId": memberID, "visibility": "family",
	}, changedAt); err != nil {
		return Update{}, err
	}
	if err := tx.Commit(); err != nil {
		return Update{}, err
	}
	return update, nil
}

func isNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }
