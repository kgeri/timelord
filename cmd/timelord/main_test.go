package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnforcerKey(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyFile, []byte("file-key\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	t.Run("explicit file wins", func(t *testing.T) {
		t.Setenv("TIMELORD_ENFORCER_KEY", "env-key")
		key, err := loadEnforcerKey(keyFile)
		if err != nil {
			t.Fatalf("loadEnforcerKey() error = %v", err)
		}
		if key != "file-key" {
			t.Errorf("key = %q, want file-key", key)
		}
	})

	t.Run("key file environment variable", func(t *testing.T) {
		t.Setenv("TIMELORD_ENFORCER_KEY", "")
		t.Setenv("TIMELORD_ENFORCER_KEY_FILE", keyFile)
		key, err := loadEnforcerKey("")
		if err != nil {
			t.Fatalf("loadEnforcerKey() error = %v", err)
		}
		if key != "file-key" {
			t.Errorf("key = %q, want file-key", key)
		}
	})

	t.Run("environment variable", func(t *testing.T) {
		t.Setenv("TIMELORD_ENFORCER_KEY", "env-key")
		t.Setenv("TIMELORD_ENFORCER_KEY_FILE", "")
		key, err := loadEnforcerKey("")
		if err != nil {
			t.Fatalf("loadEnforcerKey() error = %v", err)
		}
		if key != "env-key" {
			t.Errorf("key = %q, want env-key", key)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Setenv("TIMELORD_ENFORCER_KEY", "")
		t.Setenv("TIMELORD_ENFORCER_KEY_FILE", "")
		if _, err := loadEnforcerKey(""); err == nil {
			t.Fatal("loadEnforcerKey() error = nil, want error")
		}
	})
}
