package main

import (
	"encoding/json"
	"time"
)

type Question struct {
	ID        string    `json:"id"`
	FamilyID  string    `json:"familyId"`
	AskedBy   string    `json:"askedBy"`
	AskedTo   string    `json:"askedTo"`
	Text      string    `json:"text"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	Answer    *Answer   `json:"answer,omitempty"`
	Replies   []Reply   `json:"replies"`
}

type Answer struct {
	ID           string     `json:"id"`
	QuestionID   string     `json:"questionId"`
	AnsweredBy   string     `json:"answeredBy"`
	AudioURL     string     `json:"audioUrl"`
	Transcript   string     `json:"transcript"`
	AISummary    string     `json:"aiSummary"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	SharedAt     *time.Time `json:"sharedAt,omitempty"`
	ArchivedAt   *time.Time `json:"archivedAt,omitempty"`
}

type AuditEvent struct {
	ID         string          `json:"id"`
	EventType  string          `json:"eventType"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type AnswerHistory struct {
	Current  *Answer      `json:"current,omitempty"`
	Archived []Answer     `json:"archived"`
	Events   []AuditEvent `json:"events"`
}

type Reply struct {
	ID        string    `json:"id"`
	AnswerID  string    `json:"answerId"`
	AuthorID  string    `json:"authorId"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type AudioResult struct {
	Transcript string `json:"transcript"`
	Summary    string `json:"summary"`
}

type Member struct {
	ID        string    `json:"id"`
	FamilyID  string    `json:"familyId"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"createdAt"`
}

type Update struct {
	ID         string    `json:"id"`
	FamilyID   string    `json:"familyId"`
	MemberID   string    `json:"memberId"`
	Type       string    `json:"type"`
	Text       string    `json:"text"`
	Visibility string    `json:"visibility"`
	AudioURL   string    `json:"audioUrl,omitempty"`
	MediaURL   string    `json:"mediaUrl,omitempty"`
	Transcript string    `json:"transcript,omitempty"`
	AISummary  string    `json:"aiSummary,omitempty"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"createdAt"`
}

type MemberSettings struct {
	MemberID    string    `json:"memberId"`
	ShareMode   string    `json:"shareMode"`
	SharePrompt string    `json:"sharePrompt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type MemberCredential struct {
	Member      Member `json:"member"`
	AccessToken string `json:"accessToken"`
}

type DailySummary struct {
	ID          string    `json:"id"`
	FamilyID    string    `json:"familyId"`
	Date        string    `json:"date"`
	Content     string    `json:"content"`
	UpdateCount int       `json:"updateCount"`
	CreatedAt   time.Time `json:"createdAt"`
}
