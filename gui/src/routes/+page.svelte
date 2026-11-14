<script lang="ts">
	import { onMount } from 'svelte';
	import BottomNav from '#lib/components/BottomNav.svelte';
	import StatePanel from '#lib/components/StatePanel.svelte';
	import StatsTab from '#lib/components/StatsTab.svelte';
	import StatusTab from '#lib/components/StatusTab.svelte';
	import UserCarousel from '#lib/components/UserCarousel.svelte';
	import { dashboard } from '#lib/state/dashboard.svelte';

	onMount(() => {
		void dashboard.init();
	});
</script>

<svelte:head>
	<title>Timetower Admin Console</title>
	<meta name="theme-color" content="#09090b" />
	<meta
		name="description"
		content="Mobile-first admin console for Timetower schedules, limits, and enforcers."
	/>
</svelte:head>

<div class="app-container">
	{#if dashboard.users.length === 0}
		<main>
			{#if dashboard.loading}
				<StatePanel
					title="Connecting to Timetower"
					message="Loading monitored users and schedules…"
				/>
			{:else}
				<StatePanel
					kind="error"
					title="Timetower unavailable"
					message={dashboard.error ?? 'No users are available.'}
					actionLabel="Retry"
					onaction={() => void dashboard.init()}
				/>
			{/if}
		</main>
	{:else}
		<UserCarousel />

		<main>
			{#if dashboard.error && dashboard.schedule}
				<div class="error-banner" role="alert">
					<span>{dashboard.error}</span>
					<button
						type="button"
						class="error-dismiss"
						aria-label="Dismiss error"
						onclick={() => dashboard.dismissError()}
					>
						×
					</button>
				</div>
			{/if}

			{#if dashboard.userLoading}
				<StatePanel
					title={`Loading ${dashboard.currentUser}`}
					message="Fetching the current schedule and usage…"
				/>
			{:else if !dashboard.schedule}
				<StatePanel
					kind="error"
					title="Could not load this user"
					message={dashboard.error ?? 'The schedule is unavailable.'}
					actionLabel="Retry"
					onaction={() => void dashboard.refresh()}
				/>
			{:else if dashboard.tab === 'status'}
				<StatusTab />
			{:else if dashboard.tab === 'stats'}
				<StatsTab />
			{/if}
		</main>

		<BottomNav />
	{/if}
</div>
