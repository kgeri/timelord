<script lang="ts">
	import StatePanel from '#lib/components/StatePanel.svelte';
	import { dashboard } from '#lib/state/dashboard.svelte';
	import type { DayUsage } from '#lib/api/types';
	import { dayInitial, dayLabel, formatHours, shortDate } from '#lib/utils/format';
	import { hostColor, hostTotal, hostsInHistory } from '#lib/utils/hosts';

	const maxSeconds = $derived(
		Math.max(3 * 60 * 60, ...dashboard.history.map((day) => day.used_seconds), 1)
	);
	const hostNames = $derived(hostsInHistory(dashboard.history));

	function dayTitle(day: DayUsage): string {
		const hosts = (day.hosts ?? []).map(
			(host) => `${host.hostname} ${formatHours(host.used_seconds)}`
		);
		const details = hosts.length > 0 ? ` (${hosts.join(', ')})` : '';
		return `${dayLabel(day.date)} ${shortDate(day.date)}: ${formatHours(day.used_seconds)}${details}`;
	}

	const chartLabel = $derived(dashboard.history.map(dayTitle).join(', '));
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
				{@const hosts = day.hosts ?? []}
				<div class="bar-col" title={dayTitle(day)}>
					<div class="bar-plot">
						<div
							class="bar-stack"
							style={`height: ${(day.used_seconds / maxSeconds) * 100}%`}
						>
							{#each hosts as host (host.hostname)}
								<div
									class="bar-segment"
									style={`flex-grow: ${host.used_seconds}; background-color: ${hostColor(host.hostname)}`}
									title={`${host.hostname}: ${formatHours(host.used_seconds)}`}
								></div>
							{/each}
							{#if hosts.length === 0}
								<div
									class={`bar-segment ${day.used_seconds > 0 ? 'bar-segment--legacy' : 'bar-segment--empty'}`}
								></div>
							{/if}
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
		{#if hostNames.length > 0}
			<div class="host-legend" aria-label="Usage by host">
				{#each hostNames as hostname (hostname)}
					<span class="legend-item" title={`${hostname}: ${formatHours(hostTotal(dashboard.history, hostname))} over the period`}>
						<span
							class="legend-swatch"
							style={`background-color: ${hostColor(hostname)}`}
							aria-hidden="true"
						></span>
						<span class="legend-name">{hostname}</span>
						<span class="legend-total">{formatHours(hostTotal(dashboard.history, hostname))}</span>
					</span>
				{/each}
			</div>
		{/if}
	{/if}
</div>
