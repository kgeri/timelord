
using Microsoft.Win32;

namespace TimeLordService;

public class UserSessionService
{
    private readonly string currentUserName;

    public string CurrentUserName => currentUserName;

    public UserSessionService()
    {
        try
        {
            currentUserName = System.Security.Principal.WindowsIdentity.GetCurrent().Name;
        }
        catch (PlatformNotSupportedException)
        {
            currentUserName = "<unknown>";
        }
    }

    internal void OnSessionStart()
    {
        throw new NotImplementedException();
    }

    internal void OnSessionEnd()
    {
        throw new NotImplementedException();
    }
}

public class WindowsSessionSwitchService(ILogger<WindowsSessionSwitchService> logger, UserSessionService userSessionService) : IHostedService
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
                    userSessionService.OnSessionStart();
                    break;
                case SessionSwitchReason.SessionLogoff:
                case SessionSwitchReason.SessionLock:
                    userSessionService.OnSessionEnd();
                    break;
            }
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Error handling session switch");
        }
    }
}
