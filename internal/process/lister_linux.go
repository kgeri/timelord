//go:build linux

package process

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// defaultProcDir is the mount point of the Linux process filesystem.
const defaultProcDir = "/proc"

// linuxLister reads the process list from a /proc filesystem.
type linuxLister struct {
	procDir string
}

// NewLister returns the Linux process lister.
func NewLister() Lister {
	return &linuxLister{procDir: defaultProcDir}
}

// List returns one entry for each process that is visible in /proc.
// It skips processes that have exited or that the service cannot read.
func (l linuxLister) List() ([]Process, error) {
	entries, err := os.ReadDir(l.procDir)
	if err != nil {
		return nil, err
	}

	processes := make([]Process, 0, len(entries))
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue // not a PID directory
		}

		pidDir := filepath.Join(l.procDir, entry.Name())

		executable, err := os.Readlink(filepath.Join(pidDir, "exe"))
		if err != nil {
			continue // process has exited or is not accessible
		}

		processes = append(processes, Process{
			User:       ownerName(pidDir),
			Name:       processName(pidDir, executable),
			Executable: executable,
		})
	}

	return processes, nil
}

// ownerName returns the name of the user that owns the process.
// It returns the numeric user ID when the name is not found.
func ownerName(pidDir string) string {
	info, err := os.Stat(pidDir)
	if err != nil {
		return ""
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	uid := strconv.FormatUint(uint64(stat.Uid), 10)
	u, err := user.LookupId(uid)
	if err != nil {
		return uid
	}
	return u.Username
}

// commMaxLen is the maximum length of the Linux process comm field.
// The kernel stores at most 15 characters in this field.
const commMaxLen = 15

// processName returns the short name of the process from /proc.
// The kernel cuts the comm field to 15 characters, so it falls back to the
// base name of the executable path when the field is cut or absent.
func processName(pidDir, executable string) string {
	data, err := os.ReadFile(filepath.Join(pidDir, "comm"))
	if err == nil {
		if name := strings.TrimSpace(string(data)); name != "" && len(name) < commMaxLen {
			return name
		}
	}
	return filepath.Base(executable)
}
