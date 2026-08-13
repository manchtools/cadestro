<script lang="ts">
	// B2's left stage: the carried devices as a mini tile grid, their hostname
	// breakdown, and the eligibility rollup. Every number comes from the
	// Eligibility map the page derives once — the stage, the pill caption and
	// the commit all read the same object, so they cannot disagree.
	import { Tile } from '$lib/components/fleet';
	import * as m from '$lib/paraglide/messages';
	import { statusTone, type CarriedDevice, type Eligibility, type HostnameGroup } from './eligibility';
	import type { AssignOutcome } from './assign-data';

	let {
		label,
		devices,
		loading = false,
		groups,
		eligibility,
		failures = []
	}: {
		label: string;
		devices: CarriedDevice[];
		loading?: boolean;
		groups: HostnameGroup[];
		eligibility: Eligibility;
		failures?: AssignOutcome[];
	} = $props();
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
			<Tile tone={statusTone(device.status)} label={device.hostname || device.id} />
		{/each}
	</div>

	{#if loading}
		<p class="text-xs text-muted-foreground">{m.assign_stage_loading()}</p>
	{/if}

	{#if groups.length}
		<div class="grid gap-0.5 font-mono text-xs text-muted-foreground">
			{#each groups as group (group.name)}
				<span><span class="text-foreground">{group.name}</span> · {group.count}</span>
			{/each}
		</div>
	{/if}

	<div
		data-tour="assign-eligibility"
		data-testid="assign-eligibility"
		class="grid gap-1.5 border-t border-border pt-3 text-sm text-muted-foreground"
	>
		<span class="flex items-center gap-2">
			<span aria-hidden="true" class="h-2 w-2 shrink-0 rounded-full bg-ok"></span>
			{m.assign_eligibility_ready({ count: eligibility.ready })}
		</span>
		<span class="flex items-center gap-2">
			<span aria-hidden="true" class="h-2 w-2 shrink-0 rounded-full bg-warn"></span>
			{m.assign_eligibility_update({ count: eligibility.update })}
		</span>
		<span class="flex items-center gap-2">
			<span aria-hidden="true" class="h-2 w-2 shrink-0 rounded-full bg-crit"></span>
			{m.assign_eligibility_queued({ count: eligibility.queued })}
		</span>
		{#if eligibility.unknown > 0}
			<span class="flex items-center gap-2">
				<span aria-hidden="true" class="h-2 w-2 shrink-0 rounded-full bg-idle"></span>
				{m.assign_eligibility_unknown({ count: eligibility.unknown })}
			</span>
		{/if}
	</div>

	{#if failures.length}
		<!-- A partial commit names the devices that failed; it never rounds them
		     into a single "something went wrong". -->
		<div data-testid="assign-failures" class="grid gap-1 rounded-lg border border-crit/40 bg-crit-soft p-2 text-xs text-crit">
			<span class="font-semibold">{m.assign_failures_label()}</span>
			{#each failures as failure (failure.deviceId)}
				<span class="font-mono">{failure.deviceId} · {failure.error}</span>
			{/each}
		</div>
	{/if}
</div>
