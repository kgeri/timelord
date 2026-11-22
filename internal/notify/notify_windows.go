//go:build windows

package notify

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"timelord/internal/session"
)

type windowsNotifier struct{}

func newNotifier() Notifier {
	return windowsNotifier{}
}

var (
	moduser32                  = windows.NewLazySystemDLL("user32.dll")
	modshell32                 = windows.NewLazySystemDLL("shell32.dll")
	modkernel32                = windows.NewLazySystemDLL("kernel32.dll")
	modadvapi32                = windows.NewLazySystemDLL("advapi32.dll")
	procRegisterClassExW       = moduser32.NewProc("RegisterClassExW")
	procCreateWindowExW        = moduser32.NewProc("CreateWindowExW")
	procDefWindowProcW         = moduser32.NewProc("DefWindowProcW")
	procDestroyWindow          = moduser32.NewProc("DestroyWindow")
	procLoadIconW              = moduser32.NewProc("LoadIconW")
	procPeekMessageW           = moduser32.NewProc("PeekMessageW")
	procTranslateMessage       = moduser32.NewProc("TranslateMessage")
	procDispatchMessageW       = moduser32.NewProc("DispatchMessageW")
	procGetModuleHandleW       = modkernel32.NewProc("GetModuleHandleW")
	procShellNotifyIconW       = modshell32.NewProc("Shell_NotifyIconW")
	procCreateProcessWithToken = modadvapi32.NewProc("CreateProcessWithTokenW")
)

// Notify launches a short-lived helper in the session of the user that owns
// sess. The helper shows a tray balloon, which Windows 11 renders as a toast.
func (windowsNotifier) Notify(ctx context.Context, sess session.Session, notice Notice) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sessionID, err := strconv.ParseUint(sess.ID, 10, 32)
	if err != nil {
		return fmt.Errorf("session %s: %w", sess.ID, err)
	}
	if err := enablePrivileges(); err != nil {
		return err
	}
	var userToken windows.Token
	if err := windows.WTSQueryUserToken(uint32(sessionID), &userToken); err != nil {
		return fmt.Errorf("query user token for session %s: %w", sess.ID, err)
	}
	defer userToken.Close()

	var primary windows.Token
	if err := windows.DuplicateTokenEx(userToken, windows.TOKEN_ALL_ACCESS, nil,
		windows.SecurityImpersonation, windows.TokenPrimary, &primary); err != nil {
		return fmt.Errorf("duplicate token for session %s: %w", sess.ID, err)
	}
	defer primary.Close()

	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, primary, false); err != nil {
		return fmt.Errorf("create environment block for session %s: %w", sess.ID, err)
	}
	defer windows.DestroyEnvironmentBlock(env)

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current executable: %w", err)
	}
	commandLine := fmt.Sprintf(`"%s" -notify -notify-kind %s -notify-minutes %d`,
		exe, notice.Kind, notice.Minutes)
	return startInSession(primary, exe, commandLine, env)
}

// startInSession creates the helper on the user's interactive desktop without
// a console window.
func startInSession(token windows.Token, exe, commandLine string, env *uint16) error {
	appName, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	command, err := windows.UTF16PtrFromString(commandLine)
	if err != nil {
		return err
	}
	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return err
	}

	var startup windows.StartupInfo
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Desktop = desktop
	var process windows.ProcessInformation

	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW)
	err = windows.CreateProcessAsUser(token, appName, command, nil, nil, false,
		flags, env, nil, &startup, &process)
	if err != nil {
		// CreateProcessAsUser needs service-only privileges. CreateProcessWithTokenW
		// is the fallback for an elevated console, such as -notify-test.
		r1, _, callErr := procCreateProcessWithToken.Call(
			uintptr(token), 0,
			uintptr(unsafe.Pointer(appName)),
			uintptr(unsafe.Pointer(command)),
			uintptr(flags),
			uintptr(unsafe.Pointer(env)),
			0,
			uintptr(unsafe.Pointer(&startup)),
			uintptr(unsafe.Pointer(&process)),
		)
		if r1 == 0 {
			return fmt.Errorf("start notify helper: %v; fallback: %v", err, callErr)
		}
	}
	if process.Thread != 0 {
		windows.CloseHandle(process.Thread)
	}
	if process.Process != 0 {
		windows.CloseHandle(process.Process)
	}
	return nil
}

