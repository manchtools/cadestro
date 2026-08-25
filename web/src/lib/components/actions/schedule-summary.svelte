<script lang="ts">

	import type { ActionSchedule } from '$contract/cadestro/v1/actions_pb';
	import ActionScheduleDisplay from './ActionScheduleDisplay.svelte';
	import * as m from '$lib/paraglide/messages';

	let {
		schedule,
		label = m.container_schedule_title(),
		onedit
	}: {
		schedule?: ActionSchedule;
		label?: string;

		onedit?: () => void;
	} = $props();
</script>

<div class="space-y-1.5">
	<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{label}</span>
	{#if onedit}
		<button
			type="button"
			onclick={onedit}
			data-testid="schedule-summary-edit"
			aria-label={label}
			class="block w-full rounded-lg text-left transition-colors hover:bg-accent/50"
		>
			<ActionScheduleDisplay {schedule} />
		</button>
	{:else}
		<ActionScheduleDisplay {schedule} />
	{/if}
</div>
