namespace TimeLordService;

public class Worker(ILogger<Worker> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        Console.WriteLine("Started TimeLordService");
        logger.LogInformation("TimeLordService started at {time}", DateTimeOffset.Now);

        try
        {
            while (!stoppingToken.IsCancellationRequested)
            {
                string name = System.Security.Principal.WindowsIdentity.GetCurrent().Name;
                logger.LogInformation($"Current user: {name}");

                await Task.Delay(TimeSpan.FromSeconds(5), stoppingToken);
            }
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "TimeLordService failed");
            Environment.Exit(1);
        }
    }
}
