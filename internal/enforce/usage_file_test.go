package enforce

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"timelord/internal/schedule"
)

func TestReadUsageFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), UsageFileName)
	if err := os.WriteFile(path, []byte("DATE,USED_MINUTES\n2026-11-10,5\n2026-11-09,10\n"), 0o600); err != nil {
		t.Fatalf("write usage file: %v", err)
	}

	used, err := readUsageFile(path, "2026-11-10")
	if err != nil {
		t.Fatalf("readUsageFile() error = %v", err)
	}
	if used != 5*time.Minute {
		t.Errorf("readUsageFile() = %v, want 5m", used)
	}

	used, err = readUsageFile(path, "2026-11-11")
	if err != nil {
		t.Fatalf("readUsageFile(other date) error = %v", err)
	}
	if used != 0 {
		t.Errorf("readUsageFile(other date) = %v, want 0", used)
	}
}

func TestReadUsageFileMissing(t *testing.T) {
	_, err := readUsageFile(filepath.Join(t.TempDir(), UsageFileName), "2026-11-10")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("readUsageFile() error = %v, want fs.ErrNotExist", err)
	}
}

func TestWriteUsageFileRoundsUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), UsageFileName)
	if err := writeUsageFile(path, "2026-11-10", 61*time.Second); err != nil {
		t.Fatalf("writeUsageFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	want := "DATE,USED_MINUTES\n2026-11-10,2\n"
	if string(data) != want {
		t.Errorf("file = %q, want %q", data, want)
	}
}

func TestEngineLoadsUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), UsageFileName)
	now := time.Date(2026, time.November, 10, 10, 0, 0, 0, time.Local)
	if err := writeUsageFile(path, now.Format(dateLayout), 5*time.Minute); err != nil {
		t.Fatalf("write usage file: %v", err)
	}

	store := schedule.NewStore([]string{"dani"})
	engine := NewEngine(&fakeSessionSource{}, store, map[string]string{"dani": path})
	engine.now = func() time.Time { return now }
	engine.loadUsage(now)

	usage := engine.UsageSnapshot()
	if len(usage) != 1 || usage[0].Used != 5*time.Minute {
		t.Fatalf("UsageSnapshot() = %+v, want dani with 5m used", usage)
	}
}

func TestEngineSavesUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), UsageFileName)
	store := schedule.NewStore([]string{"dani"})
	engine := NewEngine(&fakeSessionSource{}, store, map[string]string{"dani": path})

	now := time.Date(2026, time.November, 10, 10, 0, 0, 0, time.Local)
	engine.now = func() time.Time { return now }
	engine.date = "2026-11-10"
	engine.used["dani"] = 7 * time.Minute

	engine.saveUsage()

	used, err := readUsageFile(path, "2026-11-10")
	if err != nil {
		t.Fatalf("read saved usage: %v", err)
	}
	if used != 7*time.Minute {
		t.Errorf("saved usage = %v, want 7m", used)
	}
}

func TestEngineSavesUsageOnShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), UsageFileName)
	store := schedule.NewStore([]string{"dani"})
	engine := NewEngine(&fakeSessionSource{}, store, map[string]string{"dani": path})

	now := time.Date(2026, time.November, 10, 10, 0, 0, 0, time.Local)
	engine.now = func() time.Time { return now }
	engine.date = "2026-11-10"
	engine.used["dani"] = 3 * time.Minute

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine.Run(ctx)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read usage file after Run: %v", err)
	}
	if !strings.Contains(string(data), "2026-11-10,3") {
		t.Errorf("usage file = %q, want 3 minutes", data)
	}
}
