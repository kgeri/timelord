<script lang="ts">
	import type { HostStatus } from '#lib/api/types';
	import { formatDuration, formatRelativeTime } from '#lib/utils/format';
	import { hostColor } from '#lib/utils/hosts';

	let { hosts, totalSeconds }: { hosts: HostStatus[]; totalSeconds: number } = $props();

	const showShare = $derived(hosts.length > 1 && totalSeconds > 0);

	function state(host: HostStatus): 'online' | 'offline' | 'unknown' {
		if (host.online === null) return 'unknown';
		return host.online ? 'online' : 'offline';
	}

	function stateLabel(host: HostStatus): string {
		const seen = formatRelativeTime(host.last_seen);
		if (host.online === null) {
			return host.last_seen ? `No recent report (last seen ${seen})` : 'No recent report';
		}
		return `${host.online ? 'Online' : 'Offline'}, last seen ${seen}`;
	}

	function share(host: HostStatus): number {
		if (totalSeconds <= 0) return 0;
		return Math.round((host.used_seconds / totalSeconds) * 100);
	}
</script>

{#if hosts.length > 0}
	<div class="host-breakdown">
		<div class="section-label">Usage by Host</div>
		{#each hosts as host (host.hostname)}
			<div class="host-row" title={`${host.hostname}: ${formatDuration(host.used_seconds)}`}>
				<span
					class="host-color"
					style={`background-color: ${hostColor(host.hostname)}`}
					aria-hidden="true"
				></span>
				<span class="host-name">{host.hostname}</span>
				<span
					class={`host-state ${state(host)}`}
					title={stateLabel(host)}
					aria-label={stateLabel(host)}
				></span>
				<span class="host-usage">{formatDuration(host.used_seconds)}</span>
				{#if showShare}
					<span class="host-share">{share(host)}%</span>
				{/if}
			</div>
		{/each}
	</div>
{/if}
