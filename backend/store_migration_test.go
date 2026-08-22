package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrateAddsAdministratorCapabilityToExistingMembers(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "family.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE members (
		id TEXT PRIMARY KEY,
		family_id TEXT NOT NULL,
		name TEXT NOT NULL,
		role TEXT NOT NULL,
		color TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO members(id, family_id, name, role, color, created_at) VALUES('one', 'our-family', '洋宇', 'member', '#AD4C34', '2026-08-22T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	members, err := store.listMembers(t.Context(), defaultFamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].IsAdmin {
		t.Fatalf("unexpected migrated member: %+v", members)
	}
}

func TestMigrateClearsAttentionFromNonElders(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "family.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE members (
		id TEXT PRIMARY KEY,
		family_id TEXT NOT NULL,
		name TEXT NOT NULL,
		role TEXT NOT NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		needs_attention INTEGER NOT NULL DEFAULT 0,
		color TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO members(id, family_id, name, role, needs_attention, color, created_at)
		VALUES('one', 'our-family', '妈妈', 'member', 1, '#AD4C34', '2026-08-22T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	members, err := store.listMembers(t.Context(), defaultFamilyID)
	if err != nil || len(members) != 1 || members[0].NeedsAttention {
		t.Fatalf("non-elder attention was not cleared: members=%+v err=%v", members, err)
	}
}
