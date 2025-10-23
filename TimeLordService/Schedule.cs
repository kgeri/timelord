namespace TimeLordService;

public class Schedule
{
    public Dictionary<DayOfWeek, DailySchedule> Days { get; set; } = [];

    public override string ToString()
    {
        return '[' + string.Join(", ", Days
            .Select(kvp => $"{kvp.Key}: {kvp.Value}")) + ']';
    }
}

public class DailySchedule
{
    public required TimeOnly Start { get; set; }
    public required TimeOnly End { get; set; }
    public int MaxMinutes { get; set; }

    public bool IsAllowed(DateTime now)
    {
        TimeOnly time = TimeOnly.FromDateTime(now);
        return Start < time && time < End;
    }

    public override string ToString() => $"{Start:hh\\:mm}-{End:hh\\:mm} ({MaxMinutes}m)";
}
