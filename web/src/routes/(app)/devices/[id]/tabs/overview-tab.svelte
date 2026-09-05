<script lang="ts">
 import { goto } from '$lib/navigation';
 import { toast } from 'svelte-sonner';
 import { api } from '$lib/api';
 import { Permission, type Device } from '$contract/cadestro/v1/control_pb';
 import { DeviceStatus } from '$contract/cadestro/v1/common_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { formatDate as formatTimestampDateTime } from '$lib/console';
 import { getLocalizedError } from '$lib/errors';
 import { Button } from '$lib/components/ui/button';
 import { Label } from '$lib/components/ui/label';
 import { Chip, type FleetTone } from '$lib/components/fleet';
 import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
 import { Trash2 } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
 const { can } = consoleContext();
 let { device, deviceId }: { device: Device; deviceId: string } = $props();
 let deleteDialogOpen = $state(false);
 function getStatusTone(status: DeviceStatus): FleetTone { return status === DeviceStatus.ONLINE ? 'ok' : status === DeviceStatus.OFFLINE ? 'crit' : 'idle'; }
 function getStatusLabel(status: DeviceStatus) { return status === DeviceStatus.ONLINE ? m.devices_status_online() : m.devices_status_offline(); }
 async function deleteDevice() { if (!can(Permission.DELETE_DEVICE)) return; try { await api.deleteDevice({ id: { value: deviceId } }); await goto('/devices'); } catch(error) { toast.error(getLocalizedError(error)); } }
</script>
{#snippet sectionLabel(text: string)}
	<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{text}</span>
{/snippet}

<div class="space-y-6">

	<div class="grid gap-6 md:grid-cols-2">
		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
			{@render sectionLabel(m.device_detail_title())}
			<div class="mt-3 space-y-4">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<Label class="text-muted-foreground">{m.common_status()}</Label>
						<div class="mt-1">
							<Chip tone={getStatusTone(device.status)} label={getStatusLabel(device.status)} />
						</div>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.devices_table_agent_version()}</Label>
						<p class="mt-1 font-medium">{device.agentVersion}</p>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.device_detail_registered()}</Label>
						<p class="mt-1 text-sm">{formatTimestampDateTime(device.registeredAt)}</p>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.device_detail_last_seen()}</Label>
						<p class="mt-1 text-sm">{formatTimestampDateTime(device.lastSeenAt)}</p>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.device_detail_cert_expires()}</Label>
						<p class="mt-1 text-sm">{formatTimestampDateTime(device.certExpiresAt)}</p>
					</div>
                </div>
            </div>
        </section>
    </div>
    <div class="flex gap-3">
    {#if can(Permission.CREATE_ASSIGNMENT)}<Button variant="outline" href={`/assignments?devices=${deviceId}`}>Assign actions</Button>{/if}
    {#if can(Permission.DELETE_DEVICE)}<Button variant="destructive" onclick={() => { deleteDialogOpen = true; }}><Trash2 class="mr-2 h-4 w-4" />{m.common_delete()}</Button>{/if}
    </div>
</div>
<ConfirmDeleteDialog bind:open={deleteDialogOpen} title={m.common_delete()} description={`Delete ${device.hostname}?`} onconfirm={deleteDevice} />
