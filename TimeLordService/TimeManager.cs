
namespace TimeLordService;

public class TimeManager(ILogger<TimeManager> logger, ScheduleService scheduleService)
{
    private readonly string currentUserName = WindowsSessionUtils.GetCurrentUserName();
    private volatile bool active = true;
    private DateTime lastUpdated = DateTime.Now;
    private DateTime today = DateTime.Now.Date;
    private TimeSpan used = TimeSpan.Zero;

    internal void Process(DateTime now)
    {
        if (!active) return;

        scheduleService.CurrentSchedule.Days.TryGetValue(now.DayOfWeek, out DailySchedule? schedule);

        if (schedule == null)
        {
            logger.LogWarning("No schedule for today, logging out: user={}, now={}", currentUserName, now);
            WindowsSessionUtils.LockWorkStation();
            return;
        }

        if (!schedule.IsAllowed(now))
        {
            logger.LogWarning("Not allowed at this time, logging out: user={}, now={}, schedule={}", currentUserName, now, schedule);
            WindowsSessionUtils.LockWorkStation();
            return;
        }

        if (now.Date > today)
        {
            // Date flip
            today = now.Date;
            used = TimeSpan.Zero;
            lastUpdated = now;
        }
        else
        {
            // Still today - increment activeToday
            used += now - lastUpdated;
            lastUpdated = now;
        }

        if (used.Minutes > schedule.MaxMinutes)
        {
            logger.LogWarning("Time to log out: user={}, usedMinutes={}, schedule={}", currentUserName, used.Minutes, schedule);
            WindowsSessionUtils.LockWorkStation();
            return;
        }

        logger.LogInformation("user={}, today={}, usedMinutes={}", currentUserName, today, used.Minutes);
    }

    internal void OnSessionStart()
    {
        active = true;
        logger.LogInformation("user={}, active={}", currentUserName, active);
    }

    internal void OnSessionEnd()
    {
        active = false;
        logger.LogInformation("user={}, active={}", currentUserName, active);
    }
}
