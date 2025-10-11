namespace TimeLordService;

using System.Runtime.InteropServices;

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
                if (LockWorkStation())
                {
                    logger.LogInformation($"Locked session for: {name}");
                }
                else
                {
                    logger.LogWarning("Call to LockWorkStation() was not successful");
                }

                await Task.Delay(TimeSpan.FromMinutes(5), stoppingToken);
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

    [DllImport("user32.dll")]
    static extern bool LockWorkStation();
}
