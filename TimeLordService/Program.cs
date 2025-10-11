using System.Text.Json;
using TimeLordService;

Environment.SetEnvironmentVariable("DOTNET_hostBuilder:reloadConfigOnChange", "false"); // OMG... see README.md

Console.WriteLine("Starting TimeLord...");

var builder = Host.CreateApplicationBuilder(args);
builder.Services.AddHostedService<Worker>();
builder.Services.AddSingleton<ScheduleService>();
builder.Logging.AddConsole();
builder.Logging.AddEventLog(settings => settings.SourceName = "TimeLordService");
builder.Build().Run();
