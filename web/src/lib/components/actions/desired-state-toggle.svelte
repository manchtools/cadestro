<script lang="ts">

	import { DesiredState } from '$contract/cadestro/v1/common_pb';
	import * as m from '$lib/paraglide/messages';

	let {
		value = $bindable(),
		supportsAbsent = true,
		size = 'md',
		label = m.action_detail_desired_state()
	}: {

		value: number;

		supportsAbsent?: boolean;
		size?: 'sm' | 'md';
		label?: string;
	} = $props();

	const states = $derived(
		supportsAbsent
			? [
					{ value: DesiredState.PRESENT, label: m.desired_state_present(), on: 'bg-ok-soft text-ok' },
					{ value: DesiredState.ABSENT, label: m.desired_state_absent(), on: 'bg-crit-soft text-crit' }
				]
			: [{ value: DesiredState.PRESENT, label: m.actions_new_state_run(), on: 'bg-accent-soft text-accent-ink' }]
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
