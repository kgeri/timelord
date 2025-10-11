# TimeLord kid control

This app was borne out of frustration with Windows Family Safety. The darned thing wasn't recording
usage stats nor applying limits for one of my kids so here we are.

## Developer Notes

### Debugger not showing external code

This is [a f--ing feature apparently...](https://learn.microsoft.com/en-us/visualstudio/debugger/just-my-code?view=vs-2022).
Prevented me from finding out the below issue, so disabled `Just My Code` at `Tools / Options / Debugging`.

### appsettings.json is set up for file notifications

I was building this from under Linux, in a Virtualbox, the project code mapped in a shared folder, and debugging for hours
trying to figure out why app startup hangs. Well apparently there's a [reloadOnChange: true](https://learn.microsoft.com/en-us/aspnet/core/fundamentals/configuration/?view=aspnetcore-9.0#appsettingsjson)
setting for `appsettings.json` that causes an infinite loop on filesystems where change detection doesn't work.

