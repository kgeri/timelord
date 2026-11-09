package schedule

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileProviderLoadUsesHomeOwnerPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	writeSchedule(t, path, `{
		"users": [
			{"user": "someone-else", "daily_limit_minutes": 10},
			{"user": "dani", "daily_limit_minutes": 60}
		]
	}`)

	provider := NewFileProvider([]File{{User: "dani", Path: path}})
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(config.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(config.Users))
	}
	if config.Users[0].User != "dani" {
		t.Errorf("policy user = %q, want dani", config.Users[0].User)
	}
}

func TestFileProviderMissingFileIsEmpty(t *testing.T) {
	provider := NewFileProvider([]File{{
		User: "dani",
		Path: filepath.Join(t.TempDir(), FileName),
	}})

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(config.Users) != 0 {
		t.Errorf("len(Users) = %d, want 0", len(config.Users))
	}
}

func TestFileProviderWatchReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	writeSchedule(t, path, `{"users":[{"user":"dani","daily_limit_minutes":60}]}`)

	provider := NewFileProvider([]File{{User: "dani", Path: path}})
	updates := make(chan *Config, 4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go provider.Watch(ctx, func(config *Config, err error) {
		if err != nil {
			t.Errorf("Watch() update error = %v", err)
			return
		}
		updates <- config
	})

	// Give the watcher time to register the directory before changing the file.
	time.Sleep(50 * time.Millisecond)
	writeSchedule(t, path, `{"users":[{"user":"dani","daily_limit_minutes":30}]}`)

	select {
	case config := <-updates:
		if len(config.Users) != 1 || config.Users[0].DailyLimitMinutes != 30 {
			t.Fatalf("updated config = %+v, want dani with a 30 minute limit", config.Users)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a schedule reload")
	}
}

func TestFileProviderWatchReloadsAfterRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	writeSchedule(t, path, `{"users":[{"user":"dani","daily_limit_minutes":60}]}`)

	provider := NewFileProvider([]File{{User: "dani", Path: path}})
	updates := make(chan *Config, 4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go provider.Watch(ctx, func(config *Config, err error) {
		if err != nil {
			t.Errorf("Watch() update error = %v", err)
			return
		}
		updates <- config
	})

	time.Sleep(50 * time.Millisecond)
	replacement := filepath.Join(dir, "replacement.json")
	writeSchedule(t, replacement, `{"users":[{"user":"dani","daily_limit_minutes":15}]}`)
	if err := os.Rename(replacement, path); err != nil {
		t.Fatalf("rename schedule: %v", err)
	}

	select {
	case config := <-updates:
		if len(config.Users) != 1 || config.Users[0].DailyLimitMinutes != 15 {
			t.Fatalf("updated config = %+v, want dani with a 15 minute limit", config.Users)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a schedule reload")
	}
}

func writeSchedule(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write schedule: %v", err)
	}
}
