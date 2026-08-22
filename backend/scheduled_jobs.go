package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	scheduledJobBirthday       = "birthday"
	scheduledJobFamilyActivity = "family_activity"
)

type ScheduledJob struct {
	ID               string     `json:"id"`
	FamilyID         string     `json:"familyId"`
	Type             string     `json:"type"`
	Title            string     `json:"title"`
	TargetMemberID   string     `json:"targetMemberId,omitempty"`
	TargetMemberName string     `json:"targetMemberName,omitempty"`
	IncludeTarget    bool       `json:"includeTarget"`
	MemberIDs        []string   `json:"memberIds,omitempty"`
	BirthdayMonthDay string     `json:"birthdayMonthDay,omitempty"`
	RemindDaysBefore int        `json:"remindDaysBefore,omitempty"`
	TimeZone         string     `json:"timeZone,omitempty"`
	ScheduledFor     *time.Time `json:"scheduledFor,omitempty"`
	Topic            string     `json:"topic,omitempty"`
	Message          string     `json:"message,omitempty"`
	Enabled          bool       `json:"enabled"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type scheduledJobRunResult struct {
	Evaluated            int
	Triggered            int
	NotificationsCreated int
}

func (s *store) migrateScheduledJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS scheduled_jobs (
  id TEXT PRIMARY KEY,
  family_id TEXT NOT NULL,
  job_type TEXT NOT NULL,
  title TEXT NOT NULL,
  target_member_id TEXT REFERENCES members(id) ON DELETE CASCADE,
  include_target INTEGER NOT NULL DEFAULT 0,
  birthday_month_day TEXT NOT NULL DEFAULT '',
  remind_days_before INTEGER NOT NULL DEFAULT 0,
  time_zone TEXT NOT NULL DEFAULT '',
  scheduled_for TEXT,
  topic TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_family ON scheduled_jobs(family_id, job_type, enabled);

CREATE TABLE IF NOT EXISTS scheduled_job_members (
  job_id TEXT NOT NULL REFERENCES scheduled_jobs(id) ON DELETE CASCADE,
  member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  PRIMARY KEY(job_id, member_id)
);
`)
	if err != nil {
		return err
	}
	return s.migrateActivityThreads(ctx)
}

func (s *store) listScheduledJobs(ctx context.Context, familyID string) ([]ScheduledJob, error) {
	return s.queryScheduledJobs(ctx, `WHERE j.family_id = ? ORDER BY j.created_at DESC`, familyID)
}

func (s *store) enabledScheduledJobs(ctx context.Context) ([]ScheduledJob, error) {
	return s.queryScheduledJobs(ctx, `WHERE j.enabled = 1 AND (j.job_type != ? OR j.completed_at IS NULL)`, scheduledJobFamilyActivity)
}

