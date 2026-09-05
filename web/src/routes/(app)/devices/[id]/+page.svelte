<script lang="ts">
 import { onMount } from 'svelte';
 import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
 import { goto } from '$app/navigation';
 import { pushState } from '$app/navigation';
 import { page } from '$app/state';
 import { toast } from 'svelte-sonner';
 import { api } from '$lib/api';
 import { Permission, type Device } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { Button } from '$lib/components/ui/button';
 import * as Tabs from '$lib/components/ui/tabs';
 import { ArrowLeft, RefreshCw } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
 import { getLocalizedError } from '$lib/errors';
 import OverviewTab from './tabs/overview-tab.svelte';
 import GroupsTab from './tabs/groups-tab.svelte';
 import ComplianceTab from './tabs/compliance-tab.svelte';
 import AssignmentsTab from './tabs/assignments-tab.svelte';
 import HistoryTab from './tabs/history-tab.svelte';
 const { can } = consoleContext();
 let deleteOpen = $state(false);
 async function deleteDevice() { try { await api.deleteDevice({ id: { value: deviceId } }); await goto('/devices'); } catch(error) { toast.error(getLocalizedError(error)); } }
 let device = $state<Device | null>(null);
 let loading = $state(true);
 const deviceId = $derived(page.params.id ?? '');
 const tabs = $derived([...(can(Permission.GET_DEVICE) ? [{ id: 'overview', label: m.device_tab_overview() }] : []), ...(can(Permission.LIST_DEVICE_GROUPS_FOR_DEVICE) ? [{ id: 'groups', label: m.device_tab_groups() }] : []), ...(can(Permission.GET_DEVICE_COMPLIANCE) ? [{ id: 'compliance', label: m.device_tab_compliance() }] : []), ...(can(Permission.GET_DEVICE_ASSIGNMENTS) ? [{ id: 'assignments', label: 'Assigned actions' }] : []), ...(can(Permission.LIST_EXECUTION_RESULTS) ? [{ id: 'history', label: 'Execution history' }] : [])]);
 const activeTab = $derived(tabs.find(tab => tab.id === page.url.searchParams.get('tab'))?.id ?? tabs[0]?.id ?? 'overview');
 function handleTabChange(tab: string) { const url = new URL(page.url); if (tab === 'overview') url.searchParams.delete('tab'); else url.searchParams.set('tab', tab); pushState(url, {}); }
 async function loadDevice() { loading = true; try { if (can(Permission.GET_DEVICE)) device = (await api.getDevice({ id: { value: deviceId } })).device ?? null; } catch(error) { toast.error(getLocalizedError(error)); } finally { loading = false; } }
 onMount(loadDevice);
</script>
<div class="flex-1 overflow-y-auto overflow-x-hidden p-4 md:p-6 min-w-0">
<div class="space-y-6 min-w-0">
	<div class="flex items-center gap-4">
		<Button variant="ghost" size="icon" onclick={() => history.back()}>
			<ArrowLeft class="h-4 w-4" />
		</Button>
		<div class="flex-1">
			<h1 class="text-2xl font-bold">{device?.hostname ?? (loading ? m.common_loading() : deviceId)}</h1>
			<p class="text-muted-foreground text-sm">{deviceId}</p>
		</div>
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
	{:else}
        {#if !device && can(Permission.DELETE_DEVICE)}<Button variant="destructive" onclick={() => { deleteOpen = true; }}>{m.common_delete()}</Button>{/if}
        <Tabs.Root value={activeTab} onValueChange={handleTabChange} class="min-w-0">
         <Tabs.List>{#each tabs as tab}<Tabs.Trigger value={tab.id}>{tab.label}</Tabs.Trigger>{/each}</Tabs.List>
         {#if device}<Tabs.Content value="overview"><OverviewTab {device} {deviceId} /></Tabs.Content>{/if}
         {#if can(Permission.LIST_DEVICE_GROUPS_FOR_DEVICE)}<Tabs.Content value="groups"><GroupsTab {deviceId} /></Tabs.Content>{/if}
         {#if can(Permission.GET_DEVICE_COMPLIANCE)}<Tabs.Content value="compliance"><ComplianceTab {deviceId} /></Tabs.Content>{/if}
         {#if can(Permission.GET_DEVICE_ASSIGNMENTS)}<Tabs.Content value="assignments"><AssignmentsTab {deviceId} /></Tabs.Content>{/if}
         {#if can(Permission.LIST_EXECUTION_RESULTS)}<Tabs.Content value="history"><HistoryTab {deviceId} /></Tabs.Content>{/if}
        </Tabs.Root>
    {/if}
</div>
</div>

<ConfirmDeleteDialog bind:open={deleteOpen} title="Delete device" description={`Delete device ${deviceId}?`} onconfirm={() => void deleteDevice()} />
