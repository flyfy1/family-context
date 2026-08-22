package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvDoesNotOverrideExistingValue(t *testing.T) {
	name := "FAMILY_DAILY_ENV_TEST"
	t.Setenv(name, "existing")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(name+"=from-file\nFAMILY_DAILY_SECOND=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv("FAMILY_DAILY_SECOND") })
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(name); got != "existing" {
		t.Fatalf("existing value was replaced: %q", got)
	}
	if got := os.Getenv("FAMILY_DAILY_SECOND"); got != "value" {
		t.Fatalf("second value = %q", got)
	}
}
