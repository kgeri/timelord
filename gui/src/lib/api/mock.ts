import { ServiceError } from './errors';
import type {
	DayUsage,
	EnforcerList,
	EnforcerStatus,
	Override,
	OverrideRequest,
	Schedule,
	ScheduleSnapshot,
	TimetowerService,
	UserStatus,
	UserSummary,
	Weekday
} from './types';

interface MockUser {
	username: string;
	displayName: string;
	hostname: string;
	usedSeconds: number;
	limitMinutes: number;
	windowStart: string;
	windowEnd: string;
	/** Oldest first, in minutes, for the weekly chart. */
	historyMinutes: number[];
	online: boolean;
	lastSeenSecondsAgo: number;
}

const WEEKDAYS: Weekday[] = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
const CLOCK_PATTERN = /^([01]\d|2[0-3]):[0-5]\d$/;

const INITIAL_USERS: MockUser[] = [
	{
		username: 'dani',
		displayName: 'Dani',
		hostname: 'dani-desktop',
		usedSeconds: 4500,
		limitMinutes: 120,
		windowStart: '08:00',
		windowEnd: '19:00',
		historyMinutes: [65, 110, 45, 120, 90, 140, 75],
		online: true,
		lastSeenSecondsAgo: 6
	},
	{
		username: 'alex',
		displayName: 'Alex',
		hostname: 'alex-laptop',
		usedSeconds: 5400,
		limitMinutes: 90,
		windowStart: '09:00',
		windowEnd: '18:00',
		historyMinutes: [90, 85, 90, 90, 120, 180, 160],
		online: true,
		lastSeenSecondsAgo: 14
	},
	{
		username: 'sam',
		displayName: 'Sam',
		hostname: 'sam-rig',
		usedSeconds: 1200,
		limitMinutes: 180,
		windowStart: '16:00',
		windowEnd: '21:00',
		historyMinutes: [30, 45, 60, 20, 110, 150, 180],
		online: true,
		lastSeenSecondsAgo: 3
	},
	{
		username: 'taylor',
		displayName: 'Taylor',
		hostname: 'taylor-tablet',
		usedSeconds: 1800,
		limitMinutes: 120,
		windowStart: '08:00',
		windowEnd: '12:00',
		historyMinutes: [0, 25, 40, 15, 30, 20, 10],
		online: false,
		lastSeenSecondsAgo: 2 * 60 * 60 + 17 * 60
	}
];

function pad(value: number): string {
	return String(value).padStart(2, '0');
}

function localDate(date: Date): string {
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

function dateOffset(days: number): string {
	const now = new Date();
	return localDate(new Date(now.getFullYear(), now.getMonth(), now.getDate() + days));
}

function weekdayOfDate(date: string): Weekday {
	const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
	if (!match) return 'mon';
	const value = new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])));
	return WEEKDAYS[value.getUTCDay()] ?? 'mon';
}

/**
 * The mock clock is pinned to 17:00 on the current day so the demo always shows
 * a useful mix of allowed, locked, and outside-window users regardless of when
 * the UI is opened. Replace this with `new Date()` if live-feeling mock data is
 * preferred.
 */
function mockNow(): Date {
	const now = new Date();
	return new Date(now.getFullYear(), now.getMonth(), now.getDate(), 17, 0, 0, 0);
}

function clockMinutes(clock: string): number {
	const [hours, minutes] = clock.split(':').map(Number);
	return hours * 60 + minutes;
}

function isInsideWindow(user: MockUser, now: Date): boolean {
	const clock = now.getHours() * 60 + now.getMinutes();
	return clockMinutes(user.windowStart) <= clock && clock < clockMinutes(user.windowEnd);
}

interface UserDecision {
	remainingSeconds: number | null;
	insideWindow: boolean;
	allowed: boolean;
	denyReason: UserStatus['deny_reason'];
}

function decide(user: MockUser, now: Date): UserDecision {
	const limitSeconds = user.limitMinutes * 60;
	const insideWindow = isInsideWindow(user, now);
	const limitReached = limitSeconds > 0 && user.usedSeconds >= limitSeconds;
	const remainingSeconds = limitSeconds === 0 ? null : Math.max(0, limitSeconds - user.usedSeconds);

	let denyReason: UserStatus['deny_reason'] = null;
	if (!insideWindow) {
		denyReason = 'outside_window';
	} else if (limitReached) {
		denyReason = 'daily_limit_reached';
	}

	return {
		remainingSeconds,
		insideWindow,
		allowed: denyReason === null,
		denyReason
	};
}

function clone<T>(value: T): T {
	return structuredClone(value);
}

export interface MockTimetowerServiceOptions {
	/** Artificial latency in milliseconds, to exercise loading states. */
	latencyMs?: number;
	initialRevision?: number;
}

/**
 * In-memory Timetower implementation used until the admin UI is connected to a
 * real backend. It follows the same wire shapes and policy rules as the API so
 * that switching to `HttpTimetowerService` is a configuration change.
 */
export class MockTimetowerService implements TimetowerService {
	readonly kind = 'mock' as const;

