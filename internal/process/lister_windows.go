//go:build windows

package process

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// psapi exposes GetProcessMemoryInfo, which golang.org/x/sys/windows does not
// wrap. It lives in psapi.dll on every supported Windows version.
var (
	modpsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modpsapi.NewProc("GetProcessMemoryInfo")
)

// windowsLister reads the process list with the Windows process snapshot API.
type windowsLister struct {
	systemPaths []string
}

// NewLister returns the Windows process lister. It enables SeDebugPrivilege so
// that memory of processes owned by other users is visible, like root on Linux.
func NewLister() Lister {
	enableDebugPrivilege()
	return windowsLister{systemPaths: windowsSystemPaths()}
}

// List returns one entry for each process that TimeLord can open. It skips the
// Idle and System pseudo processes and processes that have already exited.
func (l windowsLister) List() ([]Process, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	processes := make([]Process, 0, 256)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		pid := int(entry.ProcessID)
		if pid != 0 && pid != 4 { // 0 is Idle, 4 is System
			if p, ok := readProcess(pid, windows.UTF16ToString(entry.ExeFile[:]), l.systemPaths); ok {
				processes = append(processes, p)
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}

	// Process32Next ends the iteration with ERROR_NO_MORE_FILES.
	return processes, nil
}

// readProcess reads the data that TimeLord needs for one process. It returns
// false when the process exits or cannot be opened between enumeration and the
// read.
func readProcess(pid int, fallbackName string, systemPaths []string) (Process, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return Process{}, false
	}
	defer windows.CloseHandle(handle)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return Process{}, false
	}

	owner, systemOwner := ownerName(handle)

	name := fallbackName
	executable := imagePath(handle)
	if executable != "" {
		name = filepath.Base(executable)
	}

	var session uint32
	scope := ScopeSystem
	if err := windows.ProcessIdToSessionId(uint32(pid), &session); err == nil {
		scope = windowsScope(session, executable, systemOwner, systemPaths)
	}

	return Process{
		PID:         pid,
		User:        owner,
		Name:        name,
		Executable:  executable,
		Scope:       scope,
		MemoryBytes: workingSet(pid),
		CPUSeconds:  filetimeSeconds(kernel) + filetimeSeconds(user),
		StartTime:   filetimeValue(creation),
	}, true
}

// imagePath returns the full path of the process executable.
func imagePath(handle windows.Handle) string {
	// Most paths fit in MAX_PATH. Fall back to the maximum NT path length for
	// executables in long-path locations.
	for _, size := range []uint32{windows.MAX_PATH, 32768} {
		buf := make([]uint16, size)
		n := uint32(len(buf))
		if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &n); err == nil {
			return windows.UTF16ToString(buf[:n])
		}
	}
	return ""
}

// ownerName returns the name of the user that owns the process, and whether
// the owner is a system or service account. It returns an empty name when the
// owner cannot be read.
func ownerName(handle windows.Handle) (string, bool) {
	var token windows.Token
	if err := windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
		return "", false
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return "", false
	}

	system := isSystemSID(user.User.Sid)
	account, _, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		return user.User.Sid.String(), system
	}
	return account, system
}

// windowsSystemPaths returns the path prefixes that mark a process as a system
// process: the Windows directory, the Defender platform directory, and the
// Windows inbox app packages that share WindowsApps with Store apps.
func windowsSystemPaths() []string {
	paths := []string{`C:\Windows\`}
	if root := os.Getenv("SystemRoot"); root != "" {
		paths = append(paths, root+`\`)
	}
	if data := os.Getenv("ProgramData"); data != "" {
		paths = append(paths, filepath.Join(data, "Microsoft", "Windows Defender")+`\`)
	}

	for _, programFiles := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if programFiles == "" {
			continue
		}
		paths = append(paths, filepath.Join(programFiles, "Microsoft", "EdgeWebView")+`\`)

		windowsApps := filepath.Join(programFiles, "WindowsApps") + `\`
		for _, pkg := range windowsInboxPackages {
			paths = append(paths, windowsApps+pkg)
		}
	}

	return paths
}

// windowsInboxPackages are Windows inbox app package name prefixes. They live
// under WindowsApps next to user-installed Store apps, so they are matched by
// package name rather than by directory. Add to this list when another Windows
// component shows up as an app.
var windowsInboxPackages = []string{
	"MicrosoftWindows.Client.", // Shell and widgets
	"Microsoft.WidgetsPlatformRuntime",
	"Microsoft.StartExperiencesApp",
}

// systemSIDPrefixes are the well-known SIDs of system, service, and virtual
// accounts that never represent an interactive user. Localised account names
// make name checks unreliable, so TimeLord checks the SID instead.
var systemSIDPrefixes = []string{
	"S-1-5-18",  // Local System
	"S-1-5-19",  // Local Service
	"S-1-5-20",  // Network Service
	"S-1-5-80-", // NT SERVICE
	"S-1-5-83-", // Virtual Users
	"S-1-5-90-", // Window Manager (DWM)
	"S-1-5-98-", // Font Driver Host (UMFD)
}

// isSystemSID reports whether sid belongs to a system, service, or virtual
// account rather than an interactive user.
func isSystemSID(sid *windows.SID) bool {
	value := sid.String()
	for _, prefix := range systemSIDPrefixes {
		if value == prefix || strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// workingSet returns the resident memory of the process in bytes.
// It returns zero when the process cannot be opened for memory reads.
func workingSet(pid int) uint64 {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(handle)

	var counters processMemoryCounters
	counters.cb = uint32(unsafe.Sizeof(counters))
	r1, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.cb),
	)
	if r1 == 0 {
		return 0
	}
	return uint64(counters.workingSetSize)
}

// processMemoryCounters matches PROCESS_MEMORY_COUNTERS from psapi.h.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// filetimeSeconds converts a FILETIME to seconds.
func filetimeSeconds(ft windows.Filetime) float64 {
	return float64(ft.Nanoseconds()) / 1e9
}

// filetimeValue returns a FILETIME as a single integer. TimeLord uses it to
// detect a reused process ID.
func filetimeValue(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

// enableDebugPrivilege turns on SeDebugPrivilege for the current process.
// LocalSystem holds the privilege but starts with it disabled. Enabling it lets
// TimeLord read memory of processes owned by other users.
func enableDebugPrivilege() {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return
	}
	defer token.Close()

	name, err := windows.UTF16PtrFromString("SeDebugPrivilege")
	if err != nil {
		return
	}

	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return
	}

	state := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges: [1]windows.LUIDAndAttributes{
			{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED},
		},
	}
	_ = windows.AdjustTokenPrivileges(token, false, &state, 0, nil, nil)
}
