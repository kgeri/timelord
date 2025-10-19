
namespace TimeLordService;

public class UserSessionService
{
    private readonly string currentUserName;

    public string CurrentUserName => currentUserName;

    UserSessionService()
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
}