<script lang="ts">
 import type { ManagedAction } from '$contract/cadestro/v1/control_pb';
 import { actionChoice, getActionTypeInfoByValue } from '$lib/components/actions/action-type';
 let { actions, loading = false, selectedId, onselect }: { actions: ManagedAction[]; loading?: boolean; selectedId: string | null; onselect: (id: string) => void } = $props();
</script>
<div class="flex min-w-0 flex-col gap-2 border-l border-border bg-surface p-4">
	<div class="font-mono text-[0.66rem] uppercase tracking-[0.08em] text-faint">
		Actions
	</div>

	{#if loading}
		<p class="text-xs text-muted-foreground">Loading actions…</p>
	{:else if !actions.length}
		<p class="text-xs text-muted-foreground">No actions available.</p>
	{:else}
		<div data-tour="assign-sets" role="radiogroup" aria-label=Actions class="grid gap-1.5">
			{#each actions as action (action.id?.value ?? '')}
				{@const on = (action.id?.value ?? '') === selectedId}
				<div class="overflow-hidden rounded-[9px] border {on ? 'border-primary' : 'border-border'}">
					<button
						type="button"
						role="radio"
						aria-checked={on}
						onclick={() => onselect((action.id?.value ?? ''))}
						class="flex w-full items-center gap-2 px-2 py-2 text-left text-sm {on ? 'bg-accent-soft' : ''}"
					>
						<span
							class="h-3.5 w-3.5 shrink-0 rounded-full border-[1.5px] {on
								? 'border-[0.28rem] border-primary'
								: 'border-border-strong'}"
						></span>
						<span class="min-w-0 truncate font-semibold">{action.name}</span>
						<span class="ml-auto shrink-0 font-mono text-[0.66rem] text-faint">
							{getActionTypeInfoByValue(actionChoice(action)).label}
						</span>
					</button>
                    {#if on}<p class="bg-surface px-2 pb-2 pl-8 text-xs text-muted-foreground">{action.description}</p>{/if}
				</div>
			{/each}
		</div>
	{/if}

</div>