// enablePrivileges enables the LocalSystem privileges needed to create a
// process in another user's session. Missing privileges are not fatal; the
// launch reports the real error.
func enablePrivileges() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("open process token: %w", err)
	}
	defer token.Close()

	for _, name := range []string{
		"SeTcbPrivilege",
		"SeAssignPrimaryTokenPrivilege",
		"SeIncreaseQuotaPrivilege",
		"SeImpersonatePrivilege",
	} {
		var luid windows.LUID
		if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(name), &luid); err != nil {
			return fmt.Errorf("lookup %s: %w", name, err)
		}
		state := windows.Tokenprivileges{
			PrivilegeCount: 1,
			Privileges: [1]windows.LUIDAndAttributes{{
				Luid:       luid,
				Attributes: windows.SE_PRIVILEGE_ENABLED,
			}},
		}
		if err := windows.AdjustTokenPrivileges(token, false, &state, 0, nil, nil); err != nil {
			continue
		}
	}
	return nil
}

// runChild shows one balloon notification in the current session. The process
// stays alive briefly so the shell can display and own the balloon.
func runChild(ctx context.Context, notice Notice) error {
	return showBalloon(ctx, notice.Title(), notice.Body())
}

const (
	csVRedraw            = 0x0001
	pmRemove             = 0x0001
	nimAdd               = 0x00000000
	nimModify            = 0x00000001
	nimDelete            = 0x00000002
	nifIcon              = 0x00000002
	nifTip               = 0x00000004
	nifInfo              = 0x00000010
	niifInfo             = 0x00000001
	niifNoSound          = 0x00000010
	niifRespectQuietTime = 0x00000080
	idiInformation       = 32516
	idiApplication       = 32512
	errorClassExists     = 1410
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	SmallIcon  windows.Handle
}

type notifyIconData struct {
	Size             uint32
	Window           windows.Handle
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             windows.Handle
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUID             windows.GUID
	BalloonIcon      windows.Handle
}

type winPoint struct {
	X int32
	Y int32
}

type winMsg struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      winPoint
}

var notifyWndProc = windows.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	result, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return result
})

func showBalloon(ctx context.Context, title, body string) error {
	instance, _, callErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return winError("get module handle", callErr)
	}
	className, err := windows.UTF16PtrFromString("TimeLordNotifyWindow")
	if err != nil {
		return err
	}

	class := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		Style:     csVRedraw,
		WndProc:   notifyWndProc,
		Instance:  windows.Handle(instance),
		ClassName: className,
	}
	if result, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		if errno, ok := callErr.(syscall.Errno); !ok || errno != errorClassExists {
			return winError("register notify window class", callErr)
		}
	}

	hwnd, _, callErr := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), 0, 0,
		0, 0, 0, 0, 0, 0, instance, 0,
	)
	if hwnd == 0 {
		return winError("create notify window", callErr)
	}
	defer procDestroyWindow.Call(hwnd)

	icon, _, _ := procLoadIconW.Call(0, uintptr(idiInformation))
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, uintptr(idiApplication))
	}

	var data notifyIconData
	data.Size = uint32(unsafe.Sizeof(data))
	data.Window = windows.Handle(hwnd)
	data.ID = 1
	data.Flags = nifIcon | nifTip
	data.Icon = windows.Handle(icon)
	copy(data.Tip[:len(data.Tip)-1], windows.StringToUTF16("TimeLord"))

	if result, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data))); result == 0 {
		return winError("add notify icon", callErr)
	}
	defer procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))

	data.Flags = nifInfo
	data.InfoFlags = niifInfo | niifNoSound | niifRespectQuietTime
	data.TimeoutOrVersion = 8000
	copy(data.InfoTitle[:len(data.InfoTitle)-1], windows.StringToUTF16(title))
	copy(data.Info[:len(data.Info)-1], windows.StringToUTF16(body))
	if result, _, callErr := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&data))); result == 0 {
		return winError("show notify icon balloon", callErr)
	}

	// Pump messages while the shell displays the balloon. The process must stay
	// alive; a console-less process can exit as soon as the icon is removed.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil
		}
		var msg winMsg
		found, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)
		if found != 0 {
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			continue
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func winError(operation string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno != 0 {
		return fmt.Errorf("%s: %w", operation, errno)
	}
	return fmt.Errorf("%s failed", operation)
}
