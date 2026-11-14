<script lang="ts">
	import OverrideForm from '#lib/components/OverrideForm.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import { dashboard } from '#lib/state/dashboard.svelte';
	import { formatDuration } from '#lib/utils/format';

	const status = $derived(dashboard.status);
	const limitLabel = $derived(
		!status.hasPolicy ? '—' : status.unlimited ? 'No limit' : formatDuration(status.limitSeconds)
	);
	const usedLabel = $derived(
		!status.hasPolicy ? 'No policy' : `${formatDuration(status.usedSeconds)} used`
	);
	const remainingLabel = $derived(
		!status.hasPolicy
			? ''
			: status.unlimited
				? 'Unlimited'
				: `${formatDuration(status.remainingSeconds)} left`
	);
	const busy = $derived(dashboard.mutating);
	const quickAdds = [15, 30, 45, 60];
</script>

<div class="card">
	<div class="card-header">
		<div class="user-title-group">
			<div class="username-header">{dashboard.currentUser}</div>
			<div class="window-label">{status.windowLabel}</div>
		</div>
		<div class="limit-group">
			<div class="section-label section-label--flush">Daily Limit</div>
			<div class="limit-value">{limitLabel}</div>
		</div>
	</div>

	<UsageBar
		percent={status.percentUsed}
		{usedLabel}
		{remainingLabel}
		statusLabel={status.label}
		kind={status.kind}
	/>
</div>

<div class="card">
	<div class="section-label">Quick Time Add</div>
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
</div>

<OverrideForm />
