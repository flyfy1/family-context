package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// loadDotEnv loads the first existing file and never replaces variables that
// are already present in the process environment.
func loadDotEnv(paths ...string) error {
	for _, path := range paths {
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("open environment file: %w", err)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			name, value, ok := strings.Cut(line, "=")
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				file.Close()
				return fmt.Errorf("invalid environment entry in %s", path)
			}
			value = strings.TrimSpace(value)
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
			if _, exists := os.LookupEnv(name); !exists {
				if err := os.Setenv(name, value); err != nil {
					file.Close()
					return fmt.Errorf("set environment variable: %w", err)
				}
			}
		}
		err = scanner.Err()
		file.Close()
		if err != nil {
			return fmt.Errorf("read environment file: %w", err)
		}
	}
	return nil
}
