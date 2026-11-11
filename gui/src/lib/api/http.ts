import { ServiceError } from './errors';
import type {
	DayUsage,
	EnforcerList,
	Override,
	OverrideRequest,
	Schedule,
	ScheduleSnapshot,
	TimetowerService,
	UserStatus,
	UserSummary
} from './types';

export interface HttpTimetowerServiceOptions {
	/**
	 * Origin of the API (for example `https://timetower.ogreg.org`). Leave empty
	 * to call the same-origin proxy under `/api/v1`.
	 */
	baseUrl?: string;
	/** Bearer key for a direct connection. Prefer the server-side proxy. */
	token?: string;
	/**
	 * Users shown before any enforcer has reported. Timetower does not expose a
	 * configured-user listing, so the admin UI needs this fallback.
	 */
	users?: string[];
	fetch?: typeof fetch;
}

interface ApiResult<T> {
	data: T;
	revision: number | null;
}

function parseRevision(value: string | null): number | null {
	if (!value) return null;
	const revision = Number.parseInt(value, 10);
	return Number.isFinite(revision) ? revision : null;
}

/**
 * Typed client for the real Timetower API.
 *
 * When `baseUrl` is empty, requests go through the SvelteKit proxy at
 * `src/routes/api/v1/[...path]/+server.ts`. That proxy keeps the bearer key on
 * the server and avoids CORS, so the browser bundle never contains a secret.
 */
export class HttpTimetowerService implements TimetowerService {
	readonly kind = 'http' as const;

	private readonly baseUrl: string;
	private readonly token: string | undefined;
	private readonly configuredUsers: string[];
	private readonly fetchImpl: typeof fetch;

	constructor(options: HttpTimetowerServiceOptions = {}) {
		this.baseUrl = (options.baseUrl ?? '').replace(/\/+$/, '');
		this.token = options.token?.trim() || undefined;
		this.configuredUsers = [...new Set((options.users ?? []).map((user) => user.trim()).filter(Boolean))];
		this.fetchImpl = options.fetch ?? fetch;
	}

	private url(path: string): string {
		return `${this.baseUrl}/api/v1${path}`;
	}

	private async request<T>(
		method: 'GET' | 'PUT',
		path: string,
		body?: unknown,
		signal?: AbortSignal
	): Promise<ApiResult<T>> {
		const headers = new Headers({ Accept: 'application/json' });
		if (this.token) headers.set('Authorization', `Bearer ${this.token}`);
		if (body !== undefined) headers.set('Content-Type', 'application/json');

		let response: Response;
		try {
			response = await this.fetchImpl(this.url(path), {
				method,
				headers,
				body: body === undefined ? undefined : JSON.stringify(body),
				signal
			});
		} catch (error) {
			throw new ServiceError(
				error instanceof Error ? `Could not reach Timetower: ${error.message}` : 'Could not reach Timetower.',
				0,
				'network'
			);
		}

		if (!response.ok) {
			const message = await this.errorMessage(response);
			throw new ServiceError(message, response.status, `http_${response.status}`);
		}

		const text = await response.text();
		const data = (text ? JSON.parse(text) : undefined) as T;
		return { data, revision: parseRevision(response.headers.get('X-Schedule-Revision')) };
	}

	private async errorMessage(response: Response): Promise<string> {
		const fallback = `Timetower request failed (${response.status} ${response.statusText}).`;
		try {
			const text = (await response.text()).trim();
			if (!text) return fallback;

			const contentType = response.headers.get('content-type') ?? '';
			if (contentType.includes('application/json')) {
				const payload = JSON.parse(text) as { message?: unknown; error?: unknown };
				if (typeof payload.message === 'string' && payload.message) return payload.message;
				if (typeof payload.error === 'string' && payload.error) return payload.error;
			}

			return text.length > 500 ? `${text.slice(0, 497)}...` : text;
		} catch {
			return fallback;
		}
	}

	async listUsers(): Promise<UserSummary[]> {
		const listing = await this.getEnforcers();
		const statuses = new Map<string, UserStatus>();
		const hostnames = new Map<string, string>();

		for (const enforcer of listing.enforcers) {
			for (const status of enforcer.users) {
				if (!hostnames.has(status.user)) hostnames.set(status.user, enforcer.hostname);
				const existing = statuses.get(status.user);
				if (!existing || status.used_seconds > existing.used_seconds) {
					statuses.set(status.user, status);
				}
			}
		}

		const seen = new Set(this.configuredUsers);
		const extras = [...statuses.keys()]
			.filter((username) => !seen.has(username))
			.sort((a, b) => a.localeCompare(b));
		const usernames = [...this.configuredUsers, ...extras];

		return usernames.map((username) => {
			const status = statuses.get(username);
			return {
				username,
				hostname: hostnames.get(username) ?? null,
				used_seconds: status?.used_seconds ?? null,
				daily_limit_seconds: status?.daily_limit_seconds ?? null,
				remaining_seconds: status?.remaining_seconds ?? null,
				allowed: status?.allowed ?? null
			} satisfies UserSummary;
		});
	}

	async getSchedule(username: string): Promise<ScheduleSnapshot> {
		const result = await this.request<Schedule>('GET', `/schedule/${encodeURIComponent(username)}`);
		return { schedule: result.data, revision: result.revision };
	}

	async getHistory(_username: string, _days = 7): Promise<DayUsage[]> {
		throw new ServiceError(
			'Historical usage is not exposed by the Timetower API yet.',
			501,
			'not_implemented'
		);
	}

	async getEnforcers(): Promise<EnforcerList> {
		const result = await this.request<EnforcerList>('GET', '/enforcers');
		return result.data;
	}

	async putOverride(username: string, request: OverrideRequest): Promise<Override> {
		const result = await this.request<Override>(
			'PUT',
			`/override/${encodeURIComponent(username)}`,
			request
		);
		return result.data;
	}
}
