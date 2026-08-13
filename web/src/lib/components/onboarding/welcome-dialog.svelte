<script lang="ts">
	// First-run welcome. Two exits, both final: take the tour, or explore alone.
	// It is a courtesy, never a gate — Esc and a backdrop click dismiss it, and
	// the app behind it has already finished loading.
	import * as m from '$lib/paraglide/messages';
	import { cycleTab } from '$lib/onboarding/focus';
	import { motion } from '$lib/onboarding/motion';

	let { onstart, ondismiss }: { onstart: () => void; ondismiss: () => void } = $props();

	let card = $state<HTMLElement | undefined>();
	const reduced = motion.reduced();

	// Focus lands on the dialog itself, so a screen reader reads the title and
	// body before the operator tabs into the two choices.
	$effect(() => {
		card?.focus();
	});

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			ondismiss();
			return;
		}
		if (card && cycleTab(e, card)) e.preventDefault();
	}
</script>

<div
	class="fixed inset-0 z-[70] grid place-items-center bg-black/45 p-4"
	data-testid="onboarding-welcome-backdrop"
	role="presentation"
	onclick={(e) => e.target === e.currentTarget && ondismiss()}
>
	<div
		bind:this={card}
		role="dialog"
		aria-modal="true"
		aria-labelledby="onboarding-welcome-title"
		aria-describedby="onboarding-welcome-body"
		tabindex="-1"
		data-testid="onboarding-welcome"
		data-motion={reduced ? 'reduced' : 'full'}
		{onkeydown}
		class="w-full max-w-lg rounded-[14px] border bg-surface p-6 text-foreground shadow-pill outline-none"
	>
		<h2 id="onboarding-welcome-title" class="text-lg font-semibold tracking-tight">
			{m.onboarding_welcome_title()}
		</h2>
		<p id="onboarding-welcome-body" class="mt-2 text-sm text-muted-foreground">
			{m.onboarding_welcome_body()}
		</p>
		<p class="mt-2 text-sm text-muted-foreground">{m.onboarding_welcome_lead()}</p>

		<div class="mt-5 flex flex-wrap items-center gap-2">
			<button
				type="button"
				data-testid="onboarding-welcome-start"
				onclick={onstart}
				class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90"
			>
				{m.onboarding_welcome_take_tour()}
			</button>
			<button
				type="button"
				data-testid="onboarding-welcome-dismiss"
				onclick={ondismiss}
				class="rounded-full border px-4 py-2 text-sm text-muted-foreground hover:bg-accent/50"
			>
				{m.onboarding_welcome_explore()}
			</button>
		</div>

		<p class="mt-4 border-t border-hair pt-3 text-[11px] text-faint">
			{m.onboarding_welcome_restart_note()}
		</p>
	</div>
</div>
