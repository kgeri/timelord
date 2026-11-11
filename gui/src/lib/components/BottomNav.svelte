<script lang="ts">
	import { dashboard, type TabKey } from '#lib/state/dashboard.svelte';

	const tabs: { key: TabKey; icon: string; label: string }[] = [
		{ key: 'status', icon: '⏱', label: 'Status' },
		{ key: 'override', icon: '⚙', label: 'Override' },
		{ key: 'stats', icon: '📊', label: 'Stats' },
		{ key: 'nodes', icon: '🖥', label: 'Nodes' }
	];
</script>

<nav aria-label="Main">
	{#each tabs as tab (tab.key)}
		<button
			type="button"
			class="nav-btn"
			class:active={dashboard.tab === tab.key}
			aria-pressed={dashboard.tab === tab.key}
			onclick={() => dashboard.switchTab(tab.key)}
		>
			<span aria-hidden="true">{tab.icon}</span>
			<span>{tab.label}</span>
		</button>
	{/each}
</nav>
