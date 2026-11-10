// Package session discovers and classifies operating-system login sessions.
package session

import "context"

// Session is one login session as reported by the operating system.
type Session struct {
	ID      string
	User    string
	UID     string
	Seat    string
	Class   string
	Desktop string
	Remote  bool
	Active  bool
	Locked  bool
}

// SessionSource discovers sessions and terminates one session.
type SessionSource interface {
	Sessions(ctx context.Context) ([]Session, error)
	TerminateSession(ctx context.Context, id string) error
}

// IsLocalSession reports whether the session is on a local seat. SSH and other
// remote sessions are not PC use.
func IsLocalSession(s Session) bool {
	return s.Seat != "" && !s.Remote
}

// IsCountedSession reports whether the session contributes to active use. Lock
// screens can be local sessions but must not add time.
func IsCountedSession(s Session) bool {
	if !IsLocalSession(s) {
		return false
	}
	return s.Class == "" || s.Class == "user"
}
