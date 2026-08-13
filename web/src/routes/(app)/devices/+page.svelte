<script lang="ts">
	// The devices route IS the fleet surface (concept A4 / round-2 movement A):
	// a semantic zoom over one ListDevices + ListDeviceGroups snapshot, with the
	// existing server-search list kept as the device zoom level.
	import { onMount } from 'svelte';
	import { RefreshCw } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { openPanel } from '$lib/shell/shell.svelte';
	import GettingStartedChecklist from '$lib/components/onboarding/getting-started-checklist.svelte';
	import FleetSurface from './fleet-surface.svelte';
	import DeviceLevel from './device-level.svelte';
	import { loadFleet, type FleetSnapshot } from './fleet-data';

	let snapshot = $state<FleetSnapshot | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	// Captured with the snapshot so every tile's decay is measured against the
	// same instant the data was read at.
	let nowMs = $state(Date.now());

	async function refresh() {
		loading = true;
		error = null;
		try {
			const next = await loadFleet();
			nowMs = Date.now();
			snapshot = next;
		} catch (err) {
			error = getLocalizedError(err);
			console.error(err);
		} finally {
			loading = false;
		}
	}

	onMount(refresh);
</script>

<FleetSurface
	surfaceId="devices"
	heading={(count) => m.fleet_heading({ count })}
	{snapshot}
	{loading}
	{error}
	{nowMs}
	emptyTitle={m.fleet_empty_title()}
	emptyHint={m.fleet_empty_hint()}
	openLabel={m.common_open_window()}
	onOpenDevice={(d) => openPanel('device', d.id, d.hostname)}
>
	{#snippet headerExtra()}
		<Button onclick={refresh} variant="outline" disabled={loading}>
			<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
				<RefreshCw class="h-4 w-4" />
			</span>
			{m.common_refresh()}
		</Button>
	{/snippet}

	{#snippet deviceLevel()}
		<DeviceLevel surfaceId="devices" {nowMs} />
	{/snippet}

	<!-- The getting-started checklist belongs to the EMPTY fleet and to nowhere
	     else, so it rides fleet-empty's own `extra` slot. -->
	{#snippet emptyExtra()}
		<GettingStartedChecklist />
	{/snippet}
</FleetSurface>
