package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProductionStorageRequiresMountedDiskMarker(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APP_ENV", "production")
	t.Setenv("FAMILY_DAILY_STORAGE_DIR", root)
	t.Setenv("DATA_DIR", "")
	if _, err := prepareStorageRoot(); err == nil {
		t.Fatal("expected missing storage marker to fail closed")
	}
	if err := os.WriteFile(filepath.Join(root, storageMarker), []byte("Family Daily local storage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := prepareStorageRoot()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != expected {
		t.Fatalf("resolved storage = %q, want %q", resolved, expected)
	}
	for _, dir := range []string{"media", "backups"} {
		if info, err := os.Stat(filepath.Join(root, dir)); err != nil || !info.IsDir() {
			t.Fatalf("missing %s directory", dir)
		}
	}
}
