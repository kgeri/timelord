# Timetower Admin UI Specification

## Overview

A lightweight, mobile-first admin web application for managing Timetower Enforcer schedules, remaining daily times, emergency locks, and quick time extensions. The page fits the viewport without vertical page scrolling.

---

## Screen Architecture

### 1. User Selector

* **User Carousel**: Horizontal scrollable list for instant switching between monitored user accounts (`dani`, `alex`, `sam`, `taylor`).
* **Enforcer Status**: A small online/offline indicator on each user pill, based on the reporting enforcer for that user.

### 2. Main Tabs

#### Tab A: Status & Quick Controls (Primary Screen)

* **Usage Bar & Stats**: Compact horizontal progress bar showing used and remaining time inside the bar. Color communicates the allowance state: green for allowed, amber for outside the window, red for locked/limit reached, and grey for disabled/no policy.
* **Window & Limit**: The current schedule window and daily limit appear in the status card header.
* **Quick Add Toolbar**: 1-tap buttons (+15m, +30m, +45m, +60m) that update the user's limit and recalculate remaining time.
* **Emergency Lock Button**: Instant trigger setting limit to current usage (0 remaining seconds) and flagging the account as locked.
* **Schedule & Limit Override**: Inline form below the quick actions for the current policy day's `From`, `Until`, and limit in minutes. Commits with `PUT /api/v1/override/{user}`.

#### Tab B: Historical Stats

* **7-Day Bar Visualization**: CSS-native bar chart rendering weekly usage trends (minutes used per day).

---

## State Model (Svelte 5 Runes)

* `currentUser`: Active selected user account.
* `revision`: Opaque schedule revision from `X-Schedule-Revision`; tracked for refresh semantics but not displayed.
* `tab`: Active tab key (`status`, `stats`).
