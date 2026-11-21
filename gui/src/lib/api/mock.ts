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

interface MockHost {
	hostname: string;
	usedSeconds: number;
	/** Oldest first, in minutes, for the weekly chart. */
	historyMinutes: number[];
	online: boolean;
	lastSeenSecondsAgo: number;
}

interface MockUser {
	username: string;
	displayName: string;
	limitMinutes: number;
	windowStart: string;
	windowEnd: string;
	hosts: MockHost[];
}

const WEEKDAYS: Weekday[] = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
const CLOCK_PATTERN = /^([01]\d|2[0-3]):[0-5]\d$/;

const INITIAL_USERS: MockUser[] = [
	{
		username: 'dani',
		displayName: 'Dani',
		limitMinutes: 120,
		windowStart: '08:00',
		windowEnd: '19:00',
		hosts: [
			{
				hostname: 'dani-desktop',
				usedSeconds: 3000,
				historyMinutes: [40, 70, 30, 80, 55, 90, 50],
				online: true,
				lastSeenSecondsAgo: 6
			},
			{
				hostname: 'dani-laptop',
				usedSeconds: 1500,
				historyMinutes: [25, 40, 15, 40, 35, 50, 25],
				online: true,
				lastSeenSecondsAgo: 18
			}
		]
	},
	{
		username: 'alex',
		displayName: 'Alex',
		limitMinutes: 90,
		windowStart: '09:00',
		windowEnd: '18:00',
		hosts: [
			{
				hostname: 'alex-work',
				usedSeconds: 3600,
				historyMinutes: [60, 55, 60, 60, 80, 120, 105],
				online: true,
				lastSeenSecondsAgo: 14
			},
			{
				hostname: 'alex-laptop',
				usedSeconds: 1800,
				historyMinutes: [30, 30, 30, 30, 40, 60, 55],
				online: false,
				lastSeenSecondsAgo: 2 * 60 * 60 + 4 * 60
			}
		]
	},
	{
		username: 'sam',
		displayName: 'Sam',
		limitMinutes: 180,
		windowStart: '16:00',
		windowEnd: '21:00',
		hosts: [
			{
				hostname: 'sam-rig',
				usedSeconds: 1200,
				historyMinutes: [30, 45, 60, 20, 110, 150, 180],
				online: true,
				lastSeenSecondsAgo: 3
			}
		]
	},
	{
		username: 'taylor',
		displayName: 'Taylor',
		limitMinutes: 120,
		windowStart: '08:00',
		windowEnd: '12:00',
		hosts: [
			{
				hostname: 'taylor-tablet',
				usedSeconds: 1800,
				historyMinutes: [0, 25, 40, 15, 30, 20, 10],
				online: false,
				lastSeenSecondsAgo: 2 * 60 * 60 + 17 * 60
			}
		]
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

function totalUsedSeconds(user: MockUser): number {
	return user.hosts.reduce((sum, host) => sum + host.usedSeconds, 0);
}

function historyMinutesAt(host: MockHost, source: number): number {
	return source >= 0 ? (host.historyMinutes[source] ?? 0) : 0;
}

interface UserDecision {
	remainingSeconds: number | null;
	insideWindow: boolean;
	allowed: boolean;
	denyReason: UserStatus['deny_reason'];
}

function decide(user: MockUser, now: Date): UserDecision {
	const usedSeconds = totalUsedSeconds(user);
	const limitSeconds = user.limitMinutes * 60;
	const insideWindow = isInsideWindow(user, now);
	const limitReached = limitSeconds > 0 && usedSeconds >= limitSeconds;
	const remainingSeconds = limitSeconds === 0 ? null : Math.max(0, limitSeconds - usedSeconds);

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
				hostname: user.hosts[0]?.hostname ?? null,
				used_seconds: totalUsedSeconds(user),
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

		const usedSeconds = totalUsedSeconds(user);
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
				used_seconds: usedSeconds,
				remaining_seconds: decision.remainingSeconds,
				hosts: user.hosts
					.map((host) => ({ hostname: host.hostname, used_seconds: host.usedSeconds }))
					.sort((a, b) => a.hostname.localeCompare(b.hostname))
			}
		};

		return this.respond({ schedule, revision: this.revision });
	}

	async getHistory(username: string, days = 7): Promise<DayUsage[]> {
		const user = this.user(username);
		const count = Math.min(31, Math.max(1, Math.trunc(days)));
		const history: DayUsage[] = [];
		const historyLength = user.hosts[0]?.historyMinutes.length ?? 0;

		for (let index = 0; index < count; index += 1) {
			const source = historyLength - count + index;
			const hosts = user.hosts
				.map((host) => ({
					hostname: host.hostname,
					used_seconds: historyMinutesAt(host, source) * 60
				}))
				.filter((host) => host.used_seconds > 0)
				.sort((a, b) => a.hostname.localeCompare(b.hostname));
			const usedSeconds = hosts.reduce((sum, host) => sum + host.used_seconds, 0);
			history.push({
				date: dateOffset(index - count + 1),
				used_seconds: usedSeconds,
				hosts
			});
		}

		return this.respond(history);
	}

	async getEnforcers(): Promise<EnforcerList> {
		const now = mockNow();
		const enforcers: EnforcerStatus[] = this.users.flatMap((user) => {
			const decision = decide(user, now);
			const usedSeconds = totalUsedSeconds(user);
			const status: UserStatus = {
				user: user.username,
				used_seconds: usedSeconds,
				daily_limit_seconds: user.limitMinutes * 60,
				remaining_seconds: decision.remainingSeconds,
				inside_window: decision.insideWindow,
				allowed: decision.allowed,
				deny_reason: decision.denyReason
			};

			return user.hosts.map((host) => {
				const lastSeen = new Date(now.getTime() - host.lastSeenSecondsAgo * 1000);
				return {
					hostname: host.hostname,
					online: host.online,
					last_seen: lastSeen.toISOString(),
					users: [status]
				};
			});
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
