//go:build linux

package process

import (
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

func TestLinuxListerList(t *testing.T) {
	procDir := t.TempDir()

	writeProcess(t, procDir, "1234", "/usr/bin/bash", "bash\n")
	writeProcess(t, procDir, "1235", "/usr/libexec/at-spi-bus-launcher", "at-spi-bus-laun\n")

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
	if bash.Name != "bash" {
		t.Errorf("bash name = %q, want %q", bash.Name, "bash")
	}
	if bash.User == "" {
		t.Error("bash user is empty")
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
func writeProcess(t *testing.T, procDir, pid, executable, comm string) {
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
}
