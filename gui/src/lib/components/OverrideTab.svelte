<script lang="ts">
	import { windowForDay } from '#lib/domain/schedule';
	import { dashboard } from '#lib/state/dashboard.svelte';
	import { clockToMinutes } from '#lib/utils/format';

	let windowStart = $state('08:00');
	let windowEnd = $state('20:00');
	let dailyLimit = $state('120');
	let formError = $state<string | null>(null);

	// Reset the editor whenever a different schedule is loaded (user switch or
	// refresh). Typing in the form does not change these dependencies.
	$effect(() => {
		const schedule = dashboard.schedule;
		const window = windowForDay(schedule);
		windowStart = window?.start ?? '08:00';
		windowEnd = window?.end ?? '20:00';
		dailyLimit = String(Math.round((schedule?.policy?.daily_limit_seconds ?? 0) / 60));
		formError = null;
	});

	const busy = $derived(dashboard.mutating);

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		formError = null;

		const start = clockToMinutes(windowStart);
		const end = clockToMinutes(windowEnd);
		if (start === null || end === null) {
			formError = 'Enter a valid start and end time.';
			return;
		}
		if (start >= end) {
			formError = 'The window start must be earlier than the end.';
			return;
		}

		const limit = Number(dailyLimit);
		if (!Number.isInteger(limit) || limit < 0) {
			formError = 'The daily limit must be a whole, non-negative number of minutes.';
			return;
		}

		const ok = await dashboard.applyOverride({
			start: windowStart,
			end: windowEnd,
			limit_minutes: limit
		});
		if (!ok) {
			formError = 'The override was not applied. See the error above for details.';
		}
	}
</script>

<div class="card">
	<div class="section-label section-label--spaced">Policy Window &amp; Limit Override</div>
	<form onsubmit={(event) => void submit(event)}>
		<div class="form-row">
			<div class="form-group">
				<label for="override-start">WINDOW START</label>
				<input id="override-start" type="time" bind:value={windowStart} required />
			</div>
			<div class="form-group">
				<label for="override-end">WINDOW END</label>
				<input id="override-end" type="time" bind:value={windowEnd} required />
			</div>
		</div>
		<div class="form-group">
			<label for="override-limit">DAILY LIMIT (MINUTES)</label>
			<input
				id="override-limit"
				type="number"
				min="0"
				max="1440"
				step="1"
				inputmode="numeric"
				bind:value={dailyLimit}
				required
			/>
			<div class="form-hint">0 minutes means no duration limit.</div>
		</div>
		{#if formError}
			<div class="form-error" role="alert">{formError}</div>
		{/if}
		<button type="submit" class="btn-submit" disabled={busy}>
			{busy ? 'Applying…' : 'Apply PUT Override'}
		</button>
	</form>
</div>
