namespace TimeLordService;

using System.Runtime.InteropServices;
using Microsoft.Win32;

class WindowsSessionUtils
{
    internal static string GetCurrentUserName()
    {
        try
        {
            return System.Security.Principal.WindowsIdentity.GetCurrent().Name;
        }
        catch (PlatformNotSupportedException)
        {
            return "<unknown>";
        }
    }

    [DllImport("user32.dll")]
    internal static extern bool LockWorkStation();
}

/// <summary>
/// Listens for Windows session events (logon, logoff, lock, unlock) and notifies TimeManager.
/// This service is not enabled on Linux.
/// </summary>
public class WindowsSessionListener(ILogger<WindowsSessionListener> logger, TimeManager timeManager) : IHostedService
{
    public Task StartAsync(CancellationToken cancellationToken)
    {
        SystemEvents.SessionSwitch += OnSessionSwitch;
        return Task.CompletedTask;
    }

    public Task StopAsync(CancellationToken cancellationToken)
    {
        SystemEvents.SessionSwitch -= OnSessionSwitch;
        return Task.CompletedTask;
    }

    private void OnSessionSwitch(object? sender, SessionSwitchEventArgs e)
    {
        try
        {
            switch (e.Reason)
            {
                case SessionSwitchReason.SessionLogon:
                case SessionSwitchReason.SessionUnlock:
                    timeManager.OnSessionStart();
                    break;
                case SessionSwitchReason.SessionLogoff:
                case SessionSwitchReason.SessionLock:
                    timeManager.OnSessionEnd();
                    break;
            }
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Error handling session switch");
        }
    }
}
