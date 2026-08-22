package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type CoreJobRule struct {
	ID                 string    `json:"id"`
	FamilyID           string    `json:"familyId"`
	TargetMemberID     string    `json:"targetMemberId"`
	TargetMemberName   string    `json:"targetMemberName"`
	Enabled            bool      `json:"enabled"`
	IncludeTarget      bool      `json:"includeTarget"`
	RecipientMemberIDs []string  `json:"recipientMemberIds,omitempty"`
	InactivityHours    int       `json:"inactivityHours"`
	ReminderText       string    `json:"reminderText"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type Notification struct {
	ID                string     `json:"id"`
	FamilyID          string     `json:"familyId"`
	RecipientMemberID string     `json:"recipientMemberId"`
	SubjectMemberID   string     `json:"subjectMemberId"`
	SubjectMemberName string     `json:"subjectMemberName"`
	Type              string     `json:"type"`
	Message           string     `json:"message"`
	CreatedAt         time.Time  `json:"createdAt"`
	ReadAt            *time.Time `json:"readAt,omitempty"`
}

type CoreJobRunResult struct {
	RulesEvaluated       int `json:"rulesEvaluated"`
	AnomaliesDetected    int `json:"anomaliesDetected"`
	JobsTriggered        int `json:"jobsTriggered"`
	NotificationsCreated int `json:"notificationsCreated"`
}

func (s *store) migrateCoreJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS core_job_rules (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  target_member_id TEXT NOT NULL UNIQUE REFERENCES members(id) ON DELETE CASCADE,
  enabled INTEGER NOT NULL DEFAULT 0,
  include_target INTEGER NOT NULL DEFAULT 0,
  inactivity_hours INTEGER NOT NULL DEFAULT 24,
  reminder_text TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notifications (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  recipient_member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  subject_member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  message TEXT NOT NULL,
  incident_key TEXT NOT NULL,
  created_at TEXT NOT NULL,
  read_at TEXT,
  UNIQUE(recipient_member_id, incident_key)
);

CREATE INDEX IF NOT EXISTS idx_core_job_rules_family ON core_job_rules(family_id, enabled);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications(recipient_member_id, created_at DESC);

CREATE TABLE IF NOT EXISTS core_job_rule_recipients (
  rule_id TEXT NOT NULL REFERENCES core_job_rules(id) ON DELETE CASCADE,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  PRIMARY KEY(rule_id, member_id)
);
`)
	if err != nil {
		return err
	}
	if err := ensureCoreJobIncludeTargetColumn(ctx, s.db); err != nil {
		return err
	}
	return s.migrateScheduledJobs(ctx)
}

