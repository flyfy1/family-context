package main

import "testing"

func TestValidMemberRole(t *testing.T) {
	for _, role := range []string{"member", "elder", "child"} {
		if !validMemberRole(role) {
			t.Fatalf("expected %q to be valid", role)
		}
	}
	for _, role := range []string{"", "admin", "kid"} {
		if validMemberRole(role) {
			t.Fatalf("expected %q to be invalid", role)
		}
	}
}
