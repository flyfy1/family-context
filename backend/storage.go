package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const storageMarker = ".family-daily-storage"

func prepareStorageRoot() (string, error) {
	configured := os.Getenv("FAMILY_DAILY_STORAGE_DIR")
	if configured == "" {
		configured = os.Getenv("DATA_DIR") // Backward-compatible local override.
	}
	production := strings.EqualFold(os.Getenv("APP_ENV"), "production")
	if configured == "" {
		if production {
			return "", errors.New("FAMILY_DAILY_STORAGE_DIR is required in production")
		}
		configured = "data"
	}
	if production && !filepath.IsAbs(configured) {
		return "", errors.New("FAMILY_DAILY_STORAGE_DIR must be absolute in production")
	}

	root, err := filepath.Abs(configured)
	if err != nil {
		return "", fmt.Errorf("resolve storage directory: %w", err)
	}
	root = filepath.Clean(root)
	if root == string(filepath.Separator) {
		return "", errors.New("refusing to use the filesystem root as storage")
	}
	if production {
		info, err := os.Stat(root)
		if err != nil {
			return "", fmt.Errorf("dedicated storage is unavailable: %w", err)
		}
		if !info.IsDir() {
			return "", errors.New("dedicated storage path is not a directory")
		}
		markerInfo, err := os.Stat(filepath.Join(root, storageMarker))
		if err != nil || !markerInfo.Mode().IsRegular() {
			return "", fmt.Errorf("dedicated storage marker %s is missing", storageMarker)
		}
	} else if err := os.MkdirAll(root, 0o750); err != nil {
		return "", fmt.Errorf("create storage directory: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve storage symlinks: %w", err)
	}
	for _, dir := range []string{"media", "backups"} {
		if err := os.MkdirAll(filepath.Join(resolved, dir), 0o750); err != nil {
			return "", fmt.Errorf("create local storage directory: %w", err)
		}
	}
	probe, err := os.CreateTemp(resolved, ".write-check-")
	if err != nil {
		return "", fmt.Errorf("dedicated storage is not writable: %w", err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(probePath); err != nil {
		return "", err
	}
	return resolved, nil
}

func writeFileAtomically(dir, name string, data []byte) error {
	temp, err := os.CreateTemp(dir, ".incoming-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o640); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return os.Rename(tempPath, filepath.Join(dir, name))
}
