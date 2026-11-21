//go:build windows

package session

import (
	"context"
	"fmt"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// wtsCurrentServerHandle is WTS_CURRENT_SERVER_HANDLE.
	wtsCurrentServerHandle = 0

	// WTS_INFO_CLASS values used by TimeLord.
	wtsUserName = 5
	wtsInfoEx   = 25

	// WTS_INFOEX_LEVEL1 is the WTSINFOEX level that carries SessionFlags.
	wtsInfoExLevel1 = 1
)

var (
	modwtsapi32                     = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySessionInformationW = modwtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSLogoffSession            = modwtsapi32.NewProc("WTSLogoffSession")
)

// wtsSource reads sessions from the Windows Terminal Services API.
type wtsSource struct{}

// NewSessionSource returns a SessionSource backed by Windows Terminal Services.
func NewSessionSource() (SessionSource, error) {
	return wtsSource{}, nil
}

// Sessions returns one entry for each logged-on Windows session. Session 0 is
// the services session and has no interactive user, so it is skipped.
func (wtsSource) Sessions(ctx context.Context) ([]Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var infos *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(wtsCurrentServerHandle, 0, 1, &infos, &count); err != nil {
		return nil, fmt.Errorf("enumerate sessions: %w", err)
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(infos)))

	if infos == nil || count == 0 {
		return nil, nil
	}

	entries := unsafe.Slice(infos, int(count))
	sessions := make([]Session, 0, len(entries))
	var firstErr error

	for i := range entries {
		info := entries[i]
		if info.SessionID == 0 {
			continue // services session
		}

		user, err := querySessionString(info.SessionID, wtsUserName)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("session %d user: %w", info.SessionID, err)
			}
			continue
		}
		if user == "" {
			continue // logon screen or a session that is still starting
		}

		seat := "console"
		if info.WindowStationName != nil {
			if name := windows.UTF16PtrToString(info.WindowStationName); name != "" {
				seat = name
			}
		}

		flags, flagsKnown := querySessionFlags(info.SessionID)
		sessions = append(sessions, wtsSessionFromData(wtsSessionData{
			ID:         info.SessionID,
			User:       user,
			Seat:       seat,
			State:      info.State,
			Flags:      flags,
			FlagsKnown: flagsKnown,
		}))
	}

	if len(sessions) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return sessions, nil
}

// TerminateSession logs off one Windows session. Logging off ends the session
// and closes the user's programs, like loginctl terminate-session on Linux.
func (wtsSource) TerminateSession(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	sessionID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid Windows session ID %q: %w", id, err)
	}

	r1, _, e1 := procWTSLogoffSession.Call(wtsCurrentServerHandle, uintptr(sessionID), 0)
	if r1 == 0 {
		return wtsError("logoff session "+id, e1)
	}
	return nil
}

// querySessionString reads a string-valued WTS session property.
func querySessionString(sessionID uint32, class uintptr) (string, error) {
	var buffer *uint16
	var size uint32
	r1, _, e1 := procWTSQuerySessionInformationW.Call(
		wtsCurrentServerHandle,
		uintptr(sessionID),
		class,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		return "", wtsError("query session information", e1)
	}
	if buffer == nil {
		return "", nil
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	return windows.UTF16PtrToString(buffer), nil
}

// wtsInfoExHeader is the prefix of WTSINFOEX_LEVEL1_W that TimeLord reads.
// The WTSINFOEX Data union starts at offset 8 because it contains 64-bit
// fields, hence the explicit padding after Level.
type wtsInfoExHeader struct {
	Level        uint32
	_            uint32
	SessionID    uint32
	SessionState uint32
	SessionFlags int32
}

// querySessionFlags reads the lock/unlock flag from WTSInfoEx. A false result
// means the flag is unknown; callers must not treat that as locked.
func querySessionFlags(sessionID uint32) (int32, bool) {
	var buffer *byte
	var size uint32
	r1, _, _ := procWTSQuerySessionInformationW.Call(
		wtsCurrentServerHandle,
		uintptr(sessionID),
		uintptr(wtsInfoEx),
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 || buffer == nil || size < uint32(unsafe.Sizeof(wtsInfoExHeader{})) {
		return 0, false
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))

	header := (*wtsInfoExHeader)(unsafe.Pointer(buffer))
	if header.Level != wtsInfoExLevel1 {
		return 0, false
	}
	return header.SessionFlags, true
}

// wtsError converts the error returned by LazyProc.Call into a useful error.
// A zero syscall.Errno is not nil but contains no message.
func wtsError(operation string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("%s failed", operation)
	}
	if err == nil {
		return fmt.Errorf("%s failed", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
