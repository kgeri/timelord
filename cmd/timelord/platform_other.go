//go:build !windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// runPlatform runs the process loop until the process is interrupted. It exits
// with status 1 when run returns an error.
func runPlatform(run func(context.Context) error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Printf("TimeLord failed: %v", err)
		os.Exit(1)
	}
}

// isPrivileged reports whether TimeLord can see processes of other users.
func isPrivileged() bool {
	return os.Geteuid() == 0
}
