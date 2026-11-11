import { getTimetowerService } from '#lib/api';
import { ServiceError } from '#lib/api/errors';
import type {
	DayUsage,
	EnforcerStatus,
	OverrideRequest,
	Schedule,
	TimetowerService,
	UserSummary
} from '#lib/api/types';
import { deriveScheduleStatus, type ScheduleStatus } from '#lib/domain/schedule';
import { messageFromError } from '#lib/utils/errors';

export type TabKey = 'status' | 'override' | 'stats' | 'nodes';

/**
 * One shared store for the single-screen admin console. Svelte 5 runes make the
 * store reactive without a separate library, and keeping actions here means
 * components stay presentational.
 */
class Dashboard {
	private readonly service: TimetowerService;
	private requestSequence = 0;
	private initialized = false;

	tab = $state<TabKey>('status');
	users = $state<UserSummary[]>([]);
	currentUser = $state('');
	revision = $state<number | null>(null);
	schedule = $state<Schedule | null>(null);
	history = $state<DayUsage[]>([]);
	enforcers = $state<EnforcerStatus[]>([]);
	enforcersServerTime = $state<string | null>(null);
	enforcersServerDay = $state<string | null>(null);

	loading = $state(true);
	userLoading = $state(false);
	syncing = $state(false);
	mutating = $state(false);

	error = $state<string | null>(null);
	historyError = $state<string | null>(null);
	nodesError = $state<string | null>(null);
	lastSyncedAt = $state<Date | null>(null);

	status: ScheduleStatus = $derived(deriveScheduleStatus(this.schedule));
	currentSummary: UserSummary | null = $derived(
		this.users.find((user) => user.username === this.currentUser) ?? null
	);

	constructor(service: TimetowerService = getTimetowerService()) {
		this.service = service;
	}

	async init(): Promise<void> {
		if (this.initialized) return;
		this.initialized = true;
		this.loading = true;
		this.error = null;

		try {
			this.users = await this.service.listUsers();
			const first = this.users[0]?.username ?? '';
			if (!first) {
				throw new ServiceError(
					'No monitored users are configured. Set PUBLIC_TIMETOWER_USERS.',
					404,
					'no_users'
				);
			}

			this.currentUser = first;
			await this.loadUser(first);
			await this.loadEnforcers();
			this.lastSyncedAt = new Date();
		} catch (error) {
			this.error = messageFromError(error);
			this.initialized = false;
		} finally {
			this.loading = false;
		}
	}

	private async loadUser(username: string): Promise<void> {
		const sequence = ++this.requestSequence;
		const snapshot = await this.service.getSchedule(username);
		if (sequence !== this.requestSequence) return;

		this.schedule = snapshot.schedule;
		this.revision = snapshot.revision;
		this.historyError = null;

		try {
			const history = await this.service.getHistory(username);
			if (sequence === this.requestSequence) this.history = history;
		} catch (error) {
			if (sequence === this.requestSequence) {
				this.history = [];
				this.historyError = messageFromError(error);
			}
		}
	}

	private async reloadUsers(): Promise<void> {
		try {
			this.users = await this.service.listUsers();
		} catch {
			// The carousel is a convenience; a failure here must not hide the
			// schedule that is already on screen.
		}
	}

	async loadEnforcers(): Promise<void> {
		try {
			const listing = await this.service.getEnforcers();
			this.enforcers = listing.enforcers;
			this.enforcersServerTime = listing.server_time;
			this.enforcersServerDay = listing.server_day;
			this.nodesError = null;
		} catch (error) {
			this.nodesError = messageFromError(error);
		}
	}

	async selectUser(username: string): Promise<void> {
		if (username === this.currentUser || this.userLoading) return;

		this.currentUser = username;
		this.schedule = null;
		this.revision = null;
		this.history = [];
		this.historyError = null;
		this.error = null;
		this.userLoading = true;
		this.tab = 'status';

		try {
			await this.loadUser(username);
			this.lastSyncedAt = new Date();
		} catch (error) {
			this.error = messageFromError(error);
		} finally {
			this.userLoading = false;
		}
	}

	async refresh(): Promise<void> {
		if (this.syncing || this.loading || this.userLoading || this.mutating || !this.currentUser) return;

		this.syncing = true;
		this.error = null;
		try {
			await Promise.all([
				this.loadUser(this.currentUser),
				this.reloadUsers(),
				this.loadEnforcers()
			]);
			this.lastSyncedAt = new Date();
		} catch (error) {
			this.error = messageFromError(error);
		} finally {
			this.syncing = false;
		}
	}

	async reloadHistory(): Promise<void> {
		if (!this.currentUser) return;
		const username = this.currentUser;
		this.historyError = null;
		try {
			const history = await this.service.getHistory(username);
			if (username === this.currentUser) this.history = history;
		} catch (error) {
			if (username === this.currentUser) {
				this.history = [];
				this.historyError = messageFromError(error);
			}
		}
	}

	switchTab(tab: TabKey): void {
		this.tab = tab;
		if (tab === 'nodes' && this.enforcers.length === 0 && !this.nodesError) {
			void this.loadEnforcers();
		}
	}

	dismissError(): void {
		this.error = null;
	}

	/** Add minutes to the effective daily limit. */
	async addMinutes(minutes: number): Promise<boolean> {
		if (!this.schedule || this.mutating) return false;
		const baseMinutes =
			this.status.limitSeconds > 0
				? Math.ceil(this.status.limitSeconds / 60)
				: Math.ceil(this.status.usedSeconds / 60);
		// If usage is already above the limit, start from usage so the tap adds
		// the requested time instead of merely closing the existing gap.
		const overLimitMinutes = Math.ceil(this.status.usedSeconds / 60);
		return this.setOverride({
			limit_minutes: Math.max(baseMinutes, overLimitMinutes) + minutes
		});
	}

	/**
	 * Set the limit to the time already used, so the next enforcement decision
	 * reports zero remaining. Limits have minute precision, so flooring is what
	 * guarantees `used_seconds >= daily_limit_seconds`.
	 */
	async lockNow(): Promise<boolean> {
		if (!this.schedule || this.mutating) return false;
		return this.setOverride({ limit_minutes: Math.floor(this.status.usedSeconds / 60) });
	}

	/** Apply the override form and return to the status tab on success. */
	async applyOverride(request: OverrideRequest): Promise<boolean> {
		const ok = await this.setOverride(request);
		if (ok) this.tab = 'status';
		return ok;
	}

	private async setOverride(request: OverrideRequest): Promise<boolean> {
		if (!this.currentUser || this.mutating) return false;

		this.mutating = true;
		this.error = null;
		try {
			await this.service.putOverride(this.currentUser, request);
		} catch (error) {
			this.error = messageFromError(error);
			this.mutating = false;
			return false;
		}

		try {
			await this.loadUser(this.currentUser);
			await this.reloadUsers();
			this.lastSyncedAt = new Date();
		} catch (error) {
			this.error = `Override applied, but refreshing failed: ${messageFromError(error)}`;
		} finally {
			this.mutating = false;
		}
		return true;
	}
}

export const dashboard = new Dashboard();