func ensureCoreJobIncludeTargetColumn(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(core_job_rules)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "include_target" {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE core_job_rules ADD COLUMN include_target INTEGER NOT NULL DEFAULT 0`)
	return err
}

func (s *store) listCoreJobRules(ctx context.Context, familyID string) ([]CoreJobRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.family_id, r.target_member_id, m.name, r.enabled, r.include_target, r.inactivity_hours, r.reminder_text, r.updated_at
		FROM core_job_rules r JOIN members m ON m.id = r.target_member_id
		WHERE r.family_id = ? ORDER BY m.created_at`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]CoreJobRule, 0)
	for rows.Next() {
		var rule CoreJobRule
		var enabled, includeTarget int
		var updatedAt string
		if err := rows.Scan(&rule.ID, &rule.FamilyID, &rule.TargetMemberID, &rule.TargetMemberName, &enabled, &includeTarget, &rule.InactivityHours, &rule.ReminderText, &updatedAt); err != nil {
			return nil, err
		}
		rule.Enabled = enabled == 1
		rule.IncludeTarget = includeTarget == 1
		rule.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range rules {
		rules[index].RecipientMemberIDs, err = s.memberIDsForRelation(ctx, "core_job_rule_recipients", "rule_id", rules[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func (s *store) saveCoreJobRule(ctx context.Context, rule CoreJobRule) (CoreJobRule, error) {
	member, err := s.getMember(ctx, rule.TargetMemberID)
	if err != nil {
		return CoreJobRule{}, err
	}
	if member.FamilyID != rule.FamilyID {
		return CoreJobRule{}, sql.ErrNoRows
	}
	if err := s.validateFamilyMemberIDs(ctx, rule.FamilyID, rule.RecipientMemberIDs); err != nil {
		return CoreJobRule{}, err
	}
	if rule.ID == "" {
		rule.ID = newID()
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO core_job_rules(id, family_id, target_member_id, enabled, include_target, inactivity_hours, reminder_text, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_member_id) DO UPDATE SET family_id = excluded.family_id, enabled = excluded.enabled,
		include_target = excluded.include_target, inactivity_hours = excluded.inactivity_hours, reminder_text = excluded.reminder_text, updated_at = excluded.updated_at`,
		rule.ID, rule.FamilyID, rule.TargetMemberID, rule.Enabled, rule.IncludeTarget, rule.InactivityHours, rule.ReminderText, rule.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return CoreJobRule{}, err
	}
	if rule.RecipientMemberIDs != nil {
		if err := s.replaceMemberRelation(ctx, "core_job_rule_recipients", "rule_id", rule.ID, rule.RecipientMemberIDs); err != nil {
			return CoreJobRule{}, err
		}
	}
	var stored CoreJobRule
	var enabled, includeTarget int
	var updatedAt string
	err = s.db.QueryRowContext(ctx, `SELECT r.id, r.family_id, r.target_member_id, m.name, r.enabled, r.include_target, r.inactivity_hours, r.reminder_text, r.updated_at
		FROM core_job_rules r JOIN members m ON m.id = r.target_member_id WHERE r.target_member_id = ?`, rule.TargetMemberID).
		Scan(&stored.ID, &stored.FamilyID, &stored.TargetMemberID, &stored.TargetMemberName, &enabled, &includeTarget, &stored.InactivityHours, &stored.ReminderText, &updatedAt)
	if err != nil {
		return CoreJobRule{}, err
	}
	stored.Enabled = enabled == 1
	stored.IncludeTarget = includeTarget == 1
	stored.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err == nil {
		stored.RecipientMemberIDs, err = s.memberIDsForRelation(ctx, "core_job_rule_recipients", "rule_id", stored.ID)
	}
	return stored, err
}

func (s *store) runCoreJobs(ctx context.Context, now time.Time) (CoreJobRunResult, error) {
	rules, err := s.enabledCoreJobRules(ctx)
	if err != nil {
		return CoreJobRunResult{}, err
	}
	result := CoreJobRunResult{RulesEvaluated: len(rules)}
	for _, rule := range rules {
		lastActivity, err := s.memberLastActivity(ctx, rule.TargetMemberID)
		if err != nil {
			return result, err
		}
		if now.Before(lastActivity.Add(time.Duration(rule.InactivityHours) * time.Hour)) {
			continue
		}
		result.AnomaliesDetected++
		created, err := s.createInactivityNotifications(ctx, rule, lastActivity, now)
		if err != nil {
			return result, err
		}
		result.NotificationsCreated += created
	}
	scheduled, err := s.runScheduledJobs(ctx, now)
	if err != nil {
		return result, err
	}
	result.RulesEvaluated += scheduled.Evaluated
	result.JobsTriggered += scheduled.Triggered
	result.NotificationsCreated += scheduled.NotificationsCreated
	return result, nil
}

func (s *store) enabledCoreJobRules(ctx context.Context) ([]CoreJobRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.family_id, r.target_member_id, m.name, r.enabled, r.include_target, r.inactivity_hours, r.reminder_text, r.updated_at
		FROM core_job_rules r JOIN members m ON m.id = r.target_member_id WHERE r.enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]CoreJobRule, 0)
	for rows.Next() {
		var rule CoreJobRule
		var enabled, includeTarget int
		var updatedAt string
		if err := rows.Scan(&rule.ID, &rule.FamilyID, &rule.TargetMemberID, &rule.TargetMemberName, &enabled, &includeTarget, &rule.InactivityHours, &rule.ReminderText, &updatedAt); err != nil {
			return nil, err
		}
		rule.Enabled = enabled == 1
		rule.IncludeTarget = includeTarget == 1
		rule.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range rules {
		rules[index].RecipientMemberIDs, err = s.memberIDsForRelation(ctx, "core_job_rule_recipients", "rule_id", rules[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func (s *store) memberLastActivity(ctx context.Context, memberID string) (time.Time, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT activity_at FROM (
		SELECT created_at AS activity_at FROM updates WHERE member_id = ?
		UNION ALL SELECT created_at AS activity_at FROM members WHERE id = ?
	) ORDER BY julianday(activity_at) DESC LIMIT 1`, memberID, memberID).Scan(&value)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, value)
}

func (s *store) createInactivityNotifications(ctx context.Context, rule CoreJobRule, lastActivity, now time.Time) (int, error) {
	if len(rule.RecipientMemberIDs) > 0 {
		return s.createInactivityNotificationsFor(ctx, rule, rule.RecipientMemberIDs, lastActivity, now)
	}
	query := `SELECT id FROM members WHERE family_id = ?`
	args := []any{rule.FamilyID}
	if !rule.IncludeTarget {
		query += ` AND id != ?`
		args = append(args, rule.TargetMemberID)
	}
	query += ` ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	recipients := make([]string, 0)
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			rows.Close()
			return 0, err
		}
		recipients = append(recipients, memberID)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	return s.createInactivityNotificationsFor(ctx, rule, recipients, lastActivity, now)
}

func (s *store) createInactivityNotificationsFor(ctx context.Context, rule CoreJobRule, recipients []string, lastActivity, now time.Time) (int, error) {
	incidentKey := "no-post:" + rule.ID + ":" + lastActivity.UTC().Format(time.RFC3339Nano)
	created := 0
	for _, recipientID := range recipients {
		message := strings.TrimSpace(rule.ReminderText)
		if message == "" && recipientID == rule.TargetMemberID {
			message = fmt.Sprintf("你已经 %d 小时没有发布新动态了，方便时分享一下近况，让家人放心。", rule.InactivityHours)
		} else if message == "" {
			message = fmt.Sprintf("%s 已经 %d 小时没有发布新动态了，方便时请联系一下。", rule.TargetMemberName, rule.InactivityHours)
		}
		res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notifications(id, family_id, recipient_member_id, subject_member_id, type, message, incident_key, created_at)
			VALUES(?, ?, ?, ?, 'member_inactive', ?, ?, ?)`, newID(), rule.FamilyID, recipientID, rule.TargetMemberID, message, incidentKey, now.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return created, err
		}
		if rows, _ := res.RowsAffected(); rows == 1 {
			created++
		}
	}
	return created, nil
}

func (s *store) listNotifications(ctx context.Context, familyID, memberID string) ([]Notification, error) {
	member, err := s.getMember(ctx, memberID)
	if err != nil || member.FamilyID != familyID {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT n.id, n.family_id, n.recipient_member_id, n.subject_member_id, m.name, n.type, n.message, n.created_at, n.read_at
		FROM notifications n JOIN members m ON m.id = n.subject_member_id
		WHERE n.family_id = ? AND n.recipient_member_id = ? ORDER BY n.created_at DESC LIMIT 50`, familyID, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notifications := make([]Notification, 0)
	for rows.Next() {
		var notification Notification
		var createdAt string
		var readAt sql.NullString
		if err := rows.Scan(&notification.ID, &notification.FamilyID, &notification.RecipientMemberID, &notification.SubjectMemberID,
			&notification.SubjectMemberName, &notification.Type, &notification.Message, &createdAt, &readAt); err != nil {
			return nil, err
		}
		notification.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		if readAt.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, readAt.String)
			if parseErr != nil {
				return nil, parseErr
			}
			notification.ReadAt = &value
		}
		notifications = append(notifications, notification)
	}
	return notifications, rows.Err()
}

func (s *store) markNotificationRead(ctx context.Context, familyID, memberID, notificationID string, readAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE id = ? AND family_id = ? AND recipient_member_id = ?`,
		readAt.UTC().Format(time.RFC3339Nano), notificationID, familyID, memberID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *app) adminListCoreJobRules(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	rules, err := a.store.listCoreJobRules(r.Context(), familyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取检测规则")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (a *app) adminSaveCoreJobRule(w http.ResponseWriter, r *http.Request) {
	memberID := strings.TrimSpace(r.PathValue("memberId"))
	var input struct {
		FamilyID           string   `json:"familyId"`
		Enabled            bool     `json:"enabled"`
		IncludeTarget      bool     `json:"includeTarget"`
		RecipientMemberIDs []string `json:"recipientMemberIds"`
		InactivityHours    int      `json:"inactivityHours"`
		ReminderText       string   `json:"reminderText"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "检测规则格式不正确")
		return
	}
	input.FamilyID = strings.TrimSpace(input.FamilyID)
	if input.FamilyID == "" {
		input.FamilyID = defaultFamilyID
	}
	input.ReminderText = strings.TrimSpace(input.ReminderText)
	if memberID == "" || input.InactivityHours < 1 || input.InactivityHours > 24*30 || len([]rune(input.ReminderText)) > 300 {
		writeError(w, http.StatusBadRequest, "成员、未发布时长或提醒内容不正确")
		return
	}
	if input.Enabled && input.RecipientMemberIDs != nil && len(input.RecipientMemberIDs) == 0 {
		writeError(w, http.StatusBadRequest, "请至少选择一位提醒对象")
		return
	}
	rule, err := a.store.saveCoreJobRule(r.Context(), CoreJobRule{FamilyID: input.FamilyID, TargetMemberID: memberID,
		Enabled: input.Enabled, IncludeTarget: input.IncludeTarget, RecipientMemberIDs: input.RecipientMemberIDs, InactivityHours: input.InactivityHours, ReminderText: input.ReminderText, UpdatedAt: time.Now().UTC()})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个家庭成员")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法保存检测规则")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (a *app) adminRunCoreJobs(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.runCoreJobs(r.Context(), time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法运行检测任务")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *app) listMemberNotifications(w http.ResponseWriter, r *http.Request) {
	memberID := strings.TrimSpace(r.URL.Query().Get("memberId"))
	if memberID == "" {
		writeError(w, http.StatusBadRequest, "成员不能为空")
		return
	}
	notifications, err := a.store.listNotifications(r.Context(), defaultFamilyID, memberID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个家庭成员")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取提醒")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": notifications})
}

func (a *app) readMemberNotification(w http.ResponseWriter, r *http.Request) {
	var input struct {
		MemberID string `json:"memberId"`
	}
	if err := readJSON(r, &input); err != nil || strings.TrimSpace(input.MemberID) == "" {
		writeError(w, http.StatusBadRequest, "成员不能为空")
		return
	}
	err := a.store.markNotificationRead(r.Context(), defaultFamilyID, strings.TrimSpace(input.MemberID), r.PathValue("id"), time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这条提醒")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法更新提醒")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) memberListNotifications(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	notifications, err := a.store.listNotifications(r.Context(), member.FamilyID, member.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取提醒")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": notifications})
}

func (a *app) memberReadNotification(w http.ResponseWriter, r *http.Request) {
	member := memberFromContext(r.Context())
	if err := a.store.markNotificationRead(r.Context(), member.FamilyID, member.ID, r.PathValue("id"), time.Now().UTC()); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这条提醒")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法更新提醒")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) startCoreJobs(ctx context.Context) {
	interval := 15 * time.Minute
	if value := strings.TrimSpace(envOr("CORE_JOB_INTERVAL", "")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			interval = parsed
		} else {
			log.Printf("invalid CORE_JOB_INTERVAL %q; using %s", value, interval)
		}
	}
	run := func() {
		result, err := a.store.runCoreJobs(ctx, time.Now().UTC())
		if err != nil {
			log.Printf("core job run failed: %v", err)
			return
		}
		if result.NotificationsCreated > 0 {
			log.Printf("core jobs detected %d anomalies and created %d notifications", result.AnomaliesDetected, result.NotificationsCreated)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
