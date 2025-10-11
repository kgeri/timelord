using TimeLordService;

Environment.SetEnvironmentVariable("DOTNET_hostBuilder:reloadConfigOnChange", "false"); // OMG... see README.md

Console.WriteLine("Starting TimeLord...");
var builder = Host.CreateApplicationBuilder(args);
//builder.Services.AddWindowsService(options => options.ServiceName = "TimeLord");
builder.Services.AddHostedService<Worker>();
builder.Logging.AddConsole();
builder.Logging.AddEventLog(settings => settings.SourceName = "TimeLordService");
IHost host = builder.Build();
host.Run();
