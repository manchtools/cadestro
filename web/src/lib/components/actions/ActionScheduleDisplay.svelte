<script lang="ts">
	import type { ActionSchedule } from '$sdk/powermanage/v1/actions_pb';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		schedule?: ActionSchedule;
		class?: string;
	}

	let { schedule, class: className }: Props = $props();

	function yesNo(value: boolean): string {
		return value ? m.actions_display_yes() : m.actions_display_no();
	}
</script>

<div class="text-sm space-y-2 {className}">
	{#if schedule}
		{#if schedule.cron}
			<p><span class="text-muted-foreground">{m.actions_display_cron()}:</span> <code class="bg-muted px-1 rounded">{schedule.cron}</code></p>
		{:else}
			<p><span class="text-muted-foreground">{m.actions_schedule_interval()}:</span> {m.actions_display_interval({ hours: schedule.intervalHours || 8 })}</p>
		{/if}
		<p><span class="text-muted-foreground">{m.actions_display_run_on_assign()}:</span> {yesNo(schedule.runOnAssign)}</p>
		<p><span class="text-muted-foreground">{m.actions_display_skip_if_unchanged()}:</span> {yesNo(schedule.skipIfUnchanged)}</p>
	{:else}
		<p><span class="text-muted-foreground">{m.actions_schedule_interval()}:</span> {m.actions_display_interval_default()}</p>
		<p><span class="text-muted-foreground">{m.actions_display_run_on_assign()}:</span> {m.actions_display_run_on_assign_default()}</p>
	{/if}
</div>
