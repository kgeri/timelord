<script lang="ts">
	import { dashboard } from '#lib/state/dashboard.svelte';
	import { formatRelativeTime } from '#lib/utils/format';

	const revisionLabel = $derived(dashboard.revision === null ? 'r—' : `r${dashboard.revision}`);
	const syncLabel = $derived(
		dashboard.syncing
			? 'Syncing…'
			: dashboard.error
				? 'Sync failed'
				: dashboard.loading
					? 'Connecting…'
					: 'Enforcer Active'
	);
	const dotClass = $derived(
		dashboard.syncing
			? 'syncing'
			: dashboard.error
				? 'error'
				: dashboard.loading
					? 'syncing'
					: 'live'
	);
	const lastSynced = $derived(
		dashboard.lastSyncedAt ? formatRelativeTime(dashboard.lastSyncedAt.toISOString()) : 'not yet'
	);
</script>

<header>
	<div class="brand">
		<div class="brand-icon" aria-hidden="true">⏳</div>
		<div>
			<div class="brand-title">
				Timetower
				<span class="revision-tag">{revisionLabel}</span>
			</div>
			<div class="brand-subtitle">
				<span class={`dot-live ${dotClass}`} aria-hidden="true"></span>
				{syncLabel}
			</div>
		</div>
	</div>
	<button
		class="icon-btn"
		type="button"
		title={`Sync now (last synced ${lastSynced})`}
		aria-label="Sync now"
		onclick={() => void dashboard.refresh()}
		disabled={dashboard.syncing || dashboard.loading || dashboard.userLoading || dashboard.mutating}
	>
		<span class:spin={dashboard.syncing} aria-hidden="true">↻</span>
	</button>
</header>
