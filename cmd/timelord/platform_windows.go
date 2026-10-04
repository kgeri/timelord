//go:build windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// serviceName is the name that the Windows service control manager uses.
const serviceName = "TimeLord"

// runPlatform runs the process loop. When the service control manager started
// the binary it runs as a Windows service; otherwise it runs in the console
// until interrupted, which keeps local debugging simple.
func runPlatform(run func(context.Context) error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("failed to detect the Windows service environment: %v", err)
	}

	if !isService {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		if err := run(ctx); err != nil {
			log.Printf("TimeLord failed: %v", err)
			os.Exit(1)
		}
		return
	}

	setServiceLogging()

	if err := svc.Run(serviceName, serviceHandler{run: run}); err != nil {
		log.Fatalf("Windows service failed: %v", err)
	}
}

// isPrivileged reports whether TimeLord runs with administrator rights. A
// service runs as LocalSystem, which is a member of the Administrators group.
func isPrivileged() bool {
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}

	member, err := windows.GetCurrentProcessToken().IsMember(admins)
	return err == nil && member
}

// setServiceLogging sends the log to a file, because a service has no console.
func setServiceLogging() {
	dir := filepath.Join(os.Getenv("ProgramData"), "TimeLord")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	file, err := os.OpenFile(filepath.Join(dir, "timelord.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(file)
}

// serviceHandler adapts the process loop to the Windows service control manager.
type serviceHandler struct {
	run func(context.Context) error
}

// Execute runs the process loop until the service manager requests a stop. A
// self-test failure stops the service with a non-zero service-specific exit
// code so the failure is visible to the service manager.
func (h serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- h.run(ctx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}
	for {
		select {
		case err := <-done:
			changes <- svc.Status{State: svc.StopPending}
			return serviceExitCode(err)
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				cancel()
				err := <-done
				changes <- svc.Status{State: svc.StopPending}
				return serviceExitCode(err)
			}
		}
	}
}

// serviceExitCode reports a failed run to the service manager.
func serviceExitCode(err error) (bool, uint32) {
	if err != nil {
		log.Printf("TimeLord failed: %v", err)
		return true, 1
	}
	return false, 0
}
