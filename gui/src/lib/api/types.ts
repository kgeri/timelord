import type { components } from './schema';

/**
 * Wire types generated from docs/openapi.yaml (via `npm run generate:api`).
 * Keep the aliases in one place so the rest of the UI never depends on the
 * generated file's shape directly.
 */
export type Schedule = components['schemas']['Schedule'];
export type Policy = components['schemas']['Policy'];
export type PolicyWindow = components['schemas']['Window'];
export type Weekday = PolicyWindow['day'];
export type UsageState = components['schemas']['UsageState'];
export type UsageHistory = components['schemas']['UsageHistory'];
export type UsageDay = components['schemas']['UsageDay'];
export type OverrideRequest = components['schemas']['OverrideRequest'];
export type Override = components['schemas']['Override'];
export type EnforcerList = components['schemas']['EnforcerList'];
export type EnforcerStatus = components['schemas']['EnforcerStatus'];
export type UserStatus = components['schemas']['UserStatus'];
export type User = components['schemas']['User'];
export type UserList = components['schemas']['UserList'];
export type DenyReason = components['schemas']['DenyReason'];

/** A schedule plus the revision advertised by `X-Schedule-Revision`. */
export interface ScheduleSnapshot {
	schedule: Schedule;
	revision: number | null;
}

/**
 * One day of historical usage in the shape the Stats tab uses. The API calls
 * the date field `day`; the HTTP service maps it to `date` for the chart.
 */
export interface DayUsage {
	/** Local policy day, `YYYY-MM-DD`. */
	date: string;
	used_seconds: number;
}

/**
 * A user in the selector carousel. Fields that depend on a report are nullable
 * because a configured user may not have reported yet.
 */
export interface UserSummary {
	username: string;
	display_name: string;
	hostname: string | null;
	used_seconds: number | null;
	daily_limit_seconds: number | null;
	remaining_seconds: number | null;
	allowed: boolean | null;
}

export type ServiceKind = 'mock' | 'http';

/**
 * The seam between the UI and Timetower. The mock and HTTP implementations are
 * interchangeable; swapping backends only changes the factory in `api/index.ts`.
 */
export interface TimetowerService {
	readonly kind: ServiceKind;

	/** Users the admin console may inspect. */
	listUsers(): Promise<UserSummary[]>;

	/** Current policy and usage for one user, plus the schedule revision. */
	getSchedule(username: string): Promise<ScheduleSnapshot>;

	/** Last `days` days of usage, oldest first. */
	getHistory(username: string, days?: number): Promise<DayUsage[]>;

	/** Reporting enforcer hosts and their user statuses. */
	getEnforcers(): Promise<EnforcerList>;

	/** Apply a current-day override. */
	putOverride(username: string, request: OverrideRequest): Promise<Override>;
}
