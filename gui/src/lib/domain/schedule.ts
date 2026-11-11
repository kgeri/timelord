import type { DenyReason, PolicyWindow, Schedule, Weekday } from '#lib/api/types';
import { clockToMinutes, weekdayOfDate } from '#lib/utils/format';

export interface DayWindow {
	start: string;
	end: string;
}

export type StatusKind = 'allowed' | 'locked' | 'window' | 'disabled' | 'unknown';

export interface ScheduleStatus {
	hasPolicy: boolean;
	usedSeconds: number;
	limitSeconds: number;
	remainingSeconds: number | null;
	unlimited: boolean;
	percentUsed: number;
	insideWindow: boolean;
	allowed: boolean;
	limitReached: boolean;
	denyReason: DenyReason;
	kind: StatusKind;
	label: string;
	badgeClass: StatusKind;
	window: DayWindow | null;
	windowLabel: string;
}

const BADGE_LABEL: Record<StatusKind, string> = {
	allowed: '✓ Access Allowed',
	locked: '🔒 Locked / Limit Reached',
	window: '◷ Outside Window',
	disabled: '⏸ Policy Disabled',
	unknown: '… Waiting for data'
};

const WEEKDAYS: Weekday[] = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];

interface ZonedParts {
	weekday: Weekday;
	minutes: number;
}

function asWeekday(value: string): Weekday | null {
	const key = value.toLowerCase().slice(0, 3);
	return (WEEKDAYS as string[]).includes(key) ? (key as Weekday) : null;
}

function zonedParts(iso: string, timeZone: string): ZonedParts | null {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return null;

	try {
		const parts = new Intl.DateTimeFormat('en-US', {
			timeZone,
			weekday: 'short',
			hour: '2-digit',
			minute: '2-digit',
			hourCycle: 'h23'
		}).formatToParts(date);

		let weekday: Weekday | null = null;
		let hour = 0;
		let minute = 0;

		for (const part of parts) {
			if (part.type === 'weekday') weekday = asWeekday(part.value);
			if (part.type === 'hour') hour = Number(part.value) % 24;
			if (part.type === 'minute') minute = Number(part.value);
		}

		if (weekday) {
			return { weekday, minutes: hour * 60 + minute };
		}
	} catch {
		// An invalid timezone should not blank the whole status card. Fall back
		// to UTC, which is how the server stores timestamps.
	}

	return {
		weekday: WEEKDAYS[date.getUTCDay()] ?? 'mon',
		minutes: date.getUTCHours() * 60 + date.getUTCMinutes()
	};
}

function insideWindows(windows: PolicyWindow[], weekday: Weekday, minutes: number): boolean {
	return windows.some((window) => {
		if (window.day !== weekday) return false;
		const start = clockToMinutes(window.start);
		const end = clockToMinutes(window.end);
		return start !== null && end !== null && start <= minutes && minutes < end;
	});
}

/**
 * The effective window for the schedule's policy day, as the earliest start and
 * latest end of that day's windows (matching Timetower's override semantics).
 */
export function windowForDay(schedule: Schedule | null, date?: string): DayWindow | null {
	if (!schedule?.policy) return null;

	const policyDay = date ?? schedule.usage.day;
	const weekday = weekdayOfDate(policyDay);
	const windows = weekday
		? schedule.policy.windows.filter((window) => window.day === weekday)
		: schedule.policy.windows;

	if (windows.length === 0) return null;

	let start = windows[0].start;
	let end = windows[0].end;
	for (const window of windows) {
		if ((clockToMinutes(window.start) ?? 0) < (clockToMinutes(start) ?? 0)) start = window.start;
		if ((clockToMinutes(window.end) ?? 0) > (clockToMinutes(end) ?? 0)) end = window.end;
	}
	return { start, end };
}

/**
 * Derive the decision that Timetower would report for this schedule.
 *
 * The schedule endpoint does not include `allowed`/`inside_window`, while the
 * status and enforcer responses do. Mirroring policy.Allowed here keeps the
 * status tab correct for either source and makes the mock and HTTP services
 * behave the same way.
 */
export function deriveScheduleStatus(schedule: Schedule | null): ScheduleStatus {
	if (!schedule) {
		return {
			hasPolicy: false,
			usedSeconds: 0,
			limitSeconds: 0,
			remainingSeconds: null,
			unlimited: false,
			percentUsed: 0,
			insideWindow: false,
			allowed: false,
			limitReached: false,
			denyReason: null,
			kind: 'unknown',
			label: BADGE_LABEL.unknown,
			badgeClass: 'unknown',
			window: null,
			windowLabel: '—'
		};
	}

	const policy = schedule.policy;
	const usedSeconds = Math.max(0, schedule.usage.used_seconds);
	const limitSeconds = Math.max(0, policy?.daily_limit_seconds ?? 0);
	const unlimited = policy !== null && limitSeconds === 0;

	let remainingSeconds: number | null = null;
	if (policy) {
		if (limitSeconds === 0) {
			remainingSeconds = null;
		} else if (schedule.usage.remaining_seconds !== null) {
			remainingSeconds = Math.max(0, schedule.usage.remaining_seconds);
		} else {
			remainingSeconds = Math.max(0, limitSeconds - usedSeconds);
		}
	}

	const limitReached = policy !== null && limitSeconds > 0 && usedSeconds >= limitSeconds;
	const parts = zonedParts(schedule.server_time, schedule.timezone);
	const insideWindow = policy
		? policy.enabled && parts !== null && insideWindows(policy.windows, parts.weekday, parts.minutes)
		: false;

	let denyReason: DenyReason = null;
	if (!policy) {
		denyReason = 'no_policy';
	} else if (!policy.enabled) {
		denyReason = 'disabled';
	} else if (!insideWindow) {
		denyReason = 'outside_window';
	} else if (limitReached) {
		denyReason = 'daily_limit_reached';
	}

	let kind: StatusKind;
	if (!policy || !policy.enabled) {
		kind = 'disabled';
	} else if (limitReached) {
		kind = 'locked';
	} else if (!insideWindow) {
		kind = 'window';
	} else {
		kind = 'allowed';
	}

	const percentUsed =
		limitSeconds > 0 ? Math.min(100, Math.max(0, Math.round((usedSeconds / limitSeconds) * 100))) : 0;
	const window = windowForDay(schedule);

	return {
		hasPolicy: policy !== null,
		usedSeconds,
		limitSeconds,
		remainingSeconds,
		unlimited,
		percentUsed,
		insideWindow,
		allowed: denyReason === null,
		limitReached,
		denyReason,
		kind,
		label: BADGE_LABEL[kind],
		badgeClass: kind,
		window,
		windowLabel: window ? `${window.start} - ${window.end}` : 'No window'
	};
}
