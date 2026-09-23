//go:build linux

package process

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// defaultProcDir is the mount point of the Linux process filesystem.
const defaultProcDir = "/proc"

// minPID is the smallest process ID that TimeLord reports. Lower IDs belong to
// the kernel and to early system services.
const minPID = 1000

// userHZ is the number of clock ticks in one second for /proc/<pid>/stat.
// The Linux kernel fixes this value at 100 for the stat ABI.
const userHZ = 100

// linuxLister reads the process list from a /proc filesystem.
type linuxLister struct {
	procDir string
}

// NewLister returns the Linux process lister.
func NewLister() Lister {
	return &linuxLister{procDir: defaultProcDir}
}

// List returns one entry for each process that is visible in /proc.
// It skips system processes and processes that the service cannot read.
func (l linuxLister) List() ([]Process, error) {
	entries, err := os.ReadDir(l.procDir)
	if err != nil {
		return nil, err
	}

	processes := make([]Process, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a PID directory
		}
		if pid < minPID {
			continue // system process
		}

		pidDir := filepath.Join(l.procDir, entry.Name())

		executable, err := os.Readlink(filepath.Join(pidDir, "exe"))
		if err != nil {
			continue // process has exited or is not accessible
		}

		stat, err := readStat(pidDir)
		if err != nil {
			continue // process has exited
		}

		processes = append(processes, Process{
			PID:         pid,
			User:        ownerName(pidDir),
			Name:        processName(pidDir, executable),
			Executable:  executable,
			MemoryBytes: stat.rssPages * uint64(os.Getpagesize()),
			CPUSeconds:  float64(stat.utime+stat.stime) / userHZ,
			StartTime:   stat.startTime,
		})
	}

	return processes, nil
}

// procStat holds the /proc/<pid>/stat fields that TimeLord uses.
type procStat struct {
	utime     uint64
	stime     uint64
	startTime uint64
	rssPages  uint64
}

// readStat reads the CPU and memory fields from /proc/<pid>/stat.
func readStat(pidDir string) (procStat, error) {
	data, err := os.ReadFile(filepath.Join(pidDir, "stat"))
	if err != nil {
		return procStat{}, err
	}

	// The second field is the process name and can contain spaces and
	// parentheses. Parse the fields after the last ')' instead.
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 {
		return procStat{}, errors.New("invalid stat file")
	}

	// fields[0] is the state, which is field 3 in proc(5).
	// The needed fields are: utime (14), stime (15), starttime (22), rss (24).
	fields := strings.Fields(text[end+1:])
	if len(fields) < 22 {
		return procStat{}, errors.New("short stat file")
	}

	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	rss, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	if rss < 0 {
		rss = 0
	}

	return procStat{
		utime:     utime,
		stime:     stime,
		startTime: startTime,
		rssPages:  uint64(rss),
	}, nil
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
