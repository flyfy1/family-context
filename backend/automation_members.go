package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (s *store) validateFamilyMemberIDs(ctx context.Context, familyID string, memberIDs []string) error {
	seen := make(map[string]bool, len(memberIDs))
	for _, memberID := range memberIDs {
		if memberID == "" || seen[memberID] {
			return errors.New("invalid member selection")
		}
		seen[memberID] = true
		var storedFamilyID string
		if err := s.db.QueryRowContext(ctx, `SELECT family_id FROM members WHERE id = ?`, memberID).Scan(&storedFamilyID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("invalid member selection")
			}
			return err
		}
		if storedFamilyID != familyID {
			return errors.New("invalid member selection")
		}
	}
	return nil
}

func relationColumns(table, ownerColumn string) (string, string, error) {
	if table == "core_job_rule_recipients" && ownerColumn == "rule_id" {
		return table, ownerColumn, nil
	}
	if table == "scheduled_job_members" && ownerColumn == "job_id" {
		return table, ownerColumn, nil
	}
	return "", "", errors.New("invalid member relation")
}

func (s *store) memberIDsForRelation(ctx context.Context, table, ownerColumn, ownerID string) ([]string, error) {
	table, ownerColumn, err := relationColumns(table, ownerColumn)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT member_id FROM %s WHERE %s = ? ORDER BY rowid`, table, ownerColumn), ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memberIDs := make([]string, 0)
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			return nil, err
		}
		memberIDs = append(memberIDs, memberID)
	}
	return memberIDs, rows.Err()
}

func (s *store) replaceMemberRelation(ctx context.Context, table, ownerColumn, ownerID string, memberIDs []string) error {
	table, ownerColumn, err := relationColumns(table, ownerColumn)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, table, ownerColumn), ownerID); err != nil {
		return err
	}
	for _, memberID := range memberIDs {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s(%s, member_id) VALUES(?, ?)`, table, ownerColumn), ownerID, memberID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
