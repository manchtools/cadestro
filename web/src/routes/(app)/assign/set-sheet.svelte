<script lang="ts">
	// B2's right sheet: pick ONE action set (the selected one expands to its real
	// steps), then pick the schedule. Nothing here commits — the pill owns Save.
	import * as m from '$lib/paraglide/messages';
	// The app's canonical type mapping, not the SDK's `actionTypeToString`: that
	// one has no case for unsupported built-ins and falls through to "UNSPECIFIED", which
	// would mislabel a real step.
	import { getActionTypeLabel } from '$lib/components/actions/action-type';
	import type { ActionSet, ActionSetMember } from '$contract/cadestro/v1/control_pb';
	import type { AssignSchedule } from './assign-data';

	let {
		sets,
		loading = false,
		selectedId,
		steps,
		stepsLoading = false,
		schedule,
		showSchedule = true,
		onselect,
		onschedule
	}: {
		sets: ActionSet[];
		loading?: boolean;
		selectedId: string | null;
		steps: ActionSetMember[];
		stepsLoading?: boolean;
		schedule: AssignSchedule;
		/** Off for a rule target: the group assignment is evaluated by agents and the rule's
		 *  matches are the server's, so there is no "now" to offer. */
		showSchedule?: boolean;
		onselect: (id: string) => void;
		onschedule: (schedule: AssignSchedule) => void;
	} = $props();

	// Immediate assignments sync the device; scheduled assignments use the set policy.
	const SCHEDULES: { value: AssignSchedule; label: () => string; hint: () => string }[] = [
		{ value: 'now', label: m.assign_schedule_now, hint: m.assign_schedule_now_hint },
		{
			value: 'on_schedule',
			label: m.assign_schedule_on_schedule,
			hint: m.assign_schedule_on_schedule_hint
		}
	];

	const activeHint = $derived(
		SCHEDULES.find((s) => s.value === schedule)?.hint ?? m.assign_schedule_now_hint
	);
</script>

<div class="flex min-w-0 flex-col gap-2 border-l border-border bg-surface p-4">
	<div class="font-mono text-[0.66rem] uppercase tracking-[0.08em] text-faint">
		{m.assign_sets_label()}
	</div>

	{#if loading}
		<p class="text-xs text-muted-foreground">{m.assign_sets_loading()}</p>
	{:else if !sets.length}
		<p class="text-xs text-muted-foreground">{m.assign_sets_empty()}</p>
	{:else}
		<div data-tour="assign-sets" role="radiogroup" aria-label={m.assign_sets_label()} class="grid gap-1.5">
			{#each sets as set (set.id)}
				{@const on = (set.id?.value ?? '') === selectedId}
				<div class="overflow-hidden rounded-[9px] border {on ? 'border-primary' : 'border-border'}">
					<button
						type="button"
						role="radio"
						aria-checked={on}
						onclick={() => onselect((set.id?.value ?? ''))}
						class="flex w-full items-center gap-2 px-2 py-2 text-left text-sm {on ? 'bg-accent-soft' : ''}"
					>
						<span
							class="h-3.5 w-3.5 shrink-0 rounded-full border-[1.5px] {on
								? 'border-[0.28rem] border-primary'
								: 'border-border-strong'}"
						></span>
						<span class="min-w-0 truncate font-semibold">{set.name}</span>
						<span class="ml-auto shrink-0 font-mono text-[0.66rem] text-faint">
							{m.assign_set_steps({ count: set.memberCount })}
						</span>
					</button>
					{#if on}
						<div data-testid="assign-set-steps" class="grid gap-0.5 bg-surface px-2 pb-2 pl-8 font-mono text-[0.68rem] text-muted-foreground">
							{#if stepsLoading}
								<span>{m.assign_steps_loading()}</span>
							{:else}
								{#each steps as step, index (step.actionId?.value ?? '')}
									<span>{index + 1} · {getActionTypeLabel(step.actionType)} · {step.actionName}</span>
								{/each}
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	{#if showSchedule}
		<div class="mt-auto pt-3">
			<div class="mb-1 font-mono text-[0.66rem] uppercase tracking-[0.08em] text-faint">
				{m.assign_schedule_label()}
			</div>
			<div
				role="radiogroup"
				aria-label={m.assign_schedule_label()}
				class="inline-flex overflow-hidden rounded-[7px] border border-border font-mono text-[0.68rem]"
			>
				{#each SCHEDULES as option (option.value)}
					<button
						type="button"
						role="radio"
						aria-checked={schedule === option.value}
						onclick={() => onschedule(option.value)}
						class="border-r border-border px-2 py-1 last:border-r-0 {schedule === option.value
							? 'bg-accent-soft font-semibold text-accent-ink'
							: 'text-faint'}"
					>
						{option.label()}
					</button>
				{/each}
			</div>
			<p class="mt-1 text-xs text-faint">{activeHint()}</p>
		</div>
	{/if}
</div>
