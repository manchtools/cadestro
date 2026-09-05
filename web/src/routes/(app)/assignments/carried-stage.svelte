<script lang="ts">
 import { Tile, type FleetTone } from '$lib/components/fleet';
 import * as m from '$lib/paraglide/messages';
 let { label, devices, loading = false, failures = [] }: { label: string; devices: { id: string; hostname: string; tone: FleetTone }[]; loading?: boolean; failures?: { targetId: string; error: string }[] } = $props();
</script>
<div class="flex min-w-0 flex-col gap-3 bg-sunken p-4">
	<div class="flex items-baseline justify-between gap-2 font-mono text-[0.66rem] uppercase tracking-[0.08em] text-faint">
		<span>{m.assign_stage_label()}</span>
		<span>{label}</span>
	</div>

	<div
		data-tour="assign-carried"
		data-testid="assign-carried-grid"
		class="grid grid-cols-[repeat(auto-fill,minmax(1rem,1fr))] gap-[0.22rem]"
	>
		{#each devices as device (device.id)}
			<Tile tone={device.tone} label={device.hostname || device.id} />
		{/each}
	</div>

	{#if loading}
		<p class="text-xs text-muted-foreground">{m.assign_stage_loading()}</p>
	{/if}

 <p class="border-t pt-3 text-xs text-muted-foreground">Assigned actions are delivered during the next desired policy sync.</p>
	{#if failures.length}

		<div data-testid="assign-failures" class="grid gap-1 rounded-lg border border-crit/40 bg-crit-soft p-2 text-xs text-crit">
			<span class="font-semibold">{m.assign_failures_label()}</span>
			{#each failures as failure (failure.targetId)}
				<span class="font-mono">{failure.targetId} · {failure.error}</span>
			{/each}
		</div>
	{/if}
</div>
