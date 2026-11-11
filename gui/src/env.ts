import { defineEnvVars } from '@sveltejs/kit/env';

/**
 * Explicit environment contract for the admin UI.
 *
 * SvelteKit exposes these through `$app/env/public` (browser-safe) and
 * `$app/env/private` (server-only, used by the proxy route). Defaults keep the
 * app runnable on mock data with no .env file.
 */
export const variables = defineEnvVars({
	PUBLIC_TIMETOWER_MODE: {
		public: true,
		schema: (value) => value?.trim() || 'mock',
		description: 'Data source: "mock" or "http".'
	},
	PUBLIC_TIMETOWER_USERS: {
		public: true,
		schema: (value) => value ?? '',
		description: 'Comma-separated fallback list of monitored users.'
	},
	PUBLIC_TIMETOWER_API_URL: {
		public: true,
		schema: (value) => value?.trim() || '',
		description: 'Optional absolute Timetower origin; empty uses the same-origin proxy.'
	},
	PUBLIC_TIMETOWER_API_TOKEN: {
		public: true,
		schema: (value) => value?.trim() || '',
		description: 'Optional direct-connection bearer key. Not for production.'
	},
	TIMETOWER_API_URL: {
		public: false,
		schema: (value) => value?.trim() || '',
		description: 'Upstream Timetower origin used by the server-side proxy.'
	},
	TIMETOWER_API_TOKEN: {
		public: false,
		schema: (value) => value?.trim() || '',
		description: 'Bearer key injected by the server-side proxy.'
	}
});
