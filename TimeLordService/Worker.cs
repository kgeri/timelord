namespace TimeLordService;

public class Worker(ILogger<Worker> logger, TimeManager timeManager) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation("Started at {time}", DateTimeOffset.Now);

        try
        {
            while (!stoppingToken.IsCancellationRequested)
            {
                timeManager.Process(DateTime.Now);
                await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
            }
        }
        catch (TaskCanceledException)
        {
            logger.LogInformation("Exiting normally");
            Environment.Exit(0);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Worker failed");
            Environment.Exit(1);
        }
    }
}
