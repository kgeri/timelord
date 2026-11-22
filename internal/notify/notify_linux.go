//go:build linux

package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"

	"timelord/internal/session"
)

const notifyTimeout = 5 * time.Second

type linuxNotifier struct{}

func newNotifier() Notifier {
	return linuxNotifier{}
}

// Notify re-executes the binary as the session user and calls the freedesktop
// notification service on that user's session bus. The service itself runs as
// root and has no session bus.
func (linuxNotifier) Notify(ctx context.Context, sess session.Session, notice Notice) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sess.UID == "" {
		return fmt.Errorf("session %s has no uid", sess.ID)
	}
	uid, err := strconv.ParseUint(sess.UID, 10, 32)
	if err != nil {
		return fmt.Errorf("session %s uid %q: %w", sess.ID, sess.UID, err)
	}
	account, err := user.LookupId(sess.UID)
	home := ""
	gid := uint32(uid)
	if err == nil {
		home = account.HomeDir
		if parsed, parseErr := strconv.ParseUint(account.Gid, 10, 32); parseErr == nil {
			gid = uint32(parsed)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current executable: %w", err)
	}
	runDir := "/run/user/" + sess.UID
	cmd := exec.CommandContext(ctx, exe,
		"-notify",
		"-notify-kind", string(notice.Kind),
		"-notify-minutes", strconv.Itoa(notice.Minutes),
	)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + home,
		"XDG_RUNTIME_DIR=" + runDir,
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + runDir + "/bus",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: gid},
	}

	// The child is short-lived and returns any D-Bus error on stderr, so wait
	// for it. This also makes the -notify-test flag reliable.
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			return fmt.Errorf("notify child: %w", err)
		}
		return fmt.Errorf("notify child: %w: %s", err, message)
	}
	return nil
}

// runChild sends one notification on the current session bus.
func runChild(ctx context.Context, notice Notice) error {
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect session bus: %w", err)
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.Notifications", dbus.ObjectPath("/org/freedesktop/Notifications"))
	call := obj.CallWithContext(ctx, "org.freedesktop.Notifications.Notify", 0,
		"TimeLord",     // app name
		uint32(0),      // replaces id
		"",             // app icon
		notice.Title(), // summary
		notice.Body(),  // body
		[]string{},     // actions
		map[string]dbus.Variant{ // hints
			"urgency":        dbus.MakeVariant(byte(0)),
			"suppress-sound": dbus.MakeVariant(true),
			"transient":      dbus.MakeVariant(true),
			"category":       dbus.MakeVariant("x-timelord.reminder"),
		},
		int32(6000), // expire timeout in ms
	)
	if call.Err != nil {
		return fmt.Errorf("show notification: %w", call.Err)
	}
	return nil
}
