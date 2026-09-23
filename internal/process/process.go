package process

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
	// MemoryBytes is the resident memory of the process in bytes.
	MemoryBytes uint64
	// CPUSeconds is the total CPU time of the process since it started.
	CPUSeconds float64
	// StartTime identifies the process instance. TimeLord uses it to detect a
	// reused PID.
	StartTime uint64
}
