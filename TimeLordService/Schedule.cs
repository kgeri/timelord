namespace TimeLordService;

public class Schedule
{
    public Dictionary<DayOfWeek, DailySchedule> Days { get; set; }

    public override string ToString()
    {
        return string.Join(", ", Days
            .Select(kvp => $"{kvp.Key}: {kvp.Value}"));
    }
}

public class DailySchedule
{
    public string Start { get; set; }
    public string End { get; set; }
    public int MaxMinutes { get; set; }

    public override string ToString() => $"{Start:hh\\:mm}-{End:hh\\:mm} ({MaxMinutes}m)";
}
