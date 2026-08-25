<script lang="ts">

	import { onMount } from 'svelte';
	import { base } from '$app/paths';
	import { Check, Circle, CircleAlert } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { loadChecklist, progress, type ChecklistRow } from '$lib/onboarding/checklist';
	import { onboarding, dismissChecklist } from '$lib/onboarding/tour.svelte';

	let rows = $state<ChecklistRow[] | null>(null);

	onMount(async () => {

		try {
			rows = await loadChecklist();
		} catch (err) {
			console.error('onboarding: getting-started checklist could not be built', err);
			rows = [];
		}
	});

	const counts = $derived(rows ? progress(rows) : { done: 0, total: 0 });
	const hidden = $derived(onboarding.flags.checklistDismissed);
</script>

{#if !hidden}
	{#if rows === null}
		<p data-testid="onboarding-checklist-loading" class="mt-6 text-xs text-faint">
			{m.onboarding_checklist_loading()}
		</p>
	{:else if rows.length > 0}
		<section
			data-testid="onboarding-checklist"
			aria-label={m.onboarding_checklist_label()}
			class="mt-6 w-full max-w-md rounded-xl border bg-sunken p-4 text-left shadow-plate"
		>
			<div class="flex items-start gap-3">
				<div class="min-w-0 flex-1">
					<h4 class="text-sm font-semibold">{m.onboarding_checklist_title()}</h4>
					<p data-testid="onboarding-checklist-progress" class="mt-0.5 font-mono text-[11px] text-faint">
						{m.onboarding_checklist_progress({ done: counts.done, total: counts.total })}
					</p>
				</div>
				<button
					type="button"
					data-testid="onboarding-checklist-dismiss"
					aria-label={m.onboarding_checklist_dismiss()}
					onclick={dismissChecklist}
					class="rounded-full border px-2.5 py-1 text-[11px] text-muted-foreground hover:bg-accent/50"
				>
					✕
				</button>
			</div>

			<ul class="mt-3 space-y-1">
				{#each rows as row (row.id)}
					<li>
						<a
							href="{base}{row.href}"
							data-testid="onboarding-checklist-row"
							data-check={row.id}
							data-status={row.status}
							class="flex items-start gap-2.5 rounded-lg px-2 py-1.5 hover:bg-accent/50"
						>
							<span class="mt-0.5 shrink-0" aria-hidden="true">
								{#if row.status === 'done'}
									<Check class="h-4 w-4 text-ok" />
								{:else if row.status === 'unknown'}
									<CircleAlert class="h-4 w-4 text-faint" />
								{:else}
									<Circle class="h-4 w-4 text-faint" />
								{/if}
							</span>
							<span class="min-w-0 flex-1">
								<span class="block text-sm {row.status === 'done' ? 'text-muted-foreground line-through' : ''}">
									{row.title}
								</span>
								<span class="block text-[11px] text-faint">
									{row.status === 'unknown' ? m.onboarding_checklist_unknown_hint() : row.hint}
								</span>
							</span>
							<span class="mt-0.5 shrink-0 font-mono text-[10px] uppercase tracking-wide">
								{#if row.status === 'done'}
									<span class="rounded-full bg-ok-soft px-2 py-0.5 text-ok">{m.onboarding_checklist_done_badge()}</span>
								{:else if row.status === 'unknown'}
									<span class="rounded-full bg-sunken px-2 py-0.5 text-faint">
										{m.onboarding_checklist_unknown_badge()}
									</span>
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{/if}
