<script lang="ts">
	// Self-service view: the SAME fleet surface, constrained to the caller's own
	// devices (ListDevices with my_devices_only). The available-actions drill-in
	// is unchanged — it just replaces the surface while a device is open, so no
	// self-service capability was traded for the new skin.
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { apiClient, type AvailableItem } from '$lib/sdk';
	import { AssignmentSourceType } from '$sdk/powermanage/v1/common_pb';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import PageShell from '$lib/components/page-shell.svelte';
	import { Chip } from '$lib/components/fleet';
	import { Monitor, ArrowLeft, RefreshCw, Package, Check, Download } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import FleetSurface from '../devices/fleet-surface.svelte';
	import { loadFleet, type FleetSnapshot } from '../devices/fleet-data';
	import { deviceTone, type FleetDevice } from '../devices/fleet-model';

	let snapshot = $state<FleetSnapshot | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let nowMs = $state(Date.now());
	let query = $state('');

	// Device detail state
	let selectedDevice = $state<FleetDevice | null>(null);
	let availableItems = $state<AvailableItem[]>([]);
	let loadingItems = $state(false);
	let togglingIds = $state<Set<string>>(new Set());

	async function refresh() {
		loading = true;
		error = null;
		try {
			const next = await loadFleet({ myDevicesOnly: true });
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

	// The self-service search stays client-side (ListDevices, not the search
	// index). Narrowing rewrites `total` too, so heading, stats and tiles all
	// describe the same set the operator is looking at.
	const view = $derived.by<FleetSnapshot | null>(() => {
		if (!snapshot) return null;
		const q = query.trim().toLowerCase();
		if (!q) return snapshot;
		const devices = snapshot.devices.filter(
			(d) => d.hostname.toLowerCase().includes(q) || d.id.toLowerCase().includes(q)
		);
		return { ...snapshot, devices, total: devices.length };
	});

	// The narrowing box moved into the pill: ⌘K opens search already on this
	// page and its keystrokes drive the same `query` the removed input drove.
	// The Search RPC has no my-devices scope — this narrows the loaded snapshot —
	// so the registration carries `null` instead of pretending otherwise.
	$effect(() =>
		registerPageSearch({
			scope: null,
			label: m.nav_my_devices,
			get query() {
				return query;
			},
			setQuery: (value) => (query = value),
			clear: () => (query = '')
		})
	);

	async function selectDevice(device: FleetDevice) {
		selectedDevice = device;
		loadingItems = true;
		try {
			availableItems = await apiClient.listAvailableActions(device.id);
		} catch (err) {
			toast.error(getLocalizedError(err));
			console.error(err);
			availableItems = [];
		} finally {
			loadingItems = false;
		}
	}

	function backToDevices() {
		selectedDevice = null;
		availableItems = [];
	}

	async function toggleSelection(item: AvailableItem) {
		if (!selectedDevice) return;
		const key = item.sourceType + ':' + item.sourceId;
		togglingIds = new Set([...togglingIds, key]);
		try {
			await apiClient.setUserSelection(
				selectedDevice.id,
				item.sourceType,
				item.sourceId,
				!item.selected
			);
			availableItems = availableItems.map((i) =>
				i.sourceType === item.sourceType && i.sourceId === item.sourceId
					? { ...i, selected: !i.selected }
					: i
			);
			toast.success(
				item.selected ? m.my_devices_uninstall_success() : m.my_devices_install_success()
			);
		} catch (err) {
			toast.error(getLocalizedError(err));
			console.error(err);
		} finally {
			togglingIds = new Set([...togglingIds].filter((id) => id !== key));
		}
	}

	function getSourceTypeLabel(type: AssignmentSourceType): string {
		switch (type) {
			case AssignmentSourceType.ACTION:
				return m.nav_actions();
			case AssignmentSourceType.ACTION_SET:
				return m.nav_action_sets();
			case AssignmentSourceType.DEFINITION:
				return m.nav_definitions();
			default:
				return '';
		}
	}

	const STATUS_LABEL = {
		ok: m.devices_status_online,
		warn: m.fleet_status_drift,
		crit: m.devices_status_offline,
		info: m.fleet_tile_info,
		idle: m.fleet_tile_idle
	};
</script>

{#if selectedDevice}
	<!-- Bound once so the header snippet's closure carries a non-null device. -->
	{@const dev = selectedDevice}
	{@const tone = deviceTone(dev.device)}
	<PageShell contentClass="space-y-4">
		{#snippet header()}
			<div class="flex items-center gap-3">
				<Button variant="ghost" size="icon" aria-label={m.my_devices_back()} onclick={backToDevices}>
					<ArrowLeft class="h-4 w-4" />
				</Button>
				<Monitor class="h-6 w-6" />
				<div>
					<h1 class="font-mono text-2xl font-bold">{dev.hostname}</h1>
					<div class="flex items-center gap-2 text-sm text-muted-foreground">
						<Chip {tone} label={STATUS_LABEL[tone]()} />
						{#if dev.device.agentVersion}
							<span>v{dev.device.agentVersion}</span>
						{/if}
					</div>
				</div>
			</div>
		{/snippet}

		<h2 class="text-lg font-semibold">{m.my_devices_available_actions()}</h2>

		{#if loadingItems}
			<div class="flex items-center justify-center py-12">
				<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if availableItems.length === 0}
			<Card.Root>
				<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
					<Package class="mb-4 h-12 w-12 text-muted-foreground" />
					<p class="text-muted-foreground">{m.my_devices_no_available_actions()}</p>
				</Card.Content>
			</Card.Root>
		{:else}
			<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
				{#each availableItems as item (item.sourceType + ':' + item.sourceId)}
					{@const key = item.sourceType + ':' + item.sourceId}
					{@const toggling = togglingIds.has(key)}
					<Card.Root class="flex flex-col">
						<Card.Header class="flex-1">
							<div class="flex items-start justify-between gap-2">
								<div class="min-w-0 flex-1">
									<Card.Title class="text-base">{item.sourceName}</Card.Title>
									<Badge variant="outline" class="mt-1 text-xs">
										{getSourceTypeLabel(item.sourceType)}
									</Badge>
								</div>
							</div>
							{#if item.sourceDescription}
								<Card.Description class="mt-2 line-clamp-2">
									{item.sourceDescription}
								</Card.Description>
							{/if}
						</Card.Header>
						<Card.Footer>
							{#if item.selected}
								<Button
									variant="outline"
									size="sm"
									class="w-full"
									disabled={toggling}
									onclick={() => toggleSelection(item)}
								>
									{#if toggling}
										<RefreshCw class="mr-2 h-4 w-4 animate-spin" />
									{:else}
										<Check class="mr-2 h-4 w-4 text-ok" />
									{/if}
									{m.my_devices_installed()}
								</Button>
							{:else}
								<Button
									size="sm"
									class="w-full"
									disabled={toggling}
									onclick={() => toggleSelection(item)}
								>
									{#if toggling}
										<RefreshCw class="mr-2 h-4 w-4 animate-spin" />
									{:else}
										<Download class="mr-2 h-4 w-4" />
									{/if}
									{m.my_devices_install()}
								</Button>
							{/if}
						</Card.Footer>
					</Card.Root>
				{/each}
			</div>
		{/if}
	</PageShell>
{:else}
	<FleetSurface
		surfaceId="my-devices"
		heading={(count) => m.my_devices_heading({ count })}
		snapshot={view}
		{loading}
		{error}
		{nowMs}
		emptyTitle={m.my_devices_empty()}
		emptyHint={m.my_devices_empty_description()}
		openLabel={m.my_devices_available_actions()}
		onOpenDevice={selectDevice}
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
{/if}
