namespace TimeLordService;

public class Worker(ILogger<Worker> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation("Started at {time}", DateTimeOffset.Now);

        try
        {
            while (!stoppingToken.IsCancellationRequested)
            {
                string name = System.Security.Principal.WindowsIdentity.GetCurrent().Name;
                logger.LogInformation($"Current user: {name}");

                await Task.Delay(TimeSpan.FromSeconds(5), stoppingToken);
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
