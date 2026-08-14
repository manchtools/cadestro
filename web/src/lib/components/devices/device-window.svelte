<script lang="ts">
	import { base } from '$app/paths';
	import { apiClient, formatTimestampDateTime, type Device } from '$lib/sdk';
	import { DeviceStatus } from '$contract/cadestro/v1/common_pb';
	import { getLocalizedError } from '$lib/errors';
	import { openTerminal } from '$lib/shell/shell.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Chip, type FleetTone } from '$lib/components/fleet';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { ExternalLink, RotateCw, SquareTerminal } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	let { deviceId }: { deviceId: string } = $props();

	let device: Device | null = $state(null);
	let loading = $state(true);
	let error = $state('');
	let request = 0;

	function statusLabel(status: DeviceStatus): string {
		if (status === DeviceStatus.ONLINE) return m.devices_status_online();
		if (status === DeviceStatus.OFFLINE) return m.devices_status_offline();
		return m.common_unknown();
	}

	// Connectivity is status, so it rides a toned chip — an outline badge said
	// "online" and "offline" in exactly the same ink.
	function statusTone(status: DeviceStatus): FleetTone {
		if (status === DeviceStatus.ONLINE) return 'ok';
		if (status === DeviceStatus.OFFLINE) return 'crit';
		return 'idle';
	}

	async function load() {
		const current = ++request;
		loading = true;
		error = '';
		try {
			const loaded = await apiClient.getDevice(deviceId);
			if (current === request) device = loaded ?? null;
		} catch (cause) {
			if (current === request) error = getLocalizedError(cause);
		} finally {
			if (current === request) loading = false;
		}
	}

	function openDeviceTerminal() {
		if (device) openTerminal(device.id, device.hostname);
	}

	$effect(() => {
		void deviceId;
		void load();
	});
</script>

{#if loading}
	<div class="space-y-3" aria-label={m.common_loading()}>
		<Skeleton class="h-5 w-40" />
		<Skeleton class="h-4 w-full" />
		<Skeleton class="h-4 w-3/4" />
	</div>
{:else if error || !device}
	<div class="space-y-3 text-sm text-muted-foreground">
		<p>{error || m.common_unknown()}</p>
		<Button size="sm" variant="outline" onclick={load}>
			<RotateCw class="h-3.5 w-3.5" /> {m.common_refresh()}
		</Button>
	</div>
{:else}
	<div class="space-y-4 text-sm">
		<div class="flex items-center justify-between gap-2">
			<Chip tone={statusTone(device.status)} label={statusLabel(device.status)} />
			<span class="font-mono text-xs text-muted-foreground">{device.agentVersion}</span>
		</div>
		<dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2">
			<dt class="text-muted-foreground">{m.devices_table_last_seen()}</dt>
			<dd class="text-right">{formatTimestampDateTime(device.lastSeenAt)}</dd>
			<dt class="text-muted-foreground">{m.devices_table_labels()}</dt>
			<dd class="text-right">{Object.keys(device.labels).length}</dd>
		</dl>
		<div class="flex gap-2 border-t pt-3">
			<Button size="sm" variant="outline" href={`${base}/devices/${device.id}`}>
				<ExternalLink class="h-3.5 w-3.5" /> {m.common_details()}
			</Button>
			<Button size="sm" onclick={openDeviceTerminal}>
				<SquareTerminal class="h-3.5 w-3.5" /> {m.terminal_open()}
			</Button>
		</div>
	</div>
{/if}
