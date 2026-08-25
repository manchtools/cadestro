<script lang="ts">
	import { onMount } from 'svelte';
	import { pushState } from '$app/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type Device, type InventoryTableResult } from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import * as Tabs from '$lib/components/ui/tabs';
	import { ArrowLeft, RefreshCw } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	import OverviewTab from './tabs/overview-tab.svelte';
	import SystemResourcesTab from './tabs/system-resources-tab.svelte';
	import HardwareTab from './tabs/hardware-tab.svelte';
	import SoftwareTab from './tabs/software-tab.svelte';
	import PoliciesTab from './tabs/policies-tab.svelte';
	import GroupsTab from './tabs/groups-tab.svelte';
	import ComplianceTab from './tabs/compliance-tab.svelte';
	import OsqueryTab from './tabs/osquery-tab.svelte';
	import LogsTab from './tabs/logs-tab.svelte';

	const validTabs = ['overview', 'system-resources', 'hardware', 'software', 'policies', 'groups', 'compliance', 'osquery', 'logs'] as const;
	type TabValue = (typeof validTabs)[number];

	let device = $state<Device | null>(null);
	let inventory = $state<InventoryTableResult[]>([]);
	let loading = $state(true);
	let inventoryLoading = $state(false);
	let refreshKey = $state(0);

	const deviceId = $derived(page.params.id ?? '');

	const activeTab = $derived.by<TabValue>(() => {
		const param = page.url.searchParams.get('tab');
		if (param && validTabs.includes(param as TabValue)) return param as TabValue;
		return 'overview';
	});

	function handleTabChange(tab: string) {
		const url = new URL(window.location.href);
		if (tab === 'overview') {
			url.searchParams.delete('tab');
		} else {
			url.searchParams.set('tab', tab);
		}
		pushState(url, {});
	}

	onMount(() => {
		if (deviceId) {
			loadDevice();
			loadInventory();
		}
	});

	async function loadDevice() {
		if (!deviceId) return;
		loading = true;
		try {
			device = (await apiClient.getDevice(deviceId)) ?? null;
			refreshKey++;
			loadInventory();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function loadInventory() {
		if (!deviceId) return;
		try {
			const response = await apiClient.getDeviceInventory(deviceId);
			inventory = response.tables;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error('Failed to load inventory:', error);
		}
	}

	async function refreshInventory() {
		if (!deviceId) return;
		inventoryLoading = true;
		try {
			await apiClient.refreshDeviceInventory(deviceId);
			toast.success(m.inventory_refreshed());

			setTimeout(() => loadInventory(), 2000);
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			inventoryLoading = false;
		}
	}

	function handleDeviceUpdate(updated: Device) {
		device = updated;
	}
</script>

<div class="flex-1 overflow-y-auto overflow-x-hidden p-4 md:p-6 min-w-0">
<div class="space-y-6 min-w-0">
	<div class="flex items-center gap-4">
		<Button variant="ghost" size="icon" onclick={() => history.back()}>
			<ArrowLeft class="h-4 w-4" />
		</Button>
		<div class="flex-1">
			<h1 class="text-2xl font-bold">{device?.hostname ?? m.common_loading()}</h1>
			<p class="text-muted-foreground text-sm">{deviceId}</p>
		</div>
		<Button variant="outline" size="sm" onclick={refreshInventory} disabled={inventoryLoading}>
			<span class="mr-2 h-4 w-4" class:animate-spin={inventoryLoading}>
				<RefreshCw class="h-4 w-4" />
			</span>
			{m.inventory_refresh()}
		</Button>
		<Button variant="outline" onclick={loadDevice} disabled={loading}>
			<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
				<RefreshCw class="h-4 w-4" />
			</span>
			{m.common_refresh()}
		</Button>
	</div>

	{#if loading && !device}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if device}
		<Tabs.Root value={activeTab} onValueChange={handleTabChange} class="min-w-0">
			<Tabs.List>
				<Tabs.Trigger value="overview">{m.device_tab_overview()}</Tabs.Trigger>
				<Tabs.Trigger value="system-resources">{m.device_tab_system_resources()}</Tabs.Trigger>
				<Tabs.Trigger value="hardware">{m.device_tab_hardware()}</Tabs.Trigger>
				<Tabs.Trigger value="software">{m.device_tab_software()}</Tabs.Trigger>
				<Tabs.Trigger value="policies">{m.device_tab_policies()}</Tabs.Trigger>
				<Tabs.Trigger value="groups">{m.device_tab_groups()}</Tabs.Trigger>
					<Tabs.Trigger value="compliance">{m.device_tab_compliance()}</Tabs.Trigger>
					<Tabs.Trigger value="osquery">{m.device_tab_osquery()}</Tabs.Trigger>
					<Tabs.Trigger value="logs">{m.device_tab_logs()}</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="overview">
				<OverviewTab {device} {deviceId} {inventory} {refreshKey} ondeviceupdate={handleDeviceUpdate} />
			</Tabs.Content>

			<Tabs.Content value="system-resources">
				<SystemResourcesTab {deviceId} />
			</Tabs.Content>

			<Tabs.Content value="hardware">
				<HardwareTab {inventory} />
			</Tabs.Content>

			<Tabs.Content value="software">
				<SoftwareTab {inventory} {deviceId} />
			</Tabs.Content>

			<Tabs.Content value="policies">
				<PoliciesTab {deviceId} />
			</Tabs.Content>

			<Tabs.Content value="groups">
				<GroupsTab {deviceId} />
			</Tabs.Content>

				<Tabs.Content value="compliance">
				<ComplianceTab {deviceId} />
				</Tabs.Content>
				<Tabs.Content value="osquery">
					<OsqueryTab {deviceId} />
				</Tabs.Content>
				<Tabs.Content value="logs">
					<LogsTab {deviceId} />
				</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>
</div>
