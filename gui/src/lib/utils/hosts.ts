import type { DayUsage } from '#lib/api/types';

/**
 * Stable palette for per-host usage. The color is derived from the hostname so
 * the status and stats views agree without sharing component state.
 */
const HOST_COLORS = [
	'#818cf8',
	'#34d399',
	'#fbbf24',
	'#f472b6',
	'#22d3ee',
	'#c084fc',
	'#fb923c',
	'#a3e635'
];

export function hostColor(hostname: string): string {
	let hash = 0;
	for (let index = 0; index < hostname.length; index += 1) {
		hash = (hash * 31 + hostname.charCodeAt(index)) >>> 0;
	}
	return HOST_COLORS[hash % HOST_COLORS.length];
}

/** All hostnames that appear in the history, sorted for a stable legend. */
export function hostsInHistory(history: DayUsage[]): string[] {
	const hosts = new Set<string>();
	for (const day of history) {
		for (const host of day.hosts ?? []) {
			hosts.add(host.hostname);
		}
	}
	return [...hosts].sort((a, b) => a.localeCompare(b));
}

/** Total usage for one host across the history period. */
export function hostTotal(history: DayUsage[], hostname: string): number {
	let total = 0;
	for (const day of history) {
		for (const host of day.hosts ?? []) {
			if (host.hostname === hostname) total += host.used_seconds;
		}
	}
	return total;
}
