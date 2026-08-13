<script lang="ts">
	import { goto } from '$lib/navigation';
	import { getLocalizedError } from '$lib/errors';
	import { base } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { apiClient, type Device, type ActionExecution, type InventoryTableResult, type DeviceAssignee, formatTimestampDateTime, formatDuration, ActionType } from '$lib/sdk';
	import { AssignmentTargetType, DeviceStatus, ExecutionStatus } from '$sdk/powermanage/v1/common_pb';
	import { getActionTypeLabel } from '$lib/components/actions/action-type';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Chip } from '$lib/components/fleet';
	import type { FleetTone } from '$lib/components/fleet/tone';
	import { getExecutionStatusTone } from '$lib/execution-status';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import SyncIntervalDialog, { formatSyncInterval } from '$lib/components/sync-interval-dialog.svelte';
	import InventoryIntervalDialog, { formatInventoryInterval } from '$lib/components/inventory-interval-dialog.svelte';
	import AddLabelDialog from '../add-label-dialog.svelte';
	import AssignDeviceDialog from '../assign-device-dialog.svelte';
	import {
		Trash2,
		Tag,
		Plus,
		X,
		Zap,
		Clock,
		UserPlus,
		Power,
		RotateCw,
		Cpu,
		MemoryStick,
		HardDrive,
		Terminal
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import RunScriptDialog from './run-script-dialog.svelte';
	import { openTerminal } from '$lib/shell/shell.svelte';

	interface Props {
		device: Device;
		deviceId: string;
		inventory: InventoryTableResult[];
		refreshKey: number;
		ondeviceupdate: (device: Device) => void;
	}

	let { device, deviceId, inventory, refreshKey, ondeviceupdate }: Props = $props();

	let executions = $state<ActionExecution[]>([]);
	let labelDialogOpen = $state(false);
	let deleteDialogOpen = $state(false);
	let syncIntervalDialogOpen = $state(false);
	let inventoryIntervalDialogOpen = $state(false);
	let assignDialogOpen = $state(false);
	let rebootDialogOpen = $state(false);
	let dispatchingReboot = $state(false);
	let dispatchingSync = $state(false);
	let runScriptDialogOpen = $state(false);
	let assignees = $state<DeviceAssignee[]>([]);

	$effect(() => {
		// Track refreshKey to reload when parent triggers refresh
		void refreshKey;
		loadExecutions();
		loadAssignees();
	});

	async function loadAssignees() {
		try {
			assignees = await apiClient.listDeviceAssignees(deviceId);
		} catch (error) {
			console.error('Failed to load assignees:', error);
		}
	}

	// Extract inventory fields
	const systemInfo = $derived(inventory.find((t) => t.tableName === 'system_info'));
	const osVersion = $derived(inventory.find((t) => t.tableName === 'os_version'));
	const kernelInfo = $derived(inventory.find((t) => t.tableName === 'kernel_info'));
	const memoryInfo = $derived(inventory.find((t) => t.tableName === 'memory_info'));

	function getInventoryValue(table: InventoryTableResult | undefined, column: string): string {
		if (!table || table.rows.length === 0) return '';
		return table.rows[0].data[column] ?? '';
	}

	function formatBytes(bytesStr: string): string {
		const bytes = parseInt(bytesStr, 10);
		if (isNaN(bytes) || bytes === 0) return '';
		const gb = bytes / (1024 * 1024 * 1024);
		if (gb >= 1) return `${gb.toFixed(1)} GB`;
		const mb = bytes / (1024 * 1024);
		return `${mb.toFixed(0)} MB`;
	}

	async function loadExecutions() {
		try {
			// One call. This used to follow up with a sequential GetAction per
			// distinct action just to read its name — up to 20 extra round trips to
			// render five rows, enough on its own to trip the authenticated rate
			// limiter and fail the page it was decorating. The server already
			// resolves the name into the execution row (control.proto
			// ActionExecution.action_name), which is what every other execution
			// surface in the app reads.
			const response = await apiClient.listExecutions(20, '', deviceId);
			executions = response.executions;
		} catch (error) {
			console.error('Failed to load executions:', error);
		}
	}

	function getActionName(execution: ActionExecution): string {
		// Empty for inline actions (dispatched instantly, never stored), which is
		// exactly when the type label is the honest name.
		return execution.actionName || getActionTypeLabel(execution.type);
	}

	function getExecutionStatusLabel(status: ExecutionStatus): string {
		switch (status) {
			case ExecutionStatus.PENDING: return m.executions_status_pending();
			case ExecutionStatus.RUNNING: return m.executions_status_running();
			case ExecutionStatus.SUCCESS: return m.executions_status_success();
			case ExecutionStatus.FAILED: return m.executions_status_failed();
			case ExecutionStatus.INDETERMINATE: return m.executions_status_indeterminate();
			case ExecutionStatus.SKIPPED: return m.executions_status_skipped();
			case ExecutionStatus.NOT_APPLICABLE: return m.executions_status_not_applicable();
			case ExecutionStatus.TIMEOUT: return m.executions_status_timeout();
			default: return m.executions_status_unknown();
		}
	}

	// Connectivity in the fleet vocabulary: reachable, unreachable, or a device
	// control has never heard from.
	function getStatusTone(status: DeviceStatus): FleetTone {
		switch (status) {
			case DeviceStatus.ONLINE: return 'ok';
			case DeviceStatus.OFFLINE: return 'crit';
			default: return 'idle';
		}
	}

	function getStatusLabel(status: DeviceStatus): string {
		switch (status) {
			case DeviceStatus.ONLINE: return m.devices_status_online();
			case DeviceStatus.OFFLINE: return m.devices_status_offline();
			// Same words as the device window: an unlabelled status chip would
			// claim nothing while still occupying the status slot.
			default: return m.common_unknown();
		}
	}

	async function addLabel(key: string, value: string) {
		try {
			const updated = await apiClient.setDeviceLabel(deviceId, key, value);
			if (updated) ondeviceupdate(updated);
			toast.success(m.device_detail_label_added());
			labelDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function removeLabel(key: string) {
		try {
			const updated = await apiClient.removeDeviceLabel(deviceId, key);
			if (updated) ondeviceupdate(updated);
			toast.success(m.device_detail_label_removed());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function deleteDevice() {
		try {
			await apiClient.deleteDevice(deviceId);
			toast.success(m.devices_deleted());
			goto('/devices');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateSyncInterval(minutes: number) {
		try {
			const updated = await apiClient.setDeviceSyncInterval(deviceId, minutes);
			if (updated) ondeviceupdate(updated);
			toast.success(m.device_detail_sync_updated());
			syncIntervalDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateInventoryInterval(minutes: number) {
		try {
			const updated = await apiClient.setDeviceInventoryInterval(deviceId, minutes);
			if (updated) ondeviceupdate(updated);
			toast.success(m.device_detail_inventory_updated());
			inventoryIntervalDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function assignDevice(userIds: string[], groupIds: string[]) {
		try {
			const updated = await apiClient.assignDevice(deviceId, userIds, groupIds);
			if (updated) ondeviceupdate(updated);
			await loadAssignees();
			toast.success(m.devices_assigned());
			assignDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function unassignDevice(assignee: DeviceAssignee) {
		try {
			const userId = assignee.type === AssignmentTargetType.USER ? assignee.id : undefined;
			const groupId = assignee.type === AssignmentTargetType.USER_GROUP ? assignee.id : undefined;
			const updated = await apiClient.unassignDevice(deviceId, userId, groupId);
			if (updated) ondeviceupdate(updated);
			await loadAssignees();
			toast.success(m.devices_unassigned());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function dispatchReboot() {
		dispatchingReboot = true;
		try {
			await apiClient.dispatchInstantAction(deviceId, ActionType.REBOOT);
			toast.success(m.instant_actions_reboot_dispatched());
			rebootDialogOpen = false;
			loadExecutions();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			dispatchingReboot = false;
		}
	}

	async function dispatchSync() {
		dispatchingSync = true;
		try {
			await apiClient.dispatchInstantAction(deviceId, ActionType.SYNC);
			toast.success(m.instant_actions_sync_dispatched());
			loadExecutions();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			dispatchingSync = false;
		}
	}
</script>

{#snippet sectionLabel(text: string)}
	<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{text}</span>
{/snippet}

<div class="space-y-6">
	<!-- Instant Actions -->
	<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
		{@render sectionLabel(m.instant_actions_title())}
		<div class="mt-3 flex flex-wrap gap-3">
			<Button variant="outline" onclick={dispatchSync} disabled={dispatchingSync}>
				<span class="mr-2 h-4 w-4" class:animate-spin={dispatchingSync}>
					<RotateCw class="h-4 w-4" />
				</span>
				{dispatchingSync ? m.instant_actions_dispatching() : m.instant_actions_sync()}
			</Button>
			<Button variant="outline" onclick={() => (rebootDialogOpen = true)} disabled={dispatchingReboot}>
				<Power class="mr-2 h-4 w-4" />
				{dispatchingReboot ? m.instant_actions_dispatching() : m.instant_actions_reboot()}
			</Button>
				<Button variant="outline" onclick={() => (runScriptDialogOpen = true)}>
					<Terminal class="mr-2 h-4 w-4" />
					{m.instant_actions_run_script()}
				</Button>
				<Button variant="outline" onclick={() => openTerminal(deviceId, device.hostname)}>
					<Terminal class="mr-2 h-4 w-4" />
					{m.terminal_open()}
				</Button>
		</div>
	</section>

	<!-- Device Details + Labels -->
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
					<div class="col-span-2">
						<div class="flex items-center justify-between">
							<Label class="text-muted-foreground">{m.device_detail_assigned_to()}</Label>
							<Button variant="ghost" size="sm" onclick={() => (assignDialogOpen = true)}>
								<UserPlus class="h-3 w-3 mr-1" />
								{m.common_assign()}
							</Button>
						</div>
						<div class="mt-1">
							{#if assignees.length === 0}
								<span class="text-sm text-muted-foreground">{m.device_detail_unassigned()}</span>
							{:else}
								<div class="flex flex-wrap gap-2">
									{#each assignees as assignee}
										<Badge variant="outline" class="gap-1 pr-1">
											{assignee.name}
											{#if assignee.type === AssignmentTargetType.USER_GROUP}
												<span class="text-muted-foreground text-xs">({m.nav_user_groups()})</span>
											{/if}
											<button
												onclick={() => unassignDevice(assignee)}
												class="ml-1 rounded-full p-0.5 hover:bg-muted"
											>
												<X class="h-3 w-3" />
											</button>
										</Badge>
									{/each}
								</div>
							{/if}
						</div>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.device_detail_sync_interval()}</Label>
						<div class="mt-1 flex items-center gap-2">
							<Badge variant="outline" class="gap-1">
								<Clock class="h-3 w-3" />
								{formatSyncInterval(device.syncIntervalMinutes)}
							</Badge>
							<Button variant="ghost" size="sm" onclick={() => syncIntervalDialogOpen = true}>
								{m.common_edit()}
							</Button>
						</div>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.device_detail_inventory_interval()}</Label>
						<div class="mt-1 flex items-center gap-2">
							<Badge variant="outline" class="gap-1">
								<Clock class="h-3 w-3" />
								{formatInventoryInterval(device.inventoryIntervalMinutes)}
							</Badge>
							<Button variant="ghost" size="sm" onclick={() => inventoryIntervalDialogOpen = true}>
								{m.common_edit()}
							</Button>
						</div>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.device_detail_last_inventory()}</Label>
						<div class="mt-1 flex items-center gap-2">
							<span class="text-sm">
								{device.lastInventoryAt ? formatTimestampDateTime(device.lastInventoryAt) : m.device_detail_inventory_never()}
							</span>
							{#if device.inventoryOverdue}
								<Chip tone="warn" label={m.inventory_overdue_badge()} />
							{/if}
						</div>
					</div>
				</div>

				{#if inventory.length > 0}
					<hr class="my-2" />
					<div class="grid grid-cols-2 gap-4">
						{#if getInventoryValue(osVersion, 'name')}
							<div>
								<Label class="text-muted-foreground">{m.inventory_os()}</Label>
								<p class="mt-1 text-sm">{getInventoryValue(osVersion, 'name')} {getInventoryValue(osVersion, 'version')}</p>
							</div>
						{/if}
						{#if getInventoryValue(osVersion, 'arch')}
							<div>
								<Label class="text-muted-foreground">{m.inventory_arch()}</Label>
								<p class="mt-1 text-sm">{getInventoryValue(osVersion, 'arch')}</p>
							</div>
						{/if}
						{#if getInventoryValue(systemInfo, 'cpu_brand')}
							<div>
								<Label class="text-muted-foreground flex items-center gap-1">
									<Cpu class="h-3 w-3" />
									{m.inventory_cpu()}
								</Label>
								<p class="mt-1 text-sm">{getInventoryValue(systemInfo, 'cpu_brand')}</p>
								{#if getInventoryValue(systemInfo, 'cpu_cores')}
									<p class="text-xs text-muted-foreground">{getInventoryValue(systemInfo, 'cpu_cores')} cores</p>
								{/if}
							</div>
						{/if}
						{#if getInventoryValue(systemInfo, 'physical_memory') || getInventoryValue(memoryInfo, 'memory_total')}
							<div>
								<Label class="text-muted-foreground flex items-center gap-1">
									<MemoryStick class="h-3 w-3" />
									{m.inventory_ram()}
								</Label>
								<p class="mt-1 text-sm">{formatBytes(getInventoryValue(systemInfo, 'physical_memory') || getInventoryValue(memoryInfo, 'memory_total'))}</p>
							</div>
						{/if}
						{#if getInventoryValue(kernelInfo, 'version')}
							<div>
								<Label class="text-muted-foreground">{m.inventory_kernel()}</Label>
								<p class="mt-1 text-sm">{getInventoryValue(kernelInfo, 'version')}</p>
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</section>

		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
			<div class="flex flex-wrap items-center justify-between gap-2">
				{@render sectionLabel(m.device_detail_labels())}
				<Button size="sm" variant="outline" onclick={() => (labelDialogOpen = true)}>
					<Plus class="mr-2 h-4 w-4" />
					{m.device_detail_add_label()}
				</Button>
			</div>
			<div class="mt-3">
				{#if Object.keys(device.labels).length === 0}
					<p class="text-muted-foreground text-sm">{m.device_detail_no_labels()}</p>
				{:else}
					<div class="flex flex-wrap gap-2">
						{#each Object.entries(device.labels) as [key, value]}
							<Badge variant="outline" class="gap-1 pr-1">
								<Tag class="h-3 w-3" />
								{key}: {value}
								<button
									onclick={() => removeLabel(key)}
									class="ml-1 rounded-full p-0.5 hover:bg-muted"
								>
									<X class="h-3 w-3" />
								</button>
							</Badge>
						{/each}
					</div>
				{/if}
			</div>
		</section>
	</div>

	<!-- Recent Executions -->
	<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
		<div class="flex flex-wrap items-center justify-between gap-2">
			{@render sectionLabel(m.device_detail_recent_executions())}
			<Button variant="outline" onclick={() => goto(`/executions?device=${deviceId}`)}>
				{m.common_view_all()}
			</Button>
		</div>
		<div class="mt-3">
			{#if executions.length === 0}
				<div class="flex flex-col items-center justify-center py-8 text-center">
					<Zap class="h-8 w-8 text-muted-foreground mb-2" />
					<p class="text-muted-foreground">{m.device_detail_no_executions()}</p>
				</div>
			{:else}
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>{m.executions_table_action()}</Table.Head>
							<Table.Head>{m.executions_table_status()}</Table.Head>
							<Table.Head>{m.executions_table_created()}</Table.Head>
							<Table.Head>{m.executions_table_duration()}</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each executions.slice(0, 5) as execution}
							<Table.Row>
								<Table.Cell>
									<a href="{base}/executions/{execution.id}" class="hover:underline">
										{getActionName(execution)}
									</a>
								</Table.Cell>
								<Table.Cell>
									<Chip
										tone={getExecutionStatusTone(execution.status)}
										label={getExecutionStatusLabel(execution.status)}
									/>
								</Table.Cell>
								<Table.Cell class="text-sm">
									{formatTimestampDateTime(execution.createdAt)}
								</Table.Cell>
								<Table.Cell class="text-sm">
									{formatDuration(execution.durationMs)}
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			{/if}
		</div>
	</section>

	<!-- Danger Zone -->
	<section class="rounded-xl border border-crit/50 bg-surface p-4 shadow-plate">
		<p class="text-sm font-semibold text-crit">{m.common_danger_zone()}</p>
		<div class="mt-3">
			<Button variant="destructive" onclick={() => (deleteDialogOpen = true)}>
				<Trash2 class="mr-2 h-4 w-4" />
				{m.device_detail_delete_device()}
			</Button>
		</div>
	</section>
</div>

<AddLabelDialog bind:open={labelDialogOpen} onadd={addLabel} />

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.devices_delete_dialog_title()}
	description={m.devices_delete_dialog_description({ hostname: device.hostname })}
	onconfirm={deleteDevice}
/>

<SyncIntervalDialog
	bind:open={syncIntervalDialogOpen}
	currentMinutes={device.syncIntervalMinutes}
	title={m.sync_dialog_title()}
	description={m.sync_dialog_description()}
	onsave={updateSyncInterval}
/>

<InventoryIntervalDialog
	bind:open={inventoryIntervalDialogOpen}
	currentMinutes={device.inventoryIntervalMinutes}
	title={m.inventory_dialog_title()}
	description={m.inventory_dialog_description()}
	onsave={updateInventoryInterval}
/>

<AlertDialog.Root bind:open={rebootDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.instant_actions_reboot_confirm_title()}</AlertDialog.Title>
			<AlertDialog.Description>
				{m.instant_actions_reboot_confirm_description({ hostname: device.hostname })}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={dispatchReboot} disabled={dispatchingReboot}>
				{dispatchingReboot ? m.instant_actions_dispatching() : m.instant_actions_reboot()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

	<RunScriptDialog bind:open={runScriptDialogOpen} {deviceId} ondispatched={loadExecutions} />

<AssignDeviceDialog bind:open={assignDialogOpen} hostname={device.hostname} existingUserIds={assignees.filter(a => a.type === AssignmentTargetType.USER).map(a => a.id)} existingGroupIds={assignees.filter(a => a.type === AssignmentTargetType.USER_GROUP).map(a => a.id)} onassign={assignDevice} />
