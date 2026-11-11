<script lang="ts">
	import StatePanel from '#lib/components/StatePanel.svelte';
	import { dashboard } from '#lib/state/dashboard.svelte';
	import { formatDuration, formatRelativeTime } from '#lib/utils/format';

	// Relative times use the listing's server clock so they stay consistent with
	// the API for both the mock (fixed demo clock) and a real backend.
	const reference = $derived(
		dashboard.enforcersServerTime ? new Date(dashboard.enforcersServerTime) : new Date()
	);

	function reasonLabel(reason: string | null): string {
		return reason ? reason.replaceAll('_', ' ') : 'allowed';
	}
</script>

<div class="card">
	<div class="nodes-heading">
		<div class="section-label section-label--flush">Enforcer Nodes (/api/v1/enforcers)</div>
		{#if dashboard.enforcers.length > 0}
			<span class="nodes-meta">{dashboard.enforcers.filter((node) => node.online).length}/{dashboard.enforcers.length} online</span>
		{/if}
	</div>

	{#if dashboard.nodesError}
		<StatePanel
			kind="empty"
			title="Could not load enforcers"
			message={dashboard.nodesError}
			actionLabel="Retry"
			onaction={() => void dashboard.loadEnforcers()}
		/>
	{:else if dashboard.enforcers.length === 0}
		<StatePanel
			kind="empty"
			title="No reporting hosts"
			message="Enforcers appear here after their first status report."
		/>
	{:else}
		<div class="nodes-list">
			{#each dashboard.enforcers as node (node.hostname)}
				<div class="node-card">
					<div class="node-row">
						<div class="node-hostname">{node.hostname}</div>
						<span class={`node-status ${node.online ? 'online' : 'offline'}`}>
							{node.online ? 'ONLINE' : 'OFFLINE'}
						</span>
					</div>
					<div class="node-seen">Last contact {formatRelativeTime(node.last_seen, reference)}</div>
					<div class="node-users">
						{#each node.users as user (user.user)}
							<div class="node-user">
								<span class="node-user-name">{user.user}</span>
								<span class="node-user-state">
									{reasonLabel(user.deny_reason)}
									·
									{user.daily_limit_seconds === 0
										? 'no limit'
										: `${formatDuration(user.remaining_seconds)} left`}
								</span>
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
