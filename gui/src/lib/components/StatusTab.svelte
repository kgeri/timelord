<script lang="ts">
	import UsageRing from '#lib/components/UsageRing.svelte';
	import { dashboard } from '#lib/state/dashboard.svelte';
	import { formatDuration } from '#lib/utils/format';

	const status = $derived(dashboard.status);
	const limitLabel = $derived(
		!status.hasPolicy ? '—' : status.unlimited ? 'No limit' : formatDuration(status.limitSeconds)
	);
	const remainingLabel = $derived(
		!status.hasPolicy ? '—' : status.unlimited ? '∞' : formatDuration(status.remainingSeconds)
	);
	const percentLabel = $derived(
		!status.hasPolicy
			? 'No policy'
			: status.unlimited
				? 'No limit'
				: `${status.percentUsed}% Used`
	);
	const busy = $derived(dashboard.mutating);
	const quickAdds = [15, 30, 45, 60];
</script>

<div class="card">
	<div class="card-header">
		<div class="user-title-group">
			<div class="username-header">{dashboard.currentUser}</div>
			<div class={`status-badge ${status.badgeClass}`}>{status.label}</div>
		</div>
		<div class="limit-group">
			<div class="section-label section-label--flush">Daily Limit</div>
			<div class="limit-value">{limitLabel}</div>
		</div>
	</div>

	<UsageRing
		percent={status.percentUsed}
		usedLabel={formatDuration(status.usedSeconds)}
		{percentLabel}
		danger={status.limitReached}
	/>

	<div class="metrics-grid">
		<div class="metric-box">
			<div class="section-label section-label--flush">Remaining</div>
			<div class="metric-val metric-val--positive">{remainingLabel}</div>
		</div>
		<div class="metric-box">
			<div class="section-label section-label--flush">Active Window</div>
			<div class="metric-val">{status.windowLabel}</div>
		</div>
	</div>
</div>

<div class="card card--spaced">
	<div class="section-label">Quick Time Add (1-Tap Override)</div>
	<div class="action-grid">
		{#each quickAdds as minutes (minutes)}
			<button
				type="button"
				class="btn-quick"
				disabled={busy}
				onclick={() => void dashboard.addMinutes(minutes)}
			>
				+{minutes}m
			</button>
		{/each}
	</div>
	<button type="button" class="btn-lock" disabled={busy} onclick={() => void dashboard.lockNow()}>
		<span aria-hidden="true">🔒</span>
		{busy ? 'Applying…' : "LOCK IMMEDIATELY (Time's Up)"}
	</button>
	<div class="action-note">Changes apply to the current policy day only.</div>
</div>
