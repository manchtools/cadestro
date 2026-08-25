<script lang="ts">
	import { untrack } from 'svelte';
	import type { ActionSchedule } from '$contract/cadestro/v1/actions_pb';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import ScheduleSummary from '$lib/components/actions/schedule-summary.svelte';
	import ActionScheduleForm from '$lib/components/actions/forms/ActionScheduleForm.svelte';
	import {
		defaultScheduleForm,
		scheduleFormToProto,
		type ScheduleFormState,
	} from '$lib/components/actions/forms/types';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		schedule?: ActionSchedule;
		label?: string;
		description?: string;
		saving?: boolean;

		editOpen?: boolean;
		onsave: (next: ActionSchedule) => Promise<void>;
	}

	let {
		schedule,
		label = m.container_schedule_title(),
		description = m.container_schedule_description(),
		saving = $bindable(false),
		editOpen = $bindable(false),
		onsave,
	}: Props = $props();
	let formState = $state<ScheduleFormState>(defaultScheduleForm());

	let seeded = false;
	let seededJson = '';
	$effect(() => {
		if (editOpen && !seeded) {
			seeded = true;
			untrack(() => {
				formState = scheduleToForm(schedule);
				seededJson = JSON.stringify($state.snapshot(formState));
			});
		} else if (!editOpen && seeded) {
			seeded = false;
			untrack(() => void persist());
		}
	});

	async function persist() {

		if (JSON.stringify($state.snapshot(formState)) === seededJson) return;
		const next = scheduleFormToProto(formState);
		saving = true;
		try {
			await onsave(next);
		} catch {

			seeded = true;
			editOpen = true;
		} finally {
			saving = false;
		}
	}

	function scheduleToForm(s: ActionSchedule | undefined): ScheduleFormState {
		const base = defaultScheduleForm();
		if (!s) return base;
		return {
			cron: s.cron ?? '',
			intervalHours: s.intervalHours || base.intervalHours,
			runOnAssign: s.runOnAssign,
			skipIfUnchanged: s.skipIfUnchanged,
		};
	}
</script>

<div class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
	<ScheduleSummary {schedule} {label} onedit={() => (editOpen = true)} />
</div>

<Dialog.Root bind:open={editOpen}>
	<Dialog.Content class="sm:max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{label}</Dialog.Title>
			<Dialog.Description>{description}</Dialog.Description>
		</Dialog.Header>
		<div class="py-4">
			<ActionScheduleForm bind:params={formState} />
		</div>
		<p class="text-xs text-muted-foreground">{m.container_schedule_saved_on_close()}</p>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (editOpen = false)} disabled={saving}>
				{m.common_done()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
