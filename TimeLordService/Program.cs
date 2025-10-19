using System.Runtime.InteropServices;
using TimeLordService;

Environment.SetEnvironmentVariable("DOTNET_hostBuilder:reloadConfigOnChange", "false"); // OMG... see README.md

Console.WriteLine("Starting TimeLord...");

var builder = Host.CreateApplicationBuilder(args);
builder.Services.AddHostedService<Worker>()
    .AddSingleton<ScheduleService>()
    .AddSingleton<UserSessionService>();

if (RuntimeInformation.IsOSPlatform(OSPlatform.Windows))
{
    builder.Services.AddHostedService<WindowsSessionSwitchService>();
}

builder.Logging.AddConsole();
// builder.Logging.AddEventLog(settings => settings.SourceName = "TimeLordService");
builder.Build().Run();
