<script lang="ts">

	import { base } from '$app/paths';
	import { Tile, Chip } from '$lib/components/fleet';
	import { Button } from '$lib/components/ui/button';
	import { DeviceStatus } from '$contract/cadestro/v1/common_pb';
 import { Permission, type Device } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 const { can } = consoleContext();
	import type { FleetTone } from '$lib/components/fleet';
	import { Plus, Trash2 } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	export interface MemberRow {
		deviceId: string;
		hostname: string;
		agentVersion: string;
	}

	interface Props {
		members: MemberRow[];

		devices: Map<string, Device>;
		canAdd: boolean;
		onadd: () => void;
		onremove: (deviceId: string) => void;
	}

	let { members, devices, canAdd, onadd, onremove }: Props = $props();

	function tone(deviceId: string): FleetTone {
		const status = devices.get(deviceId)?.status;
		if (status === DeviceStatus.ONLINE) return 'ok';
		if (status === DeviceStatus.OFFLINE) return 'crit';
		return 'idle';
	}

	function attributes(row: MemberRow): string[] { return row.agentVersion ? [row.agentVersion] : []; }
</script>

<div class="rounded-xl border border-hair bg-surface shadow-plate" data-tour="group-members" data-testid="members-tab">
	<div class="flex items-center justify-between border-b px-3 py-2">
		<span class="font-mono text-[0.62rem] tracking-[0.08em] text-faint uppercase">
			{m.device_group_detail_devices()}
		</span>

			<Button size="sm" variant="outline" onclick={onadd} disabled={!canAdd}>
				<Plus class="mr-1 h-3.5 w-3.5" />
				{m.common_add()}
			</Button>

	</div>

	{#if members.length === 0}
		<p class="px-3 py-8 text-center text-sm text-muted-foreground">
			{m.device_group_detail_no_devices_static()}
		</p>
	{:else}
		<div class="max-h-[28rem] overflow-y-auto">
			{#each members as row (row.deviceId)}
				<div class="flex items-center gap-2.5 border-b border-hair px-3 py-1.5 last:border-b-0">
					<span class="w-3.5 shrink-0"><Tile tone={tone(row.deviceId)} label={row.hostname} /></span>
					<a aria-disabled={!can(Permission.GET_DEVICE)} href={can(Permission.GET_DEVICE) ? `${base}/devices/${row.deviceId}` : undefined} class="truncate font-mono text-[0.82rem] hover:underline">
						{row.hostname}
					</a>
					<span class="hidden truncate font-mono text-[0.68rem] text-faint sm:inline">
						{row.deviceId}
					</span>
					<span class="ml-auto flex shrink-0 flex-wrap items-center gap-1">
						{#each attributes(row) as attribute (attribute)}
							<Chip tone="idle" label={attribute} />
						{/each}

					</span>
					{#if can(Permission.REMOVE_DEVICE_FROM_GROUP)}
						<Button
							variant="ghost"
							size="icon-sm"
							class="shrink-0 text-muted-foreground hover:text-destructive"
							aria-label={m.device_groups_remove_member()}
							onclick={() => onremove(row.deviceId)}
						>
							<Trash2 class="h-3.5 w-3.5" />
						</Button>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
