package process

// Process is a generic snapshot of a running process.
// A snapshot contains only the data that TimeLord needs.
type Process struct {
	// User is the name of the user that owns the process.
	User string
	// Name is the short name of the process.
	Name string
	// Executable is the path of the process executable.
	Executable string
}
