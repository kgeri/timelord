//go:build linux

package process

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessName(t *testing.T) {
	tests := []struct {
		name       string
		comm       string
		executable string
		want       string
	}{
		{"short comm", "bash\n", "/usr/bin/bash", "bash"},
		{"cut comm uses executable", "at-spi-bus-laun\n", "/usr/libexec/at-spi-bus-launcher", "at-spi-bus-launcher"},
		{"missing comm uses executable", "", "/opt/google/chrome/chrome", "chrome"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pidDir := t.TempDir()
			if tt.comm != "" {
				if err := os.WriteFile(filepath.Join(pidDir, "comm"), []byte(tt.comm), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if got := processName(pidDir, tt.executable); got != tt.want {
				t.Errorf("processName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadStat(t *testing.T) {
	dir := t.TempDir()

	// The comm field contains spaces and parentheses.
	line := "1234 (my (weird) proc) R 0 0 0 0 0 0 0 0 0 0 7 8 0 0 20 0 1 0 42 0 3"
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readStat(dir)
	if err != nil {
		t.Fatalf("readStat() error = %v", err)
	}
	if got.utime != 7 {
		t.Errorf("utime = %d, want 7", got.utime)
	}
	if got.stime != 8 {
		t.Errorf("stime = %d, want 8", got.stime)
	}
	if got.startTime != 42 {
		t.Errorf("startTime = %d, want 42", got.startTime)
	}
	if got.rssPages != 3 {
		t.Errorf("rssPages = %d, want 3", got.rssPages)
	}
}

func TestLinuxListerList(t *testing.T) {
	procDir := t.TempDir()

	writeProcess(t, procDir, "1234", "/usr/bin/bash", "bash\n", 100, 50, 7, 3)
	writeProcess(t, procDir, "1235", "/usr/libexec/at-spi-bus-launcher", "at-spi-bus-laun\n", 0, 0, 9, 1)

	// A system process is not reported.
	writeProcess(t, procDir, "999", "/usr/lib/systemd/systemd", "systemd\n", 0, 0, 1, 1)
	// A process without an executable link is not accessible.
	if err := os.Mkdir(filepath.Join(procDir, "1236"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-numeric entry is not a PID directory.
	if err := os.Mkdir(filepath.Join(procDir, "not-a-pid"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := (linuxLister{procDir: procDir}).List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() returned %d processes, want 2", len(got))
	}

	byExecutable := make(map[string]Process, len(got))
	for _, p := range got {
		byExecutable[p.Executable] = p
	}

	bash, ok := byExecutable["/usr/bin/bash"]
	if !ok {
		t.Fatal("bash process is missing")
	}
	if bash.PID != 1234 {
		t.Errorf("bash pid = %d, want 1234", bash.PID)
	}
	if bash.Name != "bash" {
		t.Errorf("bash name = %q, want %q", bash.Name, "bash")
	}
	if bash.User == "" {
		t.Error("bash user is empty")
	}
	if bash.CPUSeconds != 1.5 {
		t.Errorf("bash cpu = %v, want 1.5", bash.CPUSeconds)
	}
	if bash.MemoryBytes != 3*uint64(os.Getpagesize()) {
		t.Errorf("bash memory = %d, want %d", bash.MemoryBytes, 3*uint64(os.Getpagesize()))
	}
	if bash.StartTime != 7 {
		t.Errorf("bash start time = %d, want 7", bash.StartTime)
	}

	atSpi, ok := byExecutable["/usr/libexec/at-spi-bus-launcher"]
	if !ok {
		t.Fatal("at-spi process is missing")
	}
	if atSpi.Name != "at-spi-bus-launcher" {
		t.Errorf("at-spi name = %q, want %q", atSpi.Name, "at-spi-bus-launcher")
	}
}

// writeProcess makes a fake /proc/<pid> directory.
func writeProcess(t *testing.T, procDir, pid, executable, comm string, utime, stime, startTime, rss int) {
	t.Helper()

	pidDir := filepath.Join(procDir, pid)
	if err := os.Mkdir(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(pidDir, "exe")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "comm"), []byte(comm), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fields after the comm: state, ppid..cmajflt, utime, stime, cutime,
	// cstime, priority, nice, num_threads, itrealvalue, starttime, vsize, rss.
	stat := fmt.Sprintf("%s (proc) R 0 0 0 0 0 0 0 0 0 0 %d %d 0 0 20 0 1 0 %d 0 %d",
		pid, utime, stime, startTime, rss)
	if err := os.WriteFile(filepath.Join(pidDir, "stat"), []byte(stat), 0o644); err != nil {
		t.Fatal(err)
	}
}
