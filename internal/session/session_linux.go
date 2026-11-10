//go:build linux

package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type loginctlSource struct {
	command string
}

// NewSessionSource returns a SessionSource backed by systemd-logind.
func NewSessionSource() (SessionSource, error) {
	return &loginctlSource{command: "loginctl"}, nil
}

func (s *loginctlSource) Sessions(ctx context.Context) ([]Session, error) {
	out, err := exec.CommandContext(ctx, s.command, "list-sessions", "--no-legend", "--no-pager").Output()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	ids := parseSessionIDs(out)
	sessions := make([]Session, 0, len(ids))
	var firstErr error
	for _, id := range ids {
		out, err := exec.CommandContext(ctx, s.command, "show-session", "--no-pager",
			"-p", "Id", "-p", "Name", "-p", "User", "-p", "Seat", "-p", "Remote",
			"-p", "Active", "-p", "LockedHint", "-p", "Class", "-p", "Desktop", id).Output()
		if err != nil {
			// A session can end between list-sessions and show-session.
			if firstErr == nil {
				firstErr = fmt.Errorf("show session %s: %w", id, err)
			}
			continue
		}
		session, err := parseSession(out)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("show session %s: %w", id, err)
			}
			continue
		}
		sessions = append(sessions, session)
	}
	if len(sessions) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return sessions, nil
}

func (s *loginctlSource) TerminateSession(ctx context.Context, id string) error {
	out, err := exec.CommandContext(ctx, s.command, "terminate-session", id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("terminate session %s: %w: %s", id, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseSessionIDs reads the first column of loginctl list-sessions.
func parseSessionIDs(out []byte) []string {
	var ids []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			ids = append(ids, fields[0])
		}
	}
	return ids
}

// parseSession reads the key=value output of loginctl show-session.
func parseSession(out []byte) (Session, error) {
	properties := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		properties[key] = value
	}

	user := properties["Name"]
	if user == "" {
		return Session{}, fmt.Errorf("session has no Name property")
	}

	return Session{
		ID:      properties["Id"],
		User:    user,
		UID:     properties["User"],
		Seat:    properties["Seat"],
		Class:   properties["Class"],
		Desktop: properties["Desktop"],
		Remote:  properties["Remote"] == "yes",
		Active:  properties["Active"] == "yes",
		Locked:  properties["LockedHint"] == "yes",
	}, nil
}
