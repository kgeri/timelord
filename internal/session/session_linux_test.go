//go:build linux

package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSessionIDs(t *testing.T) {
	out := []byte("1 1000 dani seat0 tty2\nc2 1001 alice seat0 tty7\n\n")
	ids := parseSessionIDs(out)
	if len(ids) != 2 || ids[0] != "1" || ids[1] != "c2" {
		t.Fatalf("parseSessionIDs() = %v, want [1 c2]", ids)
	}
}

func TestParseSession(t *testing.T) {
	out := []byte(`Id=1
User=1000
Name=dani
Seat=seat0
Remote=no
Active=yes
LockedHint=yes
Class=user
Desktop=gnome
`)
	session, err := parseSession(out)
	if err != nil {
		t.Fatalf("parseSession() error = %v", err)
	}
	if session.ID != "1" || session.User != "dani" || session.UID != "1000" {
		t.Errorf("parseSession() = %+v", session)
	}
	if !session.Active || !session.Locked {
		t.Errorf("parseSession() state = active %v locked %v, want true true", session.Active, session.Locked)
	}
	if session.Class != "user" || session.Desktop != "gnome" {
		t.Errorf("parseSession() class = %q desktop = %q", session.Class, session.Desktop)
	}
	if session.Seat != "seat0" || session.Remote {
		t.Errorf("parseSession() seat = %q remote = %v", session.Seat, session.Remote)
	}
}

func TestParseSessionRequiresName(t *testing.T) {
	if _, err := parseSession([]byte("Id=1\nActive=yes\n")); err == nil {
		t.Fatal("parseSession() error = nil, want error")
	}
}

func TestLoginctlSource(t *testing.T) {
	dir := t.TempDir()
	terminated := filepath.Join(dir, "terminated")
	t.Setenv("TERMINATE_FILE", terminated)

	command := filepath.Join(dir, "loginctl")
	script := `#!/bin/sh
case "$1" in
list-sessions)
	echo "1 1000 dani seat0 tty2"
	;;
show-session)
	echo "Id=1"
	echo "Name=dani"
	echo "User=1000"
	echo "Seat=seat0"
	echo "Remote=no"
	echo "Active=yes"
	echo "LockedHint=no"
	echo "Class=user"
	echo "Desktop=gnome"
	;;
terminate-session)
	echo "$2" > "$TERMINATE_FILE"
	;;
*) echo "unexpected command: $*" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake loginctl: %v", err)
	}

	source := &loginctlSource{command: command}
	sessions, err := source.Sessions(context.Background())
	if err != nil {
		t.Fatalf("Sessions() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].User != "dani" || !sessions[0].Active || sessions[0].Locked {
		t.Fatalf("Sessions() = %+v", sessions)
	}

	if err := source.TerminateSession(context.Background(), "1"); err != nil {
		t.Fatalf("TerminateSession() error = %v", err)
	}
	data, err := os.ReadFile(terminated)
	if err != nil {
		t.Fatalf("read terminate marker: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "1" {
		t.Errorf("terminated session = %q, want 1", got)
	}
}
