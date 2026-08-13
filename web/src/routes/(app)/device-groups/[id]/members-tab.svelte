<script lang="ts">
	// Dense member rows: a status tile, the hostname in mono, the ULID muted,
	// and the real inventory attributes we already hold. A dynamic group's
	// membership is the rule's output, so it carries no add/remove affordance.
	import { base } from '$app/paths';
	import { Tile, Chip } from '$lib/components/fleet';
	import { Button } from '$lib/components/ui/button';
	import { DeviceStatus, type Device } from '$lib/sdk';
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
		/** Full device reads, keyed by id — status and labels come from here. */
		devices: Map<string, Device>;
		isDynamic: boolean;
		canAdd: boolean;
		onadd: () => void;
		onremove: (deviceId: string) => void;
	}

	let { members, devices, isDynamic, canAdd, onadd, onremove }: Props = $props();

	function tone(deviceId: string): FleetTone {
		const status = devices.get(deviceId)?.status;
		if (status === DeviceStatus.ONLINE) return 'ok';
		if (status === DeviceStatus.OFFLINE) return 'crit';
		return 'idle';
	}

	/** Labels shown inline before the row runs out of width. */
	const LABEL_CAP = 3;

	function labelsOf(row: MemberRow): string[] {
		return Object.entries(devices.get(row.deviceId)?.labels ?? {}).map(([k, v]) => `${k}=${v}`);
	}

	function attributes(row: MemberRow): string[] {
		const labels = labelsOf(row).slice(0, LABEL_CAP);
		return row.agentVersion ? [...labels, row.agentVersion] : labels;
	}

	/** How many labels the cap left off — silently dropping them would make a
	 *  three-label device and a nine-label device read identically. */
	function hiddenLabels(row: MemberRow): number {
		return Math.max(0, labelsOf(row).length - LABEL_CAP);
	}
</script>

<div class="rounded-xl border border-hair bg-surface shadow-plate" data-tour="group-members" data-testid="members-tab">
	<div class="flex items-center justify-between border-b px-3 py-2">
		<span class="font-mono text-[0.62rem] tracking-[0.08em] text-faint uppercase">
			{m.device_group_detail_devices()}
		</span>
		{#if isDynamic}
			<Chip tone="info" label={m.device_group_detail_auto_managed()} />
		{:else}
			<Button size="sm" variant="outline" onclick={onadd} disabled={!canAdd}>
				<Plus class="mr-1 h-3.5 w-3.5" />
				{m.common_add()}
			</Button>
		{/if}
	</div>

	{#if members.length === 0}
		<p class="px-3 py-8 text-center text-sm text-muted-foreground">
			{isDynamic
				? m.device_group_detail_no_devices_dynamic()
				: m.device_group_detail_no_devices_static()}
		</p>
	{:else}
		<div class="max-h-[28rem] overflow-y-auto">
			{#each members as row (row.deviceId)}
				<div class="flex items-center gap-2.5 border-b border-hair px-3 py-1.5 last:border-b-0">
					<span class="w-3.5 shrink-0"><Tile tone={tone(row.deviceId)} label={row.hostname} /></span>
					<a href="{base}/devices/{row.deviceId}" class="truncate font-mono text-[0.82rem] hover:underline">
						{row.hostname}
					</a>
					<span class="hidden truncate font-mono text-[0.68rem] text-faint sm:inline">
						{row.deviceId}
					</span>
					<span class="ml-auto flex shrink-0 flex-wrap items-center gap-1">
						{#each attributes(row) as attribute (attribute)}
							<Chip tone="idle" label={attribute} />
						{/each}
						{#if hiddenLabels(row) > 0}
							<span class="font-mono text-[0.68rem] text-faint">
								{m.device_labels_more({ count: hiddenLabels(row) })}
							</span>
						{/if}
					</span>
					{#if !isDynamic}
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
