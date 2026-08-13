<script lang="ts">
	import { untrack } from 'svelte';
	import type { ActionSchedule } from '$sdk/powermanage/v1/actions_pb';
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

	// Card surfacing the schedule of an action set or definition. The
	// container's schedule fires every member action when triggered (see
	// manchtools/power-manage-agent#45) — that's what the description
	// callout reinforces. Edit opens a dialog backed by the existing
	// per-action ActionScheduleForm so cron/interval/run-on-assign/skip-
	// if-unchanged behavior is identical.
	interface Props {
		schedule?: ActionSchedule;
		label?: string;
		description?: string;
		saving?: boolean;
		/** Opens the schedule editor from outside — the entity's pill owns this
		 *  action now, so the trigger no longer has to live on the card. */
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

	// Seeding rides the OPEN, not the trigger: the pill opens this dialog from
	// outside the card, so a form seeded only inside a local click handler would
	// come up holding the previous edit.
	//
	// Dismissing rides the CLOSE for the same reason the action's schedule
	// dialog has a single Done button: the entity's one commit lives on the
	// pill, and a second Save inside a dialog opened from that pill is a
	// competing commit path. The container schedule is server state that the
	// builder's draft does not carry (it is its own RPC), so the honest single
	// path is "dismiss = commit": Done, Esc and the overlay all write once,
	// through the same `onsave`, and only when something actually changed.
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
		// An open-and-close with no edit is not a write: it would toast "Schedule
		// updated" at an operator who only looked.
		if (JSON.stringify($state.snapshot(formState)) === seededJson) return;
		const next = scheduleFormToProto(formState);
		saving = true;
		try {
			await onsave(next);
		} catch {
			// The page has already reported the failure. Reopen on the operator's
			// edit rather than dropping it — `seeded` stays true so the reopen does
			// not reseed the form from the schedule the server still holds.
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

<!-- The SAME summary an action shows: the schedule states itself in one line
     and is revealed by clicking it (or the entity's pill action). It used to be
     a full body card with a paragraph of prose — the container's schedule
     "listed in the form" the operator objected to. -->
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