func (s *store) queryScheduledJobs(ctx context.Context, condition string, args ...any) ([]ScheduledJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT j.id, j.family_id, j.job_type, j.title, j.target_member_id, COALESCE(m.name, ''),
		j.include_target, j.birthday_month_day, j.remind_days_before, j.time_zone, j.scheduled_for, j.topic, j.message,
		j.enabled, j.completed_at, j.created_at, j.updated_at
		FROM scheduled_jobs j LEFT JOIN members m ON m.id = j.target_member_id `+condition, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]ScheduledJob, 0)
	for rows.Next() {
		job, err := scanScheduledJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range jobs {
		jobs[index].MemberIDs, err = s.memberIDsForRelation(ctx, "scheduled_job_members", "job_id", jobs[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

func scanScheduledJob(row rowScanner) (ScheduledJob, error) {
	var job ScheduledJob
	var targetMemberID, scheduledFor, completedAt sql.NullString
	var includeTarget, enabled int
	var createdAt, updatedAt string
	if err := row.Scan(&job.ID, &job.FamilyID, &job.Type, &job.Title, &targetMemberID, &job.TargetMemberName,
		&includeTarget, &job.BirthdayMonthDay, &job.RemindDaysBefore, &job.TimeZone, &scheduledFor, &job.Topic, &job.Message,
		&enabled, &completedAt, &createdAt, &updatedAt); err != nil {
		return ScheduledJob{}, err
	}
	job.TargetMemberID = targetMemberID.String
	job.IncludeTarget = includeTarget == 1
	job.Enabled = enabled == 1
	var err error
	job.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ScheduledJob{}, err
	}
	job.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ScheduledJob{}, err
	}
	if scheduledFor.Valid {
		value, err := time.Parse(time.RFC3339Nano, scheduledFor.String)
		if err != nil {
			return ScheduledJob{}, err
		}
		job.ScheduledFor = &value
	}
	if completedAt.Valid {
		value, err := time.Parse(time.RFC3339Nano, completedAt.String)
		if err != nil {
			return ScheduledJob{}, err
		}
		job.CompletedAt = &value
	}
	return job, nil
}

func (s *store) saveScheduledJob(ctx context.Context, job ScheduledJob) (ScheduledJob, error) {
	if err := s.validateScheduledJob(ctx, job); err != nil {
		return ScheduledJob{}, err
	}
	if err := s.validateFamilyMemberIDs(ctx, job.FamilyID, job.MemberIDs); err != nil {
		return ScheduledJob{}, err
	}
	targetMemberID := any(nil)
	if job.TargetMemberID != "" {
		targetMemberID = job.TargetMemberID
	}
	scheduledFor := any(nil)
	if job.ScheduledFor != nil {
		scheduledFor = job.ScheduledFor.UTC().Format(time.RFC3339Nano)
	}
	if job.ID == "" {
		job.ID = newID()
		_, err := s.db.ExecContext(ctx, `INSERT INTO scheduled_jobs(id, family_id, job_type, title, target_member_id, include_target,
			birthday_month_day, remind_days_before, time_zone, scheduled_for, topic, message, enabled, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, job.FamilyID, job.Type, job.Title, targetMemberID,
			job.IncludeTarget, job.BirthdayMonthDay, job.RemindDaysBefore, job.TimeZone, scheduledFor, job.Topic, job.Message,
			job.Enabled, job.CreatedAt.UTC().Format(time.RFC3339Nano), job.UpdatedAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return ScheduledJob{}, err
		}
	} else {
		result, err := s.db.ExecContext(ctx, `UPDATE scheduled_jobs SET job_type = ?, title = ?, target_member_id = ?, include_target = ?,
			birthday_month_day = ?, remind_days_before = ?, time_zone = ?, scheduled_for = ?, topic = ?, message = ?, enabled = ?,
			completed_at = NULL, updated_at = ? WHERE id = ? AND family_id = ?`, job.Type, job.Title, targetMemberID, job.IncludeTarget,
			job.BirthdayMonthDay, job.RemindDaysBefore, job.TimeZone, scheduledFor, job.Topic, job.Message, job.Enabled,
			job.UpdatedAt.UTC().Format(time.RFC3339Nano), job.ID, job.FamilyID)
		if err != nil {
			return ScheduledJob{}, err
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return ScheduledJob{}, sql.ErrNoRows
		}
	}
	if job.MemberIDs != nil {
		if err := s.replaceMemberRelation(ctx, "scheduled_job_members", "job_id", job.ID, job.MemberIDs); err != nil {
			return ScheduledJob{}, err
		}
	}
	return s.getScheduledJob(ctx, job.FamilyID, job.ID)
}

func (s *store) validateScheduledJob(ctx context.Context, job ScheduledJob) error {
	if job.FamilyID == "" || strings.TrimSpace(job.Title) == "" || len([]rune(job.Title)) > 80 || len([]rune(job.Message)) > 300 {
		return errors.New("invalid scheduled job")
	}
	switch job.Type {
	case scheduledJobBirthday:
		member, err := s.getMember(ctx, job.TargetMemberID)
		if err != nil || member.FamilyID != job.FamilyID {
			return errors.New("invalid birthday member")
		}
		if _, err := time.Parse("2006-01-02", "2000-"+job.BirthdayMonthDay); err != nil || job.RemindDaysBefore < 0 || job.RemindDaysBefore > 30 {
			return errors.New("invalid birthday schedule")
		}
		if _, err := time.LoadLocation(job.TimeZone); err != nil {
			return errors.New("invalid time zone")
		}
		if job.ScheduledFor != nil || strings.TrimSpace(job.Topic) != "" {
			return errors.New("birthday contains activity fields")
		}
	case scheduledJobFamilyActivity:
		if job.ScheduledFor == nil || strings.TrimSpace(job.Topic) == "" || len([]rune(job.Topic)) > 120 || job.TargetMemberID != "" || job.BirthdayMonthDay != "" {
			return errors.New("invalid family activity")
		}
	default:
		return errors.New("invalid scheduled job type")
	}
	return nil
}

