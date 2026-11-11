# Timetower Admin UI Specification

## Overview

A lightweight, mobile-first admin web application for managing Timetower Enforcer schedules, remaining daily times, emergency locks, and quick time extensions.

---

## Screen Architecture

### 1. Header & Global State

* **Server Revision**: Displays active `X-Schedule-Revision`.
* **Sync Status**: Pulse indicator for real-time connection status with manual refresh trigger.
* **User Selector Carousel**: Horizontal scrollable list for instant switching between monitored user accounts (`dani`, `alex`, `sam`, `taylor`).

### 2. Main Tabs

#### Tab A: Status & Quick Controls (Primary Screen)

* **Usage Ring & Stats**: Visual radial indicator of time used vs. daily limit. Displays remaining seconds, current schedule window, and allowance status (`Allowed`, `Outside Window`, `Locked`).
* **Quick Add Toolbar**: 1-tap buttons (+15m, +30m, +45m, +60m) that update the user's limit and recalculate remaining time.
* **Emergency Lock Button**: Instant trigger setting limit to current usage (0 remaining seconds) and flagging the account as locked.

#### Tab B: Override Settings

* **Window Editor**: Inputs to modify start and end boundaries for today's policy day.
* **Daily Limit Input**: Numeric input for setting overall daily allowed duration (in minutes).
* **Commit Trigger**: Issues `PUT /api/v1/override/{user}`.

#### Tab C: Historical Stats

* **7-Day Bar Visualization**: CSS-native bar chart rendering weekly usage trends (minutes used per day).

#### Tab D: Enforcer Nodes

* **Reporting Hosts List**: Cards showing active/offline status, hostnames, and last contact time (`/api/v1/enforcers`).

---

## State Model (Vanilla JS)

* `currentUser`: Active selected user object.
* `revision`: Integer incremented on any mutation or manually refreshed sync.
* `activeTab`: String tab key (`status`, `override`, `stats`, `nodes`).
