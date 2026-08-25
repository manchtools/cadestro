<script lang="ts">
	// Device zoom for the admin fleet. The existing server-search machinery backs
	// this level unchanged — same DEVICES scope, same sort / status-filter / page
	// URL params, same row capabilities (open window, assign, delete) — worn
	// under the fleet skin: every row leads with the same status tile the far
	// pane draws, and clicking it selects into the same fleet selection.
	//
	// Mounted only while zoom === 'device', so the fleet and group levels never
	// fire a search RPC they don't render.
	import { base } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { apiClient, type Device, formatTimestampDateTime } from '$lib/sdk';
	import { SearchScope, SortField } from '$contract/cadestro/v1/common_pb';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import AssignDeviceDialog from './[id]/assign-device-dialog.svelte';
	import { Monitor, MoreHorizontal, Trash2, AppWindow, UserPlus } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { codecs } from '$lib/url-state';
	import { searchResultToDevice } from '$lib/search-adapters';
	import { DataTable, DataTablePagination, createSearchListState } from '$lib/components/data-table';
	import { openPanel } from '$lib/shell/shell.svelte';
	import { Tile, Chip, type FleetTone } from '$lib/components/fleet';
	import { toFleetDevice, type FleetDevice } from './fleet-model';
	import { getFleetSelection, setFleetSelection } from './fleet-selection.svelte';

	let { surfaceId, nowMs = Date.now() }: { surfaceId: string; nowMs?: number } = $props();

	// The list is a scoped PostgreSQL search query. Online/offline is a
	// query-time filter derived from the server's five-minute heartbeat window.
	type SortKey = 'hostname' | 'status' | 'lastSeen';

	const table = createSearchListState<Device, SortKey, { status: string[] }>({
		scope: SearchScope.DEVICES,
		adapter: searchResultToDevice,
		sortKeys: ['hostname', 'status', 'lastSeen'],
		defaultSort: 'hostname',
		// Compliance status is sortable; connectivity remains a last-seen filter.
		sortFieldMap: {
			hostname: SortField.HOSTNAME,
			status: SortField.COMPLIANCE_STATUS,
			lastSeen: SortField.LAST_SEEN_AT
		},
		// Timestamps read newest-first when you switch to them.
		sortDir: (key) => (key === 'lastSeen' ? 'desc' : 'asc'),
		filters: { status: { key: 'status', codec: codecs.stringArray([]) } },
		// The server takes one connectivity value. Selecting both means no filter.
		filterToTags: (f) => (f.status.length === 1 ? { status: f.status[0] } : undefined)
	});

	// The query lives in the pill now: ⌘K opens search already on the Devices
	// facet and its keystrokes land on the same setSearch the removed input
	// drove. This level mounts only at zoom === 'device', so the registration
	// appears and is withdrawn with the list it belongs to.
	$effect(() =>
		registerPageSearch({
			scope: SearchScope.DEVICES,
			label: m.nav_devices,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);

	let deleteDialogOpen = $state(false);
	let deviceToDelete = $state<Device | null>(null);
	let assignDialogOpen = $state(false);
	let deviceToAssign = $state<Device | null>(null);

	const statusFilterItems = [
		{ id: 'online', label: m.devices_status_online() },
		{ id: 'offline', label: m.devices_status_offline() }
	];

	// The same four buckets the near pane labels, in the same words. A row's
	// chip must name the bucket its tile was painted from: collapsing the tone
	// to "online or offline" made a drifting device read as offline and a
	// never-seen device read as offline too — two states the fleet model keeps
	// deliberately apart.
	const STATUS_LABEL: Record<FleetTone, () => string> = {
		ok: m.devices_status_online,
		warn: m.fleet_status_drift,
		crit: m.devices_status_offline,
		info: m.fleet_tile_info,
		idle: m.fleet_tile_idle
	};

	const columns = [
		{ id: 'select', label: '', class: 'w-8' },
		{ id: 'hostname', label: m.devices_table_hostname(), sortKey: 'hostname' as const },
		{ id: 'status', label: m.devices_table_status(), sortKey: 'status' as const },
		{ id: 'agentVersion', label: m.devices_table_agent_version() },
		{ id: 'lastSeen', label: m.devices_table_last_seen(), sortKey: 'lastSeen' as const },
		{ id: 'lastInventory', label: m.devices_table_last_inventory() },
		{ id: 'labels', label: m.devices_table_labels() },
		{ id: 'actions', label: '', class: 'w-12' }
	];

	const nowSec = $derived(Math.floor(nowMs / 1000));
	// The search document carries no sync_interval_minutes, so these tiles decay
	// against the server default — the group-resolved cadence only exists where
	// the snapshot does (fleet / group zoom).
	const fleetRows = $derived<FleetDevice[]>(table.rows.map((d) => toFleetDevice(d, [], nowSec)));
	const rowAt = $derived(new Map(fleetRows.map((d, i) => [d.id, i])));
	const selectedIds = $derived(getFleetSelection(surfaceId));
	const selected = $derived(new Set(selectedIds));

	let anchor = $state<number | null>(null);
	let shiftHeld = $state(false);

	function toggle(index: number, shift: boolean) {
		const target = fleetRows[index];
		if (!target) return;
		if (shift && anchor !== null) {
			const from = Math.min(anchor, index);
			const to = Math.max(anchor, index);
			const add = fleetRows.slice(from, to + 1).map((d) => d.id);
			setFleetSelection(surfaceId, [...new Set([...selectedIds, ...add])]);
			return;
		}
		anchor = index;
		setFleetSelection(
			surfaceId,
			selected.has(target.id)
				? selectedIds.filter((id) => id !== target.id)
				: [...selectedIds, target.id]
		);
	}

	function confirmDelete(device: Device) {
		deviceToDelete = device;
		deleteDialogOpen = true;
	}

	async function deleteDevice() {
		if (!deviceToDelete) return;
		try {
			await apiClient.deleteDevice((deviceToDelete.id?.value ?? ''));
			toast.success(m.devices_deleted());
			table.patchRows((list) => list.filter((d) => d.id !== deviceToDelete!.id));
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			deviceToDelete = null;
		}
	}

	function openAssignDialog(device: Device) {
		deviceToAssign = device;
		assignDialogOpen = true;
	}

	async function assignDevice(userIds: string[], groupIds: string[]) {
		if (!deviceToAssign) return;
		try {
			const updated = await apiClient.assignDevice((deviceToAssign.id?.value ?? ''), userIds, groupIds);
			if (updated) {
				table.patchRows((list) => list.map((d) => (d.id === deviceToAssign!.id ? updated : d)));
			}
			toast.success(m.devices_assigned());
			assignDialogOpen = false;
			deviceToAssign = null;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}
</script>

<!-- ONE toolbar line for the device zoom. The search box is gone — ⌘K is the
     search, already scoped to Devices — so only the status filter remains, and
     it stays with the list it filters: this level is mounted only while
     zoom === 'device', and the fleet header above outlives it. -->
<div class="flex flex-wrap items-center justify-end gap-2">
	<MultiSelectCombobox
		items={statusFilterItems}
		selected={table.filters.status}
		onSelectedChange={(next) => table.setFilter('status', next)}
		placeholder={m.devices_filter_all_statuses()}
		searchPlaceholder={m.common_search()}
		class="w-44"
	/>
</div>

<div
	class="space-y-3"
	onpointerdowncapture={(e) => (shiftHeld = e.shiftKey)}
	onkeydowncapture={(e) => (shiftHeld = e.shiftKey)}
>
	<DataTable {table} {columns} rowKey={(d) => (d.id?.value ?? '')}>
		{#snippet row(device)}
			{@const index = rowAt.get((device.id?.value ?? '')) ?? -1}
			{@const fleet = index >= 0 ? fleetRows[index] : undefined}
			<Table.Cell>
				{#if fleet}
					<span class="block w-3.5">
						<Tile
							tone={fleet.tone}
							age={fleet.age}
							label={fleet.hostname}
							selected={selected.has(fleet.id)}
							onclick={() => toggle(index, shiftHeld)}
						/>
					</span>
				{/if}
			</Table.Cell>
			<Table.Cell>
				<a href="{base}/devices/{device.id}" class="font-mono font-medium hover:underline">
					{device.hostname}
				</a>
				<div class="font-mono text-xs text-faint">{device.id}</div>
			</Table.Cell>
			<Table.Cell>
				{#if fleet}
					<span data-testid="device-status" data-tone={fleet.tone}>
						<Chip tone={fleet.tone} label={STATUS_LABEL[fleet.tone]()} />
					</span>
				{/if}
			</Table.Cell>
			<Table.Cell>{device.agentVersion}</Table.Cell>
			<Table.Cell class="text-sm">
				{formatTimestampDateTime(device.lastSeenAt)}
			</Table.Cell>
			<Table.Cell class="text-sm">
				<div class="flex items-center gap-2">
					{device.lastInventoryAt
						? formatTimestampDateTime(device.lastInventoryAt)
						: m.device_detail_inventory_never()}
					{#if device.inventoryOverdue}
						<Chip tone="warn" label={m.inventory_overdue_badge()} />
					{/if}
				</div>
			</Table.Cell>
			<Table.Cell>
				<div class="flex flex-wrap gap-1">
					{#each Object.entries(device.labels) as [key, value] (key)}
						<Badge variant="outline" class="text-xs">
							{key}: {value}
						</Badge>
					{/each}
				</div>
			</Table.Cell>
			<Table.Cell>
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
								<MoreHorizontal class="h-4 w-4" />
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end">
						<DropdownMenu.Item onclick={() => openPanel('device', (device.id?.value ?? ''), device.hostname)}>
							<AppWindow class="mr-2 h-4 w-4" />
							{m.common_open_window()}
						</DropdownMenu.Item>
						<DropdownMenu.Item onclick={() => openAssignDialog(device)}>
							<UserPlus class="mr-2 h-4 w-4" />
							{m.common_assign()}
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item onclick={() => confirmDelete(device)} class="text-destructive">
							<Trash2 class="mr-2 h-4 w-4" />
							{m.common_delete()}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</Table.Cell>
		{/snippet}

		{#snippet empty()}
			<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
				<Monitor class="mb-4 h-12 w-12 text-muted-foreground" />
				<h3 class="font-semibold">{m.devices_empty()}</h3>
				<p class="text-muted-foreground">
					{table.query || table.filters.status.length > 0
						? m.common_try_different_search()
						: m.devices_empty_hint()}
				</p>
			</Card.Content>
		{/snippet}
	</DataTable>

	<DataTablePagination {table} />
</div>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.devices_delete_dialog_title()}
	description={m.devices_delete_dialog_description({ hostname: deviceToDelete?.hostname ?? '' })}
	onconfirm={deleteDevice}
/>

<AssignDeviceDialog
	bind:open={assignDialogOpen}
	hostname={deviceToAssign?.hostname ?? ''}
	onassign={assignDevice}
/>
