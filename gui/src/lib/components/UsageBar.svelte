<script lang="ts">
	import type { StatusKind } from '#lib/domain/schedule';

	interface Props {
		percent: number;
		usedLabel: string;
		remainingLabel: string;
		statusLabel: string;
		kind: StatusKind;
	}

	let { percent, usedLabel, remainingLabel, statusLabel, kind }: Props = $props();

	const clamped = $derived(Math.max(0, Math.min(100, percent)));
</script>

<div
	class={`usage-bar usage-bar--${kind}`}
	role="img"
	aria-label={`${statusLabel}. ${usedLabel}. ${remainingLabel}`.trim()}
>
	<div class="usage-track">
		<div class="usage-fill" style={`width: ${clamped}%`}></div>
		<div class="usage-text">
			<span>{usedLabel}</span>
			<span>{remainingLabel}</span>
		</div>
	</div>
</div>
