package session

import "strconv"

// Windows Terminal Services constants. They are kept here so the mapping from
// WTS state to a Session can be tested on every platform.
const (
	wtsStateActive uint32 = 0

	// WTS_SESSIONSTATE_LOCK is zero and WTS_SESSIONSTATE_UNLOCK is one. A
	// failed query must not be interpreted as a lock, so the caller passes
	// FlagsKnown separately.
	wtsSessionStateLock   int32 = 0
	wtsSessionStateUnlock int32 = 1
)

// wtsSessionData is the platform-independent input for a Windows session.
type wtsSessionData struct {
	ID         uint32
	User       string
	Seat       string
	State      uint32
	Flags      int32
	FlagsKnown bool
}

// wtsSessionFromData converts a WTS session into the shared Session model.
// Windows sessions are treated as local. The session flags are only
// meaningful for an active session.
func wtsSessionFromData(data wtsSessionData) Session {
	return Session{
		ID:     strconv.FormatUint(uint64(data.ID), 10),
		User:   data.User,
		Seat:   data.Seat,
		Active: data.State == wtsStateActive,
		Locked: data.FlagsKnown && data.State == wtsStateActive && data.Flags == wtsSessionStateLock,
	}
}
