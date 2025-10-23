namespace TimeLordService;

using System.Text.Json;

public class ScheduleService : IDisposable
{
    private readonly string SchedulePath = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
        ".timelord\\kidcontrol.json"
    );
    private readonly ILogger<ScheduleService> logger;
    private readonly FileSystemWatcher watcher;
    private Schedule schedule;

    public Schedule CurrentSchedule => schedule;

    public ScheduleService(ILogger<ScheduleService> logger)
    {
        this.logger = logger;
        schedule = LoadSchedule(SchedulePath);

        // Set up file watching for the schedule
        var dir = Path.GetDirectoryName(SchedulePath)!;
        var file = Path.GetFileName(SchedulePath);
        watcher = new FileSystemWatcher(dir, file)
        {
            NotifyFilter = NotifyFilters.LastWrite | NotifyFilters.Size
        };
        watcher.Changed += (_, _) => schedule = LoadSchedule(SchedulePath);
        watcher.EnableRaisingEvents = true;
    }

    private Schedule LoadSchedule(string path)
    {
        Schedule s;
        if (File.Exists(path))
        {
            using var stream = File.OpenRead(path);
            s = JsonSerializer.Deserialize<Schedule>(stream) ?? throw new JsonException("Failed to deserialize schedule");
            logger.LogInformation("Loaded {path}: {schedule}", path, s);
        }
        else
        {
            s = new Schedule();
            logger.LogInformation("{path} not found, using default schedule", path);
        }
        return s;
    }

    public void Dispose()
    {
        watcher.Dispose();
    }
}