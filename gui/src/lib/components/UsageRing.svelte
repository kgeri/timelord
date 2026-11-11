<script lang="ts">
	interface Props {
		percent: number;
		usedLabel: string;
		percentLabel: string;
		danger?: boolean;
	}

	let { percent, usedLabel, percentLabel, danger = false }: Props = $props();

	const clamped = $derived(Math.max(0, Math.min(100, percent)));
</script>

<div class="radial-container">
	<svg
		class="radial-svg"
		viewBox="0 0 36 36"
		role="img"
		aria-label={`${usedLabel} used today, ${percentLabel}`}
	>
		<path
			class="radial-bg"
			d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
		/>
		<path
			class="radial-progress"
			class:danger
			stroke-dasharray={`${clamped}, 100`}
			d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
		/>
	</svg>
	<div class="radial-center-text">
		<div class="timer-display">{usedLabel}</div>
		<div class="timer-label">Used Today</div>
		<div class="timer-pct">{percentLabel}</div>
	</div>
</div>
