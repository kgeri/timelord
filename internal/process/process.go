package process

// Scope is the cross-platform classification of what started a process. The
// values are deliberately coarse so that the same labels work on Linux and
// Windows and dashboards do not depend on platform details.
type Scope int

const (
	// ScopeUnknown is the zero value, used when a lister cannot classify a
	// process.
	ScopeUnknown Scope = iota
	// ScopeApp marks a user-facing application.
	ScopeApp
	// ScopeSystem marks every other process. On Linux this includes the user's
	// session and background services; on Windows it includes session 0, system
	// accounts, and Windows components under %SystemRoot%.
	ScopeSystem
)

// String returns the Prometheus label value for the scope.
func (s Scope) String() string {
	switch s {
	case ScopeApp:
		return "app"
	case ScopeSystem:
		return "system"
	default:
		return "unknown"
	}
}

// Process is a generic snapshot of a running process.
// A snapshot contains only the data that TimeLord needs.
type Process struct {
	// PID is the process ID.
	PID int
	// User is the name of the user that owns the process.
	User string
	// Name is the short name of the process.
	Name string
	// Executable is the path of the process executable.
	Executable string
	// Scope is what started the process, such as an app or a system service.
	Scope Scope
	// MemoryBytes is the resident memory of the process in bytes.
	MemoryBytes uint64
	// CPUSeconds is the total CPU time of the process since it started.
	CPUSeconds float64
	// StartTime identifies the process instance. TimeLord uses it to detect a
	// reused PID.
	StartTime uint64
}
