//go:build !linux && !windows

package notify

import (
	"context"
	"fmt"

	"timelord/internal/session"
)

type noopNotifier struct{}

func newNotifier() Notifier {
	return noopNotifier{}
}

func (noopNotifier) Notify(context.Context, session.Session, Notice) error {
	return nil
}

func runChild(context.Context, Notice) error {
	return fmt.Errorf("notifications are not supported on this platform")
}