func (s *store) getScheduledJob(ctx context.Context, familyID, id string) (ScheduledJob, error) {
	row := s.db.QueryRowContext(ctx, `SELECT j.id, j.family_id, j.job_type, j.title, j.target_member_id, COALESCE(m.name, ''),
		j.include_target, j.birthday_month_day, j.remind_days_before, j.time_zone, j.scheduled_for, j.topic, j.message,
		j.enabled, j.completed_at, j.created_at, j.updated_at
		FROM scheduled_jobs j LEFT JOIN members m ON m.id = j.target_member_id WHERE j.family_id = ? AND j.id = ?`, familyID, id)
	job, err := scanScheduledJob(row)
	if err != nil {
		return ScheduledJob{}, err
	}
	job.MemberIDs, err = s.memberIDsForRelation(ctx, "scheduled_job_members", "job_id", job.ID)
	return job, err
}

func (s *store) deleteScheduledJob(ctx context.Context, familyID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_jobs WHERE family_id = ? AND id = ?`, familyID, id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *store) runScheduledJobs(ctx context.Context, now time.Time) (scheduledJobRunResult, error) {
	jobs, err := s.enabledScheduledJobs(ctx)
	if err != nil {
		return scheduledJobRunResult{}, err
	}
	result := scheduledJobRunResult{Evaluated: len(jobs)}
	for _, job := range jobs {
		due, incidentKey, err := scheduledJobDue(job, now)
		if err != nil {
			return result, err
		}
		if !due {
			continue
		}
		threadCreated := false
		if job.Type == scheduledJobFamilyActivity {
			threadCreated, err = s.ensureScheduledActivityThread(ctx, job, now)
			if err != nil {
				return result, err
			}
		}
		created, err := s.createScheduledJobNotifications(ctx, job, incidentKey, now)
		if err != nil {
			return result, err
		}
		result.NotificationsCreated += created
		if created > 0 || threadCreated {
			result.Triggered++
		}
		if job.Type == scheduledJobFamilyActivity && (created > 0 || threadCreated) {
			if _, err := s.db.ExecContext(ctx, `UPDATE scheduled_jobs SET completed_at = ?, updated_at = ? WHERE id = ?`,
				now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), job.ID); err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

func scheduledJobDue(job ScheduledJob, now time.Time) (bool, string, error) {
	switch job.Type {
	case scheduledJobFamilyActivity:
		if job.ScheduledFor == nil {
			return false, "", errors.New("activity is missing scheduled time")
		}
		return !now.Before(*job.ScheduledFor), "family-activity:" + job.ID + ":" + job.ScheduledFor.UTC().Format(time.RFC3339Nano), nil
	case scheduledJobBirthday:
		location, err := time.LoadLocation(job.TimeZone)
		if err != nil {
			return false, "", err
		}
		localNow := now.In(location)
		today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
		birthday, err := birthdayInYear(job.BirthdayMonthDay, today.Year(), location)
		if err != nil {
			return false, "", err
		}
		if today.After(birthday) {
			birthday, err = birthdayInYear(job.BirthdayMonthDay, today.Year()+1, location)
			if err != nil {
				return false, "", err
			}
		}
		reminderDate := birthday.AddDate(0, 0, -job.RemindDaysBefore)
		due := !today.Before(reminderDate) && !today.After(birthday)
		return due, fmt.Sprintf("birthday:%s:%d", job.ID, birthday.Year()), nil
	default:
		return false, "", errors.New("unknown scheduled job type")
	}
}

func birthdayInYear(monthDay string, year int, location *time.Location) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", "2000-"+monthDay)
	if err != nil {
		return time.Time{}, err
	}
	month, day := parsed.Month(), parsed.Day()
	if month == time.February && day == 29 && time.Date(year, time.March, 0, 0, 0, 0, 0, location).Day() != 29 {
		day = 28
	}
	return time.Date(year, month, day, 0, 0, 0, 0, location), nil
}

func (s *store) createScheduledJobNotifications(ctx context.Context, job ScheduledJob, incidentKey string, now time.Time) (int, error) {
	if len(job.MemberIDs) > 0 {
		return s.createScheduledJobNotificationsFor(ctx, job, job.MemberIDs, incidentKey, now)
	}
	query := `SELECT id FROM members WHERE family_id = ?`
	args := []any{job.FamilyID}
	if job.Type == scheduledJobBirthday && !job.IncludeTarget {
		query += ` AND id != ?`
		args = append(args, job.TargetMemberID)
	}
	query += ` ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	recipients := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		recipients = append(recipients, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	return s.createScheduledJobNotificationsFor(ctx, job, recipients, incidentKey, now)
}

func (s *store) createScheduledJobNotificationsFor(ctx context.Context, job ScheduledJob, recipients []string, incidentKey string, now time.Time) (int, error) {
	created := 0
	for _, recipientID := range recipients {
		message := scheduledJobMessage(job, recipientID, now)
		subjectMemberID := job.TargetMemberID
		if job.Type == scheduledJobFamilyActivity {
			subjectMemberID = recipientID
		}
		res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notifications(id, family_id, recipient_member_id, subject_member_id, type, message, incident_key, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, newID(), job.FamilyID, recipientID, subjectMemberID, job.Type, message, incidentKey, now.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return created, err
		}
		if rows, _ := res.RowsAffected(); rows == 1 {
			created++
		}
	}
	return created, nil
}

func scheduledJobMessage(job ScheduledJob, recipientID string, now time.Time) string {
	if custom := strings.TrimSpace(job.Message); custom != "" {
		return custom
	}
	if job.Type == scheduledJobFamilyActivity {
		return fmt.Sprintf("家庭互动主题：%s。大家可以围绕这个主题安排一次小活动，从每个人分享一个想法开始。", job.Topic)
	}
	location, _ := time.LoadLocation(job.TimeZone)
	localNow := now.In(location)
	birthday, _ := birthdayInYear(job.BirthdayMonthDay, localNow.Year(), location)
	if localNow.After(birthday.Add(24 * time.Hour)) {
		birthday, _ = birthdayInYear(job.BirthdayMonthDay, localNow.Year()+1, location)
	}
	if recipientID == job.TargetMemberID {
		return "生日快乐！今天也给自己留一点时间，和家人分享你的愿望吧。"
	}
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	days := int(birthday.Sub(today).Hours() / 24)
	if days <= 0 {
		return fmt.Sprintf("今天是%s的生日，记得送上祝福！", job.TargetMemberName)
	}
	return fmt.Sprintf("距离%s的生日还有%d天，记得准备一句祝福。", job.TargetMemberName, days)
}

func (a *app) adminListScheduledJobs(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	jobs, err := a.store.listScheduledJobs(r.Context(), familyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法读取家庭自动任务")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a *app) adminCreateScheduledJob(w http.ResponseWriter, r *http.Request) {
	a.adminSaveScheduledJob(w, r, "")
}

func (a *app) adminUpdateScheduledJob(w http.ResponseWriter, r *http.Request) {
	a.adminSaveScheduledJob(w, r, r.PathValue("id"))
}

func (a *app) adminSaveScheduledJob(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		FamilyID         string     `json:"familyId"`
		Type             string     `json:"type"`
		Title            string     `json:"title"`
		TargetMemberID   string     `json:"targetMemberId"`
		IncludeTarget    bool       `json:"includeTarget"`
		BirthdayMonthDay string     `json:"birthdayMonthDay"`
		RemindDaysBefore int        `json:"remindDaysBefore"`
		TimeZone         string     `json:"timeZone"`
		ScheduledFor     *time.Time `json:"scheduledFor"`
		Topic            string     `json:"topic"`
		Message          string     `json:"message"`
		Enabled          bool       `json:"enabled"`
		MemberIDs        []string   `json:"memberIds"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "自动任务格式不正确")
		return
	}
	input.FamilyID = strings.TrimSpace(input.FamilyID)
	if input.FamilyID == "" {
		input.FamilyID = defaultFamilyID
	}
	if input.MemberIDs != nil && len(input.MemberIDs) == 0 {
		writeError(w, http.StatusBadRequest, "请至少选择一位任务成员")
		return
	}
	now := time.Now().UTC()
	job, err := a.store.saveScheduledJob(r.Context(), ScheduledJob{ID: id, FamilyID: input.FamilyID, Type: strings.TrimSpace(input.Type),
		Title: strings.TrimSpace(input.Title), TargetMemberID: strings.TrimSpace(input.TargetMemberID), IncludeTarget: input.IncludeTarget,
		MemberIDs:        input.MemberIDs,
		BirthdayMonthDay: strings.TrimSpace(input.BirthdayMonthDay), RemindDaysBefore: input.RemindDaysBefore, TimeZone: strings.TrimSpace(input.TimeZone),
		ScheduledFor: input.ScheduledFor, Topic: strings.TrimSpace(input.Topic), Message: strings.TrimSpace(input.Message), Enabled: input.Enabled,
		CreatedAt: now, UpdatedAt: now})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个家庭自动任务")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "自动任务配置不正确")
		return
	}
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	writeJSON(w, status, job)
}

func (a *app) adminDeleteScheduledJob(w http.ResponseWriter, r *http.Request) {
	familyID := strings.TrimSpace(r.URL.Query().Get("familyId"))
	if familyID == "" {
		familyID = defaultFamilyID
	}
	if err := a.store.deleteScheduledJob(r.Context(), familyID, r.PathValue("id")); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有找到这个家庭自动任务")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "暂时无法删除家庭自动任务")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
