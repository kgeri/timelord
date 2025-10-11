namespace TimeLordService;

using System.Text.Json;

public class ScheduleService : IDisposable
{
    private readonly string SchedulePath = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
        "kidcontrol.json"
    );
    private readonly ILogger<ScheduleService> logger;
    private readonly FileSystemWatcher watcher;
    private Schedule schedule;
    public Schedule Schedule => schedule;

    public ScheduleService(ILogger<ScheduleService> logger)
    {
        this.logger = logger;
        LoadSchedule();

        // Set up file watching for the schedule
        var dir = Path.GetDirectoryName(SchedulePath)!;
        var file = Path.GetFileName(SchedulePath);
        watcher = new FileSystemWatcher(dir, file)
        {
            NotifyFilter = NotifyFilters.LastWrite | NotifyFilters.Size
        };
        watcher.Changed += (_, _) => LoadSchedule();
        watcher.EnableRaisingEvents = true;
    }

    private void LoadSchedule()
    {
        if (File.Exists(SchedulePath))
        {
            using var stream = File.OpenRead(SchedulePath);
            schedule = JsonSerializer.Deserialize<Schedule>(stream);
            logger.LogInformation($"Loaded {SchedulePath}: {schedule}");
        }
        else
        {
            schedule = null;
            logger.LogInformation($"{SchedulePath} not found, using default schedule");
        }
    }

    public void Dispose()
    {
        watcher.Dispose();
    }
}