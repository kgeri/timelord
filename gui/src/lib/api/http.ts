import { ServiceError } from './errors';
import type {
	DayUsage,
	EnforcerList,
	Override,
	OverrideRequest,
	Schedule,
	ScheduleSnapshot,
	TimetowerService,
	UserList,
	UserSummary
} from './types';

export interface HttpTimetowerServiceOptions {
	/** Override `fetch`, primarily for tests. */
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
 * Requests always use same-origin relative URLs: the admin UI is served next to
 * the API. HTTP Basic credentials are handled by the browser (`admin` plus
 * `TIMELORD_ADMIN_PASSWORD`), so the frontend never stores a secret.
 */
export class HttpTimetowerService implements TimetowerService {
	readonly kind = 'http' as const;

	private readonly fetchImpl: typeof fetch;

	constructor(options: HttpTimetowerServiceOptions = {}) {
		this.fetchImpl = options.fetch ?? fetch;
	}

	private async request<T>(
		method: 'GET' | 'PUT',
		path: string,
		body?: unknown,
		signal?: AbortSignal
	): Promise<ApiResult<T>> {
		const headers = new Headers({ Accept: 'application/json' });
		if (body !== undefined) headers.set('Content-Type', 'application/json');

		let response: Response;
		try {
			response = await this.fetchImpl(`/api/v1${path}`, {
				method,
				headers,
				body: body === undefined ? undefined : JSON.stringify(body),
				signal,
				credentials: 'same-origin'
			});
		} catch (error) {
			throw new ServiceError(
				error instanceof Error ? `Could not reach Timetower: ${error.message}` : 'Could not reach Timetower.',
				0,
				'network'
			);
		}

		if (!response.ok) {
			throw new ServiceError(
				await this.errorMessage(response),
				response.status,
				`http_${response.status}`
			);
		}

		const text = await response.text();
		const data = (text ? JSON.parse(text) : undefined) as T;
		return { data, revision: parseRevision(response.headers.get('X-Schedule-Revision')) };
	}

	private async errorMessage(response: Response): Promise<string> {
		if (response.status === 401) return 'Admin authentication required.';

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
		const result = await this.request<UserList>('GET', '/users');
		return result.data.users.map((user) => ({
			username: user.user,
			display_name: user.display_name,
			hostname: null,
			used_seconds: null,
			daily_limit_seconds: null,
			remaining_seconds: null,
			allowed: null
		}));
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
