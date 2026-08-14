<script lang="ts">
	// The one way a schedule states itself outside its editor: a labelled,
	// clickable summary. Actions, action sets and definitions all showed their
	// schedule differently — an action put it in a rail, a container gave it a
	// full body card with a paragraph of prose — for the same piece of state.
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
		/** Click-to-reveal. Omitted where the pill is the only way in. */
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
