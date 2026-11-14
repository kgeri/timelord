<script lang="ts">
	import StatePanel from '#lib/components/StatePanel.svelte';
	import { dashboard } from '#lib/state/dashboard.svelte';
	import { dayInitial, dayLabel, formatHours, shortDate } from '#lib/utils/format';

	const maxSeconds = $derived(
		Math.max(3 * 60 * 60, ...dashboard.history.map((day) => day.used_seconds), 1)
	);

	const chartLabel = $derived(
		dashboard.history
			.map((day) => `${dayLabel(day.date)} ${shortDate(day.date)}: ${formatHours(day.used_seconds)}`)
			.join(', ')
	);
</script>

<div class="card">
	<div class="section-label">Past 7 Days Usage (Hours)</div>

	{#if dashboard.historyError}
		<StatePanel
			kind="empty"
			title="History unavailable"
			message={dashboard.historyError}
			actionLabel="Retry"
			onaction={() => void dashboard.reloadHistory()}
		/>
	{:else if dashboard.history.length === 0}
		<StatePanel kind="empty" title="No history yet" message="Usage will appear here after the first reports." />
	{:else}
		<div class="bar-chart-container" role="img" aria-label={chartLabel}>
			{#each dashboard.history as day (day.date)}
				<div
					class="bar-col"
					title={`${dayLabel(day.date)} ${shortDate(day.date)}: ${formatHours(day.used_seconds)}`}
				>
					<div class="bar-plot">
						<div
							class="bar-fill"
							style={`height: ${(day.used_seconds / maxSeconds) * 100}%`}
						>
							<span class="bar-value">{formatHours(day.used_seconds)}</span>
						</div>
					</div>
					<div class="bar-label">{dayInitial(day.date)}</div>
				</div>
			{/each}
		</div>
		<div class="chart-caption">
			<span>{dayLabel(dashboard.history[0]?.date ?? '')} – {dayLabel(dashboard.history.at(-1)?.date ?? '')}</span>
			<span>{formatHours(dashboard.history.reduce((sum, day) => sum + day.used_seconds, 0))} total</span>
		</div>
	{/if}
</div>
