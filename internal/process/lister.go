package process

// Lister reads the process list from the operating system.
// Each operating system supplies its own implementation.
type Lister interface {
	// List returns one entry for each running process.
	List() ([]Process, error)
}
