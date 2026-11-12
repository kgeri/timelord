import { HttpTimetowerService } from './http';
import { MockTimetowerService } from './mock';
import type { TimetowerService } from './types';

export { HttpTimetowerService } from './http';
export { MockTimetowerService } from './mock';
export { ServiceError } from './errors';
export * from './types';

/**
 * Build the service for the current Vite mode.
 *
 * `npm run mock` starts Vite in `mock` mode and uses in-memory fixtures. Every
 * other mode talks to the same-origin Timetower API.
 */
export function createTimetowerService(): TimetowerService {
	return import.meta.env.MODE === 'mock' ? new MockTimetowerService() : new HttpTimetowerService();
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
