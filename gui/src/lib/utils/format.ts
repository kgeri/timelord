import type { Weekday } from '#lib/api/types';

const WEEKDAYS: Weekday[] = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
const DAY_INITIAL: Record<Weekday, string> = {
	sun: 'S',
	mon: 'M',
	tue: 'T',
	wed: 'W',
	thu: 'T',
	fri: 'F',
	sat: 'S'
};
const DAY_LABEL: Record<Weekday, string> = {
	sun: 'Sun',
	mon: 'Mon',
	tue: 'Tue',
	wed: 'Wed',
	thu: 'Thu',
	fri: 'Fri',
	sat: 'Sat'
};

/** Format seconds as `1h 05m` / `45m`. `null` means no limit. */
export function formatDuration(seconds: number | null | undefined): string {
	if (seconds === null || seconds === undefined) return '∞';
	if (!Number.isFinite(seconds) || seconds <= 0) return '0m';

	const hours = Math.floor(seconds / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	if (hours > 0) {
		return `${hours}h ${String(minutes).padStart(2, '0')}m`;
	}
	return `${minutes}m`;
}

/** Format seconds as decimal hours, for the weekly chart. */
export function formatHours(seconds: number): string {
	if (!Number.isFinite(seconds) || seconds <= 0) return '0h';
	const hours = seconds / 3600;
	if (hours < 0.05) return '<0.1h';
	return `${Number(hours.toFixed(1))}h`;
}

/** Parse a `YYYY-MM-DD` policy day as a UTC date and return its weekday. */
export function weekdayOfDate(date: string): Weekday | null {
	const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
	if (!match) return null;
	const [, year, month, day] = match;
	const value = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)));
	if (Number.isNaN(value.getTime())) return null;
	return WEEKDAYS[value.getUTCDay()] ?? null;
}

/** `2024-06-03` -> `Mon`. */
export function dayLabel(date: string): string {
	const weekday = weekdayOfDate(date);
	return weekday ? DAY_LABEL[weekday] : '—';
}

/** Single-letter chart label (`M`, `T`, ...). */
export function dayInitial(date: string): string {
	const weekday = weekdayOfDate(date);
	return weekday ? DAY_INITIAL[weekday] : '—';
}

/** `2024-06-03` -> `Jun 3`. */
export function shortDate(date: string): string {
	const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
	if (!match) return date;
	const [, year, month, day] = match;
	const value = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)));
	if (Number.isNaN(value.getTime())) return date;
	return value.toLocaleDateString(undefined, {
		month: 'short',
		day: 'numeric',
		timeZone: 'UTC'
	});
}

/** Human-friendly relative time for node last-seen values. */
export function formatRelativeTime(iso: string | null | undefined, now: Date = new Date()): string {
	if (!iso) return 'never';
	const then = new Date(iso);
	if (Number.isNaN(then.getTime())) return 'unknown';

	const seconds = Math.max(0, Math.round((now.getTime() - then.getTime()) / 1000));
	if (seconds < 10) return 'just now';
	if (seconds < 60) return `${seconds}s ago`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	return `${days}d ago`;
}

/** Convert `HH:MM` to minutes since midnight. Invalid values return null. */
export function clockToMinutes(clock: string): number | null {
	const match = /^([01]\d|2[0-3]):([0-5]\d)$/.exec(clock);
	if (!match) return null;
	return Number(match[1]) * 60 + Number(match[2]);
}
