// Package selftest runs the startup checks that prove TimeLord can collect
// process data and serve metrics. TimeLord refuses to start when a check
// fails, so a broken deployment fails loudly instead of reporting nothing.
package selftest

import (
	"errors"
	"fmt"
	"log"

	"timelord/internal/process"
)

// Config holds the checks that Run performs. The functions are injected so the
// checks are easy to test.
type Config struct {
	// Privileged reports whether TimeLord can see processes of other users.
	Privileged bool
	// List returns the current process list.
	List func() ([]process.Process, error)
	// Listen opens the metrics endpoint.
	Listen func() error
}

// Run performs the startup checks, logging each one. It returns the first
// failure, or nil when TimeLord can collect and serve data.
func Run(cfg Config) error {
	if !cfg.Privileged {
		return errors.New("not running with administrative rights; processes of other users are not visible")
	}
	log.Println("self-test: administrative rights OK")

	processes, err := cfg.List()
	if err != nil {
		return fmt.Errorf("cannot list processes: %w", err)
	}
	if len(processes) == 0 {
		return errors.New("process listing returned no processes")
	}

	readable := 0
	for _, p := range processes {
		if p.Executable != "" {
			readable++
		}
	}
	if readable == 0 {
		return errors.New("cannot read the executable of any process")
	}
	log.Printf("self-test: process listing OK (%d processes, %d with an executable)", len(processes), readable)

	if err := cfg.Listen(); err != nil {
		return fmt.Errorf("cannot open the metrics endpoint: %w", err)
	}
	log.Println("self-test: metrics endpoint OK")

	return nil
}
