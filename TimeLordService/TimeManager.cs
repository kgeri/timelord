
namespace TimeLordService;

public class TimeManager(ScheduleService scheduleService)
{
    internal void Process(DateTime now)
    {
        Console.WriteLine($"user={WindowsSessionUtils.GetCurrentUserName()}, schedule={scheduleService.Schedule}");
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
