<script lang="ts">
	interface Props {
		title: string;
		message?: string;
		kind?: 'loading' | 'error' | 'empty';
		actionLabel?: string;
		onaction?: () => void;
	}

	let { title, message, kind = 'loading', actionLabel, onaction }: Props = $props();
</script>

<div class="state-panel card" role={kind === 'error' ? 'alert' : 'status'}>
	{#if kind === 'loading'}
		<div class="spinner" aria-hidden="true"></div>
	{/if}
	<div class="state-title">{title}</div>
	{#if message}
		<div class="state-message">{message}</div>
	{/if}
	{#if actionLabel && onaction}
		<button type="button" class="btn-submit state-action" onclick={onaction}>
			{actionLabel}
		</button>
	{/if}
</div>
