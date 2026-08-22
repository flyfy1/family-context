package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestBirthdayAndFamilyActivityJobsRunAndDeduplicate(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	members := []Member{
		{ID: "elder", FamilyID: defaultFamilyID, Name: "妈妈", Role: "elder", Color: "#54706A", CreatedAt: now.Add(-time.Hour)},
		{ID: "daughter", FamilyID: defaultFamilyID, Name: "女儿", Role: "member", Color: "#AD4C34", CreatedAt: now.Add(-time.Hour)},
		{ID: "son", FamilyID: defaultFamilyID, Name: "儿子", Role: "member", Color: "#35677B", CreatedAt: now.Add(-time.Hour)},
	}
	for _, member := range members {
		if err := store.createMember(ctx, member); err != nil {
			t.Fatal(err)
		}
	}
	birthday, err := store.saveScheduledJob(ctx, ScheduledJob{FamilyID: defaultFamilyID, Type: scheduledJobBirthday,
		Title: "妈妈生日提醒", TargetMemberID: "elder", BirthdayMonthDay: "08-25", RemindDaysBefore: 3,
		TimeZone: "Asia/Singapore", Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.runCoreJobs(ctx, now)
	if err != nil || result.JobsTriggered != 1 || result.NotificationsCreated != 2 {
		t.Fatalf("birthday result=%+v err=%v", result, err)
	}
	if notifications, err := store.listNotifications(ctx, defaultFamilyID, birthday.TargetMemberID); err != nil || len(notifications) != 0 {
		t.Fatalf("birthday person should be excluded: %+v err=%v", notifications, err)
	}
	result, err = store.runCoreJobs(ctx, now.Add(time.Minute))
	if err != nil || result.JobsTriggered != 0 || result.NotificationsCreated != 0 {
		t.Fatalf("birthday duplicate result=%+v err=%v", result, err)
	}

	scheduledFor := now.Add(time.Hour)
	activity, err := store.saveScheduledJob(ctx, ScheduledJob{FamilyID: defaultFamilyID, Type: scheduledJobFamilyActivity,
		Title: "周末家庭活动", Topic: "每个人分享一张最喜欢的老照片", ScheduledFor: &scheduledFor,
		Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	result, err = store.runCoreJobs(ctx, now.Add(59*time.Minute))
	if err != nil || result.NotificationsCreated != 0 {
		t.Fatalf("early activity result=%+v err=%v", result, err)
	}
	result, err = store.runCoreJobs(ctx, scheduledFor)
	if err != nil || result.JobsTriggered != 1 || result.NotificationsCreated != 3 {
		t.Fatalf("due activity result=%+v err=%v", result, err)
	}
	stored, err := store.getScheduledJob(ctx, defaultFamilyID, activity.ID)
	if err != nil || stored.CompletedAt == nil {
		t.Fatalf("activity was not completed: %+v err=%v", stored, err)
	}
	result, err = store.runCoreJobs(ctx, scheduledFor.Add(time.Hour))
	if err != nil || result.NotificationsCreated != 0 {
		t.Fatalf("completed activity duplicate result=%+v err=%v", result, err)
	}

	nextYear := time.Date(2027, 8, 22, 10, 0, 0, 0, time.UTC)
	result, err = store.runCoreJobs(ctx, nextYear)
	if err != nil || result.JobsTriggered != 1 || result.NotificationsCreated != 2 {
		t.Fatalf("annual birthday result=%+v err=%v", result, err)
	}
}

func TestLeapDayBirthdayUsesFebruary28InNonLeapYear(t *testing.T) {
	location, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	job := ScheduledJob{ID: "leap", Type: scheduledJobBirthday, BirthdayMonthDay: "02-29", TimeZone: location.String()}
	due, incident, err := scheduledJobDue(job, time.Date(2027, 2, 28, 4, 0, 0, 0, time.UTC))
	if err != nil || !due || incident != "birthday:leap:2027" {
		t.Fatalf("leap birthday due=%v incident=%q err=%v", due, incident, err)
	}
}

func TestScheduledJobAdminCRUD(t *testing.T) {
	t.Setenv("ADMIN_API_TOKEN", "admin-token")
	temp := t.TempDir()
	store, err := openStore(filepath.Join(temp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	member := Member{ID: "elder", FamilyID: defaultFamilyID, Name: "爸爸", Role: "elder", Color: "#54706A", CreatedAt: time.Now().UTC()}
	if err := store.createMember(context.Background(), member); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newApp(store, stubAudioProcessor{}, filepath.Join(temp, "media"), "family-token").routes())
	t.Cleanup(server.Close)

	job := requestScopedJSON[ScheduledJob](t, server.Client(), http.MethodPost, server.URL+"/api/v1/admin/scheduled-jobs", map[string]any{
		"familyId": defaultFamilyID, "type": "birthday", "title": "爸爸生日", "targetMemberId": "elder",
		"birthdayMonthDay": "12-03", "remindDaysBefore": 7, "timeZone": "Asia/Singapore", "enabled": true,
	}, "X-Admin-Token", "admin-token", http.StatusCreated)
	if job.ID == "" || job.TargetMemberName != "爸爸" || job.BirthdayMonthDay != "12-03" {
		t.Fatalf("unexpected created job: %+v", job)
	}
	listed := requestScopedJSON[struct {
		Jobs []ScheduledJob `json:"jobs"`
	}](t, server.Client(), http.MethodGet, server.URL+"/api/v1/admin/scheduled-jobs?familyId="+defaultFamilyID, nil,
		"X-Admin-Token", "admin-token", http.StatusOK)
	if len(listed.Jobs) != 1 || listed.Jobs[0].ID != job.ID {
		t.Fatalf("unexpected job list: %+v", listed.Jobs)
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/admin/scheduled-jobs/"+job.ID+"?familyId="+defaultFamilyID, nil)
	request.Header.Set("X-Admin-Token", "admin-token")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status=%d", response.StatusCode)
	}
}
