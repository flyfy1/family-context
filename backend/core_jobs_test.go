package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCoreJobMigrationAddsIncludeTargetToExistingRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE core_job_rules (
		id TEXT PRIMARY KEY, family_id TEXT NOT NULL, target_member_id TEXT NOT NULL UNIQUE,
		enabled INTEGER NOT NULL DEFAULT 0, inactivity_hours INTEGER NOT NULL DEFAULT 24,
		reminder_text TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	rows, err := store.db.Query(`PRAGMA table_info(core_job_rules)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		found = found || name == "include_target"
	}
	if !found {
		t.Fatal("include_target column was not added")
	}
}

func TestNoPostCoreJobDetectsDeduplicatesAndResetsAfterActivity(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	subject := Member{ID: "elder", FamilyID: defaultFamilyID, Name: "妈妈", Role: "elder", Color: "#54706A", CreatedAt: now.Add(-48 * time.Hour)}
	recipientA := Member{ID: "daughter", FamilyID: defaultFamilyID, Name: "女儿", Role: "member", Color: "#AD4C34", CreatedAt: now.Add(-48 * time.Hour)}
	recipientB := Member{ID: "son", FamilyID: defaultFamilyID, Name: "儿子", Role: "member", Color: "#35677B", CreatedAt: now.Add(-48 * time.Hour)}
	for _, member := range []Member{subject, recipientA, recipientB} {
		if err := store.createMember(ctx, member); err != nil {
			t.Fatal(err)
		}
	}

	rule, err := store.saveCoreJobRule(ctx, CoreJobRule{FamilyID: defaultFamilyID, TargetMemberID: subject.ID,
		Enabled: false, InactivityHours: 24, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.runCoreJobs(ctx, now)
	if err != nil || result.NotificationsCreated != 0 {
		t.Fatalf("disabled rule result=%+v err=%v", result, err)
	}

	rule.Enabled = true
	rule.ReminderText = "妈妈今天还没有分享近况，方便时问候一下。"
	rule.UpdatedAt = now
	if _, err := store.saveCoreJobRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	result, err = store.runCoreJobs(ctx, now)
	if err != nil || result.AnomaliesDetected != 1 || result.NotificationsCreated != 2 {
		t.Fatalf("first run result=%+v err=%v", result, err)
	}
	result, err = store.runCoreJobs(ctx, now.Add(time.Hour))
	if err != nil || result.NotificationsCreated != 0 {
		t.Fatalf("duplicate run result=%+v err=%v", result, err)
	}

	for _, recipient := range []Member{recipientA, recipientB} {
		notifications, err := store.listNotifications(ctx, defaultFamilyID, recipient.ID)
		if err != nil || len(notifications) != 1 || notifications[0].SubjectMemberID != subject.ID || notifications[0].Message != rule.ReminderText {
			t.Fatalf("recipient %s notifications=%+v err=%v", recipient.ID, notifications, err)
		}
	}
	if notifications, err := store.listNotifications(ctx, defaultFamilyID, subject.ID); err != nil || len(notifications) != 0 {
		t.Fatalf("subject received own reminder: %+v err=%v", notifications, err)
	}
	rule.IncludeTarget = true
	rule.UpdatedAt = now.Add(90 * time.Minute)
	if _, err := store.saveCoreJobRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	result, err = store.runCoreJobs(ctx, now.Add(90*time.Minute))
	if err != nil || result.NotificationsCreated != 1 {
		t.Fatalf("including subject in existing quiet period result=%+v err=%v", result, err)
	}
	if notifications, err := store.listNotifications(ctx, defaultFamilyID, subject.ID); err != nil || len(notifications) != 1 {
		t.Fatalf("subject reminder missing: %+v err=%v", notifications, err)
	}

	activityAt := now.Add(2 * time.Hour)
	if err := store.createUpdate(ctx, Update{ID: "new-activity", FamilyID: defaultFamilyID, MemberID: subject.ID, Type: "text",
		Text: "今天在家看书。", Visibility: "private", Source: "member_api", CreatedAt: activityAt}, ""); err != nil {
		t.Fatal(err)
	}
	result, err = store.runCoreJobs(ctx, activityAt.Add(23*time.Hour))
	if err != nil || result.AnomaliesDetected != 0 {
		t.Fatalf("recent private activity should count result=%+v err=%v", result, err)
	}
	result, err = store.runCoreJobs(ctx, activityAt.Add(24*time.Hour))
	if err != nil || result.NotificationsCreated != 3 {
		t.Fatalf("new quiet period result=%+v err=%v", result, err)
	}
}

func TestCoreJobAdminConfigurationAndRecipientAPI(t *testing.T) {
	t.Setenv("ADMIN_API_TOKEN", "admin-token")
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	now := time.Now().UTC().Add(-48 * time.Hour)
	for _, member := range []Member{
		{ID: "elder", FamilyID: defaultFamilyID, Name: "爸爸", Role: "elder", Color: "#54706A", CreatedAt: now},
		{ID: "family", FamilyID: defaultFamilyID, Name: "家人", Role: "member", Color: "#AD4C34", CreatedAt: now},
	} {
		if err := store.createMember(context.Background(), member); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, filepath.Join(temp, "media"), "admin-token").routes())
	t.Cleanup(server.Close)

	request, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/admin/core-job-rules/elder", nil)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("admin rule without token status=%d", response.StatusCode)
	}

	rule := requestScopedJSON[CoreJobRule](t, server.Client(), http.MethodPut, server.URL+"/api/v1/admin/core-job-rules/elder",
		map[string]any{"familyId": defaultFamilyID, "enabled": true, "includeTarget": true, "inactivityHours": 24, "reminderText": "请分享或联系爸爸确认近况。"},
		"X-Admin-Token", "admin-token", http.StatusOK)
	if !rule.Enabled || !rule.IncludeTarget || rule.TargetMemberName != "爸爸" {
		t.Fatalf("unexpected saved rule: %+v", rule)
	}
	result := requestScopedJSON[CoreJobRunResult](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/core-jobs/run", map[string]any{},
		"X-Admin-Token", "admin-token", http.StatusOK)
	if result.NotificationsCreated != 2 {
		t.Fatalf("unexpected run result: %+v", result)
	}
	notifications := requestScopedJSON[struct {
		Notifications []Notification `json:"notifications"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/notifications?memberId=family", nil,
		"X-Admin-Token", "admin-token", http.StatusOK)
	if len(notifications.Notifications) != 1 || notifications.Notifications[0].RecipientMemberID != "family" {
		t.Fatalf("unexpected notifications: %+v", notifications.Notifications)
	}
	subjectNotifications := requestScopedJSON[struct {
		Notifications []Notification `json:"notifications"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/notifications?memberId=elder", nil,
		"X-Admin-Token", "admin-token", http.StatusOK)
	if len(subjectNotifications.Notifications) != 1 || subjectNotifications.Notifications[0].RecipientMemberID != "elder" {
		t.Fatalf("unexpected subject notifications: %+v", subjectNotifications.Notifications)
	}
}

func TestCoreJobUsesExplicitRecipientSelection(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	now := time.Now().UTC()
	for _, member := range []Member{
		{ID: "elder", FamilyID: defaultFamilyID, Name: "奶奶", Role: "elder", CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "daughter", FamilyID: defaultFamilyID, Name: "女儿", Role: "member", CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "son", FamilyID: defaultFamilyID, Name: "儿子", Role: "member", CreatedAt: now.Add(-48 * time.Hour)},
	} {
		if err := store.createMember(ctx, member); err != nil {
			t.Fatal(err)
		}
	}
	rule, err := store.saveCoreJobRule(ctx, CoreJobRule{FamilyID: defaultFamilyID, TargetMemberID: "elder", Enabled: true,
		RecipientMemberIDs: []string{"elder", "daughter"}, InactivityHours: 24, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rule.RecipientMemberIDs) != 2 {
		t.Fatalf("selection not stored: %+v", rule)
	}
	result, err := store.runCoreJobs(ctx, now)
	if err != nil || result.NotificationsCreated != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if notifications, _ := store.listNotifications(ctx, defaultFamilyID, "son"); len(notifications) != 0 {
		t.Fatalf("unselected member was notified: %+v", notifications)
	}
}

func TestAdminBroadcastNotificationCreatesMemberScopedHTTPSActions(t *testing.T) {
	t.Setenv("ADMIN_API_TOKEN", "admin-token")
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	now := time.Now().UTC()
	for _, member := range []Member{
		{ID: "first", FamilyID: defaultFamilyID, Name: "外婆", Role: "elder", CreatedAt: now},
		{ID: "second", FamilyID: defaultFamilyID, Name: "妈妈", Role: "member", CreatedAt: now},
	} {
		if err := store.createMember(context.Background(), member); err != nil {
			t.Fatal(err)
		}
	}
	memberToken := "member-token"
	if err := store.setMemberTokenHash(context.Background(), "second", hashToken(memberToken), now); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, filepath.Join(temp, "media"), "admin-token").routes())
	t.Cleanup(server.Close)

	requestScopedJSON[map[string]any](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/notifications/broadcast",
		map[string]any{"title": "想念你了", "titleEn": "We miss you", "message": "家里好久没有新动态，点这里看看。", "messageEn": "There have not been new family updates for a while. Tap to check in.", "actionUrl": "http://family.integ.life"},
		"X-Admin-Token", "admin-token", http.StatusBadRequest)
	result := requestScopedJSON[struct {
		NotificationsCreated      int `json:"notificationsCreated"`
		MembersMarkedForAttention int `json:"membersMarkedForAttention"`
	}](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/notifications/broadcast",
		map[string]any{"title": "想念你了", "titleEn": "We miss you", "message": "家里好久没有新动态，点这里看看。", "messageEn": "There have not been new family updates for a while. Tap to check in.", "actionUrl": "https://family.integ.life/#/feed", "markNeedsAttention": true},
		"X-Admin-Token", "admin-token", http.StatusCreated)
	if result.NotificationsCreated != 2 || result.MembersMarkedForAttention != 1 {
		t.Fatalf("broadcast result=%+v", result)
	}
	members, err := store.listMembers(context.Background(), defaultFamilyID)
	if err != nil || len(members) != 2 || !members[0].NeedsAttention || members[1].NeedsAttention {
		t.Fatalf("attention members=%+v err=%v", members, err)
	}
	for _, memberID := range []string{"first", "second"} {
		notifications, err := store.listNotifications(context.Background(), defaultFamilyID, memberID)
		if err != nil || len(notifications) != 1 {
			t.Fatalf("member %s notifications=%+v err=%v", memberID, notifications, err)
		}
		got := notifications[0]
		if got.Title != "想念你了" || got.TitleEN != "We miss you" || got.Message != "家里好久没有新动态，点这里看看。" || got.MessageEN != "There have not been new family updates for a while. Tap to check in." || got.ActionURL != "https://family.integ.life/#/feed" {
			t.Fatalf("member %s notification=%+v", memberID, got)
		}
	}
	requestScopedJSON[map[string]any](t, server.Client(), http.MethodPost, server.URL+"/api/v1/me/members/first/attention/dismiss",
		nil, "Authorization", "Bearer "+memberToken, http.StatusNoContent)
	members, err = store.listMembers(context.Background(), defaultFamilyID)
	if err != nil || members[0].NeedsAttention {
		t.Fatalf("attention was not dismissed: members=%+v err=%v", members, err)
	}
}
