<script lang="ts">
	// The action's INSTALL / REMOVE switch — one control, used by the create
	// surface, the detail editor and the pipeline step rows.
	//
	// It is a toggle everywhere, never a dropdown: the state has exactly two
	// values (three counting the types that have only one), and a select for a
	// binary choice hid the current value behind a click.
	import * as m from '$lib/paraglide/messages';

	let {
		value = $bindable(),
		supportsAbsent = true,
		size = 'md',
		label = m.action_detail_desired_state()
	}: {
		/** Proto DesiredState: 0 = PRESENT, 1 = ABSENT. */
		value: number;
		/** Types with no ABSENT are not offered a dead second option — they show
		 *  the single RUN state they actually have. */
		supportsAbsent?: boolean;
		size?: 'sm' | 'md';
		label?: string;
	} = $props();

	const states = $derived(
		supportsAbsent
			? [
					{ value: 0, label: m.desired_state_present(), on: 'bg-ok-soft text-ok' },
					{ value: 1, label: m.desired_state_absent(), on: 'bg-crit-soft text-crit' }
				]
			: [{ value: 0, label: m.actions_new_state_run(), on: 'bg-accent-soft text-accent-ink' }]
	);
	const pad = $derived(size === 'sm' ? 'px-1.5 py-0.5' : 'px-2 py-1');
</script>

<div
	role="group"
	aria-label={label}
	data-testid="action-state-toggle"
	class="inline-flex shrink-0 overflow-hidden rounded-lg border font-mono text-[0.66rem]"
>
	{#each states as s (s.value)}
		<button
			type="button"
			data-state-value={s.value}
			aria-pressed={value === s.value}
			disabled={states.length === 1}
			onclick={() => (value = s.value)}
			class="{pad} tracking-[0.06em] uppercase {value === s.value
				? `${s.on} font-semibold`
				: 'text-faint hover:bg-accent/50'}"
		>
			{s.label}
		</button>
	{/each}
</div>
