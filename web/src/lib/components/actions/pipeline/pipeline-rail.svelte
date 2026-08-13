<script lang="ts">
	// B1's centre column, taken to the Twenty/Airtable idiom the brief calls for:
	// steps stay collapsed to ONE line (number, icon, name, mono summary, state
	// toggle) and the selected step's full config renders in the sibling panel,
	// because the real per-type param forms are far too tall to expand inline.
	//
	// Ordering is keyboard-first: every step carries ↑/↓ buttons, so reorder never
	// depends on a pointer drag. Palette drags are an addition, not the contract.
	import * as m from '$lib/paraglide/messages';
	import { ChevronUp, ChevronDown, X } from '@lucide/svelte';
	import { STEP_DND_TYPE, type RailStep, type StepState } from './types';

	let {
		steps,
		selectedKey,
		onselect,
		onmove,
		onremove,
		onstate,
		oninsertAt,
		emptyLabel,
		dropLabel
	}: {
		steps: RailStep[];
		selectedKey: string | null;
		onselect: (key: string) => void;
		onmove: (index: number, dir: 'up' | 'down') => void;
		onremove: (key: string) => void;
		onstate?: (key: string, next: StepState) => void;
		/** A palette entry dropped at `index` (end of list when index === length). */
		oninsertAt?: (paletteId: string, index: number) => void;
		emptyLabel: string;
		dropLabel: string;
	} = $props();

	const STATE_LABEL: Record<StepState, () => string> = {
		present: m.desired_state_present,
		absent: m.desired_state_absent,
		run: m.action_set_detail_builder_state_run
	};

	const STATE_ON: Record<StepState, string> = {
		present: 'bg-ok-soft text-ok font-semibold',
		absent: 'bg-crit-soft text-crit font-semibold',
		run: 'bg-accent-soft text-accent-ink font-semibold'
	};

	let dropAt = $state<number | null>(null);

	function accepts(e: DragEvent): boolean {
		return !!oninsertAt && !!e.dataTransfer?.types.includes(STEP_DND_TYPE);
	}

	function over(e: DragEvent, index: number) {
		if (!accepts(e)) return;
		e.preventDefault();
		dropAt = index;
	}

	function drop(e: DragEvent, index: number) {
		if (!accepts(e)) return;
		e.preventDefault();
		dropAt = null;
		const id = e.dataTransfer?.getData(STEP_DND_TYPE);
		if (id) oninsertAt?.(id, index);
	}
</script>

<div
	data-tour="builder-pipeline"
	class="rounded-xl border border-hair bg-surface p-2.5"
	role="list"
	aria-label={m.action_set_detail_builder_pipeline()}
	ondragleave={() => (dropAt = null)}
>
	{#each steps as step, index (step.key)}
		{@const Icon = step.icon}
		{@const selected = step.key === selectedKey}
		<div
			role="listitem"
			class="relative {index > 0 ? 'mt-[1.05rem]' : ''}"
			ondragover={(e) => over(e, index)}
			ondrop={(e) => drop(e, index)}
		>
			{#if index > 0}
				<!-- the connecting edge between two steps -->
				<span
					aria-hidden="true"
					class="absolute -top-[0.95rem] left-[1.3rem] h-[0.95rem] w-px bg-border-strong"
				></span>
			{/if}
			{#if dropAt === index}
				<span
					aria-hidden="true"
					class="absolute -top-[0.6rem] left-0 right-0 h-0.5 rounded-full bg-accent-ink"
				></span>
			{/if}

			<div
				class="rounded-[10px] border bg-frame p-1.5 {step.error
					? 'border-crit/55'
					: selected
						? 'border-accent-ink'
						: ''}"
			>
				<div class="flex items-center gap-2">
					<span
						data-step-index={index + 1}
						class="grid h-6 w-6 shrink-0 place-items-center rounded-[7px] bg-sunken font-mono text-[0.76rem] tabular-nums text-muted-foreground"
					>
						{index + 1}
					</span>

					<button
						type="button"
						data-tour={index === 0 ? 'builder-step' : undefined}
						data-step-key={step.key}
						aria-current={selected}
						class="min-w-0 flex-1 text-left"
						onclick={() => onselect(step.key)}
						onfocus={() => onselect(step.key)}
					>
						<span class="flex items-center gap-1.5">
							{#if Icon}
								<span
									class="grid h-[1.2rem] w-[1.2rem] shrink-0 place-items-center rounded-[5px] bg-accent-soft text-accent-ink"
								>
									<Icon class="h-3 w-3" />
								</span>
							{/if}
							<span class="truncate text-[0.84rem] font-semibold">{step.title}</span>
						</span>
						<span class="mt-0.5 block truncate font-mono text-[0.7rem] text-muted-foreground">
							{step.summary}
						</span>
					</button>

					{#if step.state}
						{@const options = step.stateOptions ?? [step.state]}
						<span
							data-testid="step-row-state"
							class="hidden overflow-hidden rounded-[7px] border font-mono text-[0.62rem] sm:inline-flex"
						>
							{#each options as option (option)}
								<button
									type="button"
									disabled={options.length < 2 || !onstate}
									aria-pressed={option === step.state}
									class="px-1.5 py-0.5 uppercase {option === step.state
										? STATE_ON[option]
										: 'text-faint'}"
									onclick={() => onstate?.(step.key, option)}
								>
									{STATE_LABEL[option]()}
								</button>
							{/each}
						</span>
					{/if}

					<span class="flex shrink-0 items-center">
						<button
							type="button"
							class="grid h-6 w-6 place-items-center rounded-md text-muted-foreground disabled:opacity-30"
							disabled={index === 0}
							aria-label={m.action_set_detail_builder_move_up()}
							onclick={() => onmove(index, 'up')}
						>
							<ChevronUp class="h-3.5 w-3.5" />
						</button>
						<button
							type="button"
							class="grid h-6 w-6 place-items-center rounded-md text-muted-foreground disabled:opacity-30"
							disabled={index === steps.length - 1}
							aria-label={m.action_set_detail_builder_move_down()}
							onclick={() => onmove(index, 'down')}
						>
							<ChevronDown class="h-3.5 w-3.5" />
						</button>
						<button
							type="button"
							class="grid h-6 w-6 place-items-center rounded-md text-muted-foreground hover:text-crit"
							aria-label={m.action_set_detail_builder_remove_step()}
							onclick={() => onremove(step.key)}
						>
							<X class="h-3.5 w-3.5" />
						</button>
					</span>
				</div>

				{#if step.error}
					<p
						data-step-error={step.key}
						class="mt-1.5 border-t border-dashed pt-1.5 font-mono text-[0.68rem] text-crit"
					>
						{step.error}
					</p>
				{/if}
			</div>
		</div>
	{/each}

	<!-- tail drop zone doubles as the empty state -->
	<div
		role="group"
		aria-label={dropLabel}
		class="relative {steps.length ? 'mt-[1.05rem]' : ''}"
		ondragover={(e) => over(e, steps.length)}
		ondrop={(e) => drop(e, steps.length)}
	>
		{#if steps.length}
			<span
				aria-hidden="true"
				class="absolute -top-[0.95rem] left-[1.3rem] h-[0.95rem] w-px bg-border-strong"
			></span>
		{/if}
		<p
			class="rounded-[10px] border border-dashed border-border-strong p-2 text-center text-[0.76rem] {dropAt ===
			steps.length
				? 'border-accent-ink text-accent-ink'
				: 'text-faint'}"
		>
			{steps.length ? dropLabel : emptyLabel}
		</p>
	</div>
</div>
