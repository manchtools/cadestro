<script lang="ts">

	import { onMount } from 'svelte';
	import { RefreshCw } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { goto } from '$app/navigation';
 import { api } from '$lib/api';
 import { toast } from 'svelte-sonner';
 import { openPanel } from '$lib/shell/shell.svelte';
	import FleetSurface from './fleet-surface.svelte';
	import { loadFleet, type FleetSnapshot } from './fleet-data';

	import { Permission } from '$contract/cadestro/v1/control_pb';
	import { consoleContext } from '$lib/console-context.svelte';
	const { can } = consoleContext();
	let snapshot = $state<FleetSnapshot | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let nowMs = $state(Date.now());

	async function refresh() {
		loading = true;
		error = null;
		try {
			const next = await loadFleet(can);
			nowMs = Date.now();
			snapshot = next;
		} catch (err) {
			error = getLocalizedError(err);
			console.error(err);
		} finally {
			loading = false;
		}
	}

async function deleteDevices(ids: string[]) {
        try { for (const id of ids) await api.deleteDevice({ id: { value: id } }); }
        catch (cause) { toast.error(getLocalizedError(cause)); }
        finally { await refresh(); }
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
	onOpenDevice={(d) => { if (can(Permission.GET_DEVICE)) openPanel('device', d.id, d.hostname); else void goto(`/devices/${d.id}`); }}
    onDeleteDevices={deleteDevices}
>
	{#snippet headerExtra()}
		<Button onclick={refresh} variant="outline" disabled={loading}>
			<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
				<RefreshCw class="h-4 w-4" />
			</span>
			{m.common_refresh()}
		</Button>
	{/snippet}

</FleetSurface>
