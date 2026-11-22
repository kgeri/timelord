// Package notify delivers short, low-obtrusiveness reminders to logged-in user
// sessions. The Enforcer runs as a service, so platform code launches a helper
// in the child's session instead of calling desktop APIs directly.
package notify

import (
	"context"
	"fmt"

	"timelord/internal/session"
)

// Kind identifies why a notice is shown.
type Kind string

const (
	// KindLimit is a warning about the daily usage limit.
	KindLimit Kind = "limit"
	// KindWindow is a warning about the end of a permitted time window.
	KindWindow Kind = "window"
)

// Notice is one short reminder shown to the child.
type Notice struct {
	Kind    Kind
	Minutes int
}

// Title returns the notification title.
func (n Notice) Title() string {
	return "TimeLord"
}

// Body returns the notification body.
func (n Notice) Body() string {
	switch n.Kind {
	case KindWindow:
		if n.Minutes <= 0 {
			return "The time window is closed."
		}
		return fmt.Sprintf("The computer locks in %s.", duration(n.Minutes))
	default:
		if n.Minutes <= 0 {
			return "Time is up for today."
		}
		return fmt.Sprintf("%s left today.", duration(n.Minutes))
	}
}

func duration(minutes int) string {
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// NewNotice validates notice arguments and returns a Notice.
func NewNotice(kind string, minutes int) (Notice, error) {
	switch Kind(kind) {
	case KindLimit, KindWindow:
	default:
		return Notice{}, fmt.Errorf("unknown notice kind %q", kind)
	}
	if minutes < 0 || minutes > 24*60 {
		return Notice{}, fmt.Errorf("notice minutes %d is out of range", minutes)
	}
	return Notice{Kind: Kind(kind), Minutes: minutes}, nil
}

// Notifier delivers a notice to one live session. Implementations must not
// block the enforcement loop for long.
type Notifier interface {
	Notify(ctx context.Context, sess session.Session, notice Notice) error
}

// New returns the platform notifier. Unsupported platforms get a no-op.
func New() Notifier {
	return newNotifier()
}

// RunChild shows one notice in the current process's session. It backs the
// hidden -notify mode that the platform notifier launches in the user session.
func RunChild(ctx context.Context, kind string, minutes int) error {
	notice, err := NewNotice(kind, minutes)
	if err != nil {
		return err
	}
	return runChild(ctx, notice)
}