	private readonly latencyMs: number;
	private readonly users: MockUser[];
	private revision: number;

	constructor(options: MockTimetowerServiceOptions = {}) {
		this.latencyMs = options.latencyMs ?? 140;
		this.revision = options.initialRevision ?? 142;
		this.users = clone(INITIAL_USERS);
	}

	private async respond<T>(value: T): Promise<T> {
		if (this.latencyMs > 0) {
			await new Promise((resolve) => setTimeout(resolve, this.latencyMs));
		}
		return clone(value);
	}

	private user(username: string): MockUser {
		const user = this.users.find((candidate) => candidate.username === username);
		if (!user) {
			throw new ServiceError(`Unknown user "${username}".`, 404, 'not_found');
		}
		return user;
	}

	async listUsers(): Promise<UserSummary[]> {
		const now = mockNow();
		const summaries = this.users.map((user) => {
			const decision = decide(user, now);
			return {
				username: user.username,
				display_name: user.displayName,
				hostname: user.hostname,
				used_seconds: user.usedSeconds,
				daily_limit_seconds: user.limitMinutes * 60,
				remaining_seconds: decision.remainingSeconds,
				allowed: decision.allowed
			} satisfies UserSummary;
		});
		return this.respond(summaries);
	}

	async getSchedule(username: string): Promise<ScheduleSnapshot> {
		const user = this.user(username);
		const now = mockNow();
		const day = localDate(now);
		const decision = decide(user, now);
		const limitSeconds = user.limitMinutes * 60;

		const schedule: Schedule = {
			schema_version: 1,
			server_time: now.toISOString(),
			timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
			status_interval_seconds: 60,
			offline_grace_seconds: 3600,
			user: user.username,
			policy: {
				enabled: true,
				daily_limit_seconds: limitSeconds,
				windows: [
					{
						day: weekdayOfDate(day),
						start: user.windowStart,
						end: user.windowEnd
					}
				]
			},
			usage: {
				day,
				used_seconds: user.usedSeconds,
				remaining_seconds: decision.remainingSeconds
			}
		};

		return this.respond({ schedule, revision: this.revision });
	}

	async getHistory(username: string, days = 7): Promise<DayUsage[]> {
		const user = this.user(username);
		const count = Math.min(31, Math.max(1, Math.trunc(days)));
		const history: DayUsage[] = [];

		for (let index = 0; index < count; index += 1) {
			const source = user.historyMinutes.length - count + index;
			const minutes = source >= 0 ? (user.historyMinutes[source] ?? 0) : 0;
			history.push({
				date: dateOffset(index - count + 1),
				used_seconds: minutes * 60
			});
		}

		return this.respond(history);
	}

	async getEnforcers(): Promise<EnforcerList> {
		const now = mockNow();
		const enforcers: EnforcerStatus[] = this.users.map((user) => {
			const decision = decide(user, now);
			const lastSeen = new Date(now.getTime() - user.lastSeenSecondsAgo * 1000);
			const status: UserStatus = {
				user: user.username,
				used_seconds: user.usedSeconds,
				daily_limit_seconds: user.limitMinutes * 60,
				remaining_seconds: decision.remainingSeconds,
				inside_window: decision.insideWindow,
				allowed: decision.allowed,
				deny_reason: decision.denyReason
			};

			return {
				hostname: user.hostname,
				online: user.online,
				last_seen: lastSeen.toISOString(),
				users: [status]
			};
		});

		return this.respond({
			server_time: now.toISOString(),
			server_day: localDate(now),
			enforcers
		});
	}

	async putOverride(username: string, request: OverrideRequest): Promise<Override> {
		const user = this.user(username);

		if (request.start === undefined && request.end === undefined && request.limit_minutes === undefined) {
			throw new ServiceError(
				'At least one of start, end, or limit_minutes is required.',
				422,
				'validation'
			);
		}

		if (request.limit_minutes !== undefined) {
			if (!Number.isInteger(request.limit_minutes) || request.limit_minutes < 0) {
				throw new ServiceError('limit_minutes must be a non-negative integer.', 422, 'validation');
			}
			user.limitMinutes = request.limit_minutes;
		}

		if (request.start !== undefined && !CLOCK_PATTERN.test(request.start)) {
			throw new ServiceError('start must use HH:MM.', 422, 'validation');
		}
		if (request.end !== undefined && !CLOCK_PATTERN.test(request.end)) {
			throw new ServiceError('end must use HH:MM.', 422, 'validation');
		}

		const start = request.start ?? user.windowStart;
		const end = request.end ?? user.windowEnd;
		if (clockMinutes(start) >= clockMinutes(end)) {
			throw new ServiceError('start must be earlier than end.', 422, 'validation');
		}

		user.windowStart = start;
		user.windowEnd = end;
		this.revision += 1;

		const day = localDate(mockNow());
		return this.respond({
			user: user.username,
			day,
			windows: [{ day: weekdayOfDate(day), start: user.windowStart, end: user.windowEnd }],
			limit_minutes: user.limitMinutes
		});
	}
}
