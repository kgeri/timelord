<script lang="ts">
	import type { UserStatus, UserSummary } from '#lib/api/types';
	import { dashboard } from '#lib/state/dashboard.svelte';
	import { formatDuration } from '#lib/utils/format';

	const reported = $derived.by(() => {
		const statuses = new Map<string, UserStatus>();
		for (const node of dashboard.enforcers) {
			for (const status of node.users) {
				const existing = statuses.get(status.user);
				if (!existing || status.used_seconds > existing.used_seconds) {
					statuses.set(status.user, status);
				}
			}
		}
		return statuses;
	});

	const connectivity = $derived.by(() => {
		const states = new Map<string, boolean>();
		for (const node of dashboard.enforcers) {
			for (const status of node.users) {
				states.set(status.user, (states.get(status.user) ?? false) || node.online);
			}
		}
		return states;
	});

	function subtitle(user: UserSummary): string {
		if (user.username === dashboard.currentUser && dashboard.schedule) {
			const status = dashboard.status;
			if (!status.hasPolicy) return 'No policy';
			if (status.limitReached) return 'Locked';
			if (status.kind === 'window') return 'Outside window';
			if (status.unlimited) return 'Unlimited';
			if (status.remainingSeconds !== null) return formatDuration(status.remainingSeconds);
			return 'No limit';
		}

		const status = reported.get(user.username);
		if (status) {
			if (status.deny_reason === 'outside_window') return 'Outside window';
			if (status.deny_reason === 'disabled') return 'Disabled';
			if (status.deny_reason === 'no_policy') return 'No policy';
			if (status.deny_reason === 'daily_limit_reached') return 'Locked';
			if (status.daily_limit_seconds === 0) return 'Unlimited';
			if (status.remaining_seconds !== null) return formatDuration(status.remaining_seconds);
			return 'No limit';
		}

		if (user.remaining_seconds === 0) return 'Locked';
		if (user.allowed === false) return 'Outside window';
		if (user.daily_limit_seconds === 0 && user.remaining_seconds === null) return 'Unlimited';
		if (user.remaining_seconds !== null) return formatDuration(user.remaining_seconds);
		return user.hostname ?? 'No report';
	}

	function label(user: UserSummary): string {
		return user.display_name || user.username;
	}

	function connection(user: UserSummary): 'online' | 'offline' | null {
		const online = connectivity.get(user.username);
		if (online === undefined) return null;
		return online ? 'online' : 'offline';
	}
</script>

<div class="user-carousel-container">
	<div class="user-carousel" aria-label="Monitored users">
		{#each dashboard.users as user (user.username)}
			{@const state = connection(user)}
			<button
				type="button"
				class="user-pill"
				class:active={user.username === dashboard.currentUser}
				aria-pressed={user.username === dashboard.currentUser}
				title={user.username}
				onclick={() => void dashboard.selectUser(user.username)}
			>
				<span class="avatar" aria-hidden="true">{label(user).slice(0, 1).toUpperCase()}</span>
				<span class="pill-info">
					<span class="pill-name">{label(user)}</span>
					<span class="pill-sub">{subtitle(user)}</span>
				</span>
				{#if state}
					<span
						class={`pill-status ${state}`}
						title={`Enforcer ${state}`}
						aria-label={`Enforcer ${state}`}
					></span>
				{/if}
			</button>
		{/each}
	</div>
</div>
