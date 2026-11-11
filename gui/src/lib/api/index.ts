import {
	PUBLIC_TIMETOWER_API_TOKEN,
	PUBLIC_TIMETOWER_API_URL,
	PUBLIC_TIMETOWER_MODE,
	PUBLIC_TIMETOWER_USERS
} from '$app/env/public';
import { HttpTimetowerService } from './http';
import { MockTimetowerService } from './mock';
import type { TimetowerService } from './types';

export { HttpTimetowerService } from './http';
export { MockTimetowerService } from './mock';
export { ServiceError } from './errors';
export * from './types';

function configuredUsers(): string[] {
	return PUBLIC_TIMETOWER_USERS.split(',')
		.map((user) => user.trim())
		.filter(Boolean);
}

/**
 * Build a service from public environment configuration.
 *
 * - `PUBLIC_TIMETOWER_MODE=mock` (default) uses in-memory fixtures.
 * - `PUBLIC_TIMETOWER_MODE=http` talks to the API, usually through the
 *   same-origin proxy so the bearer key stays server-side.
 */
export function createTimetowerService(): TimetowerService {
	const mode = PUBLIC_TIMETOWER_MODE.trim().toLowerCase();
	if (mode === 'http' || mode === 'live') {
		return new HttpTimetowerService({
			baseUrl: PUBLIC_TIMETOWER_API_URL,
			token: PUBLIC_TIMETOWER_API_TOKEN,
			users: configuredUsers()
		});
	}
	return new MockTimetowerService();
}

let instance: TimetowerService | null = null;

export function getTimetowerService(): TimetowerService {
	instance ??= createTimetowerService();
	return instance;
}

/** Replace the singleton, primarily for tests or storybook-style setups. */
export function setTimetowerService(service: TimetowerService | null): void {
	instance = service;
}
