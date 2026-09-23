# TimeLord Specification

## Purpose

TimeLord limits the time that a child can use a computer.
It reads a schedule for each day of the week.
It locks the workstation when the time is not permitted.
It also locks the workstation when the child uses more than the daily limit.

## Environment

TimeLord is a .NET 8 worker service.
It runs on Windows.
It uses Windows session events and the Windows workstation lock.
It writes log messages to the console.

## Schedule File

The schedule file is `kidcontrol.json`.
It is in the `.timelord` directory in the user profile directory.
On Windows, the path is `C:\Users\<user>\.timelord\kidcontrol.json`.
The file gives the days of the week.
Each day gives a start time, an end time, and a daily limit in minutes.
A sample file is in `samples/kidcontrol.json`.
The service loads the file at start.
If the file is absent, the service uses an empty schedule.
If the file changes, the service loads it again.

## Behavior

The service checks the time each minute.
The service tracks the user session.
A logon or unlock starts the session.
A lock or logoff ends the session.
The service counts time only during a session.
The service adds the elapsed time to the used time for the day.
The used time resets to zero at the start of a new day.
The service locks the workstation in these cases:

* The current day has no schedule.
* The current time is outside the permitted range.
* The used time is more than the daily limit.
The service locks the workstation only.
It does not close the user session.
If the main loop fails, the service stops with exit code 1.
If the service stops on request, it stops with exit code 0.
