<script lang="ts">

	import { onMount, onDestroy, untrack } from 'svelte';
	import { getLocalizedError } from '$lib/errors';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, fetchAllPages, DeviceStatus, type DeviceGroup, type Device } from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';
	import { createFormValidation } from '$lib/forms';
	import { editNameSchema } from '$lib/forms/schemas/common';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Tile, Chip, Stat } from '$lib/components/fleet';
	import ItemTablePicker from '$lib/components/item-table-picker.svelte';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import SyncIntervalDialog, { formatSyncInterval } from '$lib/components/sync-interval-dialog.svelte';
	import InventoryIntervalDialog, {
		formatInventoryInterval
	} from '$lib/components/inventory-interval-dialog.svelte';
	import MaintenanceWindowDialog, {
		entriesFromWindow,
		type MaintenanceWindowEntryInput
	} from '$lib/components/maintenance-window-dialog.svelte';
	import DynamicRuleEditor, {
		type RulePreviewRow,
		ruleSubtext
	} from '$lib/components/dynamic-rule-editor.svelte';
	import FutureScopeDialog from '$lib/components/future-scope-dialog.svelte';
	import type { QueryEditorState } from '$lib/components/query-builder.svelte';
	import MembersTab from './members-tab.svelte';
	import { create } from '@bufbuild/protobuf';
	import {
		MaintenanceWindowSchema,
		MaintenanceWindowEntrySchema
	} from '$contract/cadestro/v1/common_pb';
	import {
		shell,
		enterContext,
		updateContext,
		exitContext,
		leaveContext,
		removeDraft,
		claimDraft,
		draftIdFor,
		type ContextState
	} from '$lib/shell/shell.svelte';
	import { ArrowLeft, RefreshCw, Pencil, Play, Clock } from '@lucide/svelte';

	let group = $state<DeviceGroup | null>(null);
	let memberDevices = $state<Array<{ deviceId: string; hostname: string; agentVersion: string }>>([]);
	let memberDeviceIds = $state<string[]>([]);
	let allDevices = $state<Device[]>([]);
	let loading = $state(true);
	let evaluating = $state(false);
	let activeTab = $state('members');

	let deleteDialogOpen = $state(false);
	let addDeviceDialogOpen = $state(false);
	let selectedDeviceIds = $state<string[]>([]);
	let syncIntervalDialogOpen = $state(false);
	let inventoryIntervalDialogOpen = $state(false);
	let maintenanceWindowDialogOpen = $state(false);

	let editingIdentity = $state(false);
	let savingIdentity = $state(false);
	let draftName = $state('');
	let draftDescription = $state('');
	const nameValidation = createFormValidation(editNameSchema);

	const groupId = $derived(page.params.id ?? '');

	const groupContextId = $derived(`device-group:${groupId}`);
	const deviceById = $derived(new Map<string, Device>(allDevices.flatMap((d) => d.id ? [[d.id.value, d] as const] : [])));
	const availableDevices = $derived(allDevices.filter((d) => !memberDeviceIds.includes((d.id?.value ?? ''))));

	const identityDirty = $derived(
		editingIdentity &&
			group !== null &&
			(draftName !== group.name || draftDescription !== group.description)
	);
	const identityNameValid = $derived(draftName.trim().length > 0);

	let draftQuery = $state('');
	let ruleState = $state<QueryEditorState>({
		text: '',
		valid: false,
		count: null,
		error: m.query_incomplete(),
		validating: false
	});
	let ruleConfirmOpen = $state(false);

	let lastSavedQuery = '';
	const savedQuery = $derived(group?.dynamicQuery ?? '');
	const ruleDirty = $derived(group !== null && draftQuery !== savedQuery);
	const ruleValid = $derived(ruleState.valid === true);

	const PREVIEW_LABEL_CAP = 3;

	const previewRows = $derived<RulePreviewRow[]>(
		memberDevices.map((row) => {
			const device = deviceById.get(row.deviceId);
			const all = Object.entries(device?.labels ?? {}).map(([k, v]) => `${k}=${v}`);
			return {
				id: row.deviceId,
				primary: row.hostname,
				attributes: all.slice(0, PREVIEW_LABEL_CAP),

				hiddenAttributes: Math.max(0, all.length - PREVIEW_LABEL_CAP),
				tone:
					device?.status === DeviceStatus.ONLINE
						? 'ok'
						: device?.status === DeviceStatus.OFFLINE
							? 'crit'
							: 'idle'
			};
		})
	);

	onMount(() => {
		if (groupId) loadData();
	});

	onDestroy(() => {
		if (owns) {
			owns = false;
			leaveContext(groupContextId);
		}
	});

	async function loadData() {
		loading = true;
		try {
			const response = await apiClient.getDeviceGroup(groupId);
			group = response.group ?? null;
			memberDeviceIds = (response.deviceIds ?? []).map((id) => id.value);
			memberDevices = (response.devices ?? []).map((row) => ({
				...row,
				deviceId: row.deviceId?.value ?? ''
			}));
			if (group) {
				if (!editingIdentity) {
					draftName = group.name;
					draftDescription = group.description;
				}

				if (group.dynamicQuery !== lastSavedQuery) {
					lastSavedQuery = group.dynamicQuery;
					draftQuery = group.dynamicQuery;
				}

				const parked = claimDraft(groupContextId) as GroupDraft | undefined;
				if (parked) {
					draftName = parked.name;
					draftDescription = parked.description;
					if (parked.query !== undefined) draftQuery = parked.query;
					editingIdentity = true;
				}
			}

			allDevices = await fetchAllPages<Device>(async (size, token) => {
				const r = await apiClient.listDevices(size, token);
				return { items: r.devices, nextPageToken: r.nextPageToken };
			});
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	const contextRoute = $derived(`/device-groups/${groupId}`);

	interface GroupDraft {
		name: string;
		description: string;
		query: string;
	}

	let owns = false;

	let stashParked = false;

	function startIdentityEdit() {
		draftName = group?.name ?? '';
		draftDescription = group?.description ?? '';
		nameValidation.clearErrors();
		editingIdentity = true;
	}

	function revertIdentityEdit() {
		editingIdentity = false;
		draftName = group?.name ?? '';
		draftDescription = group?.description ?? '';
	}

	function contextState(): ContextState {
		return {
			id: groupContextId,
			route: contextRoute,
			title: draftName || (group?.name ?? ''),

			dirty: identityDirty || ruleDirty,
			valid: identityNameValid && ruleValid,

			commitLabel:
				ruleDirty && group && !group.isDynamic ? m.query_commit_convert() : m.common_save(),

			subtext: !identityNameValid
				? m.validation_name_required()
				: ruleDirty || !ruleValid
					? ruleSubtext(ruleState, 'device').text
					: undefined,
			subtextTone: (!identityNameValid
				? 'warn'
				: ruleDirty || !ruleValid
					? ruleSubtext(ruleState, 'device').tone
					: 'neutral') as 'neutral' | 'warn',
			onCommit: () => {

				owns = false;

				if (ruleDirty) ruleConfirmOpen = true;
				else void saveGroup();
			},
			onCancel: () => {
				owns = false;
				revertIdentityEdit();
				draftQuery = savedQuery;
			},

			onStash: () => {
				stashParked = true;
				owns = false;
			},
			onRestore: () => {
				stashParked = false;
				owns = true;
			},
			stashPayload: (): GroupDraft => ({
				name: draftName,
				description: draftDescription,
				query: draftQuery
			}),
			stashSubtitle: m.device_groups_edit_identity(),

			extraActions: [
				{
					id: 'window',
					label: m.device_group_detail_window_label(),
					onRun: () => (maintenanceWindowDialogOpen = true)
				},
				{
					id: 'delete',
					label: m.device_groups_delete_group(),
					tone: 'danger' as const,
					onRun: () => (deleteDialogOpen = true)
				}
			]
		};
	}

	function acquire() {
		owns = true;
		stashParked = false;

		removeDraft(draftIdFor(groupContextId));
		enterContext(contextState());
	}

	function release() {
		owns = false;

		if (shell.pill.context?.id === groupContextId) exitContext();
	}

	$effect(() => {

		const active = group !== null && !savingIdentity;

		void activeTab;

		const holder = shell.pill.context?.id ?? null;

		const patch = contextState();

		untrack(() => {

			const held = holder === groupContextId;
			if (!active) {
				if (held) release();
				return;
			}
			if (held) updateContext(patch);

			else if (holder === null && !stashParked) acquire();
		});
	});

	async function saveGroup() {
		if (!group) return;
		const name = draftName.trim();
		const description = draftDescription.trim();
		const query = draftQuery;
		const wantsRule = ruleDirty;
		savingIdentity = true;
		try {
			await nameValidation.handleSubmit({ name }, async () => {
				try {
					if (name !== group!.name) {
						group = (await apiClient.renameDeviceGroup(groupId, name)) ?? group;
						toast.success(m.device_group_detail_name_updated());
					}
					if (description !== group!.description) {
						group = (await apiClient.updateDeviceGroupDescription(groupId, description)) ?? group;
						toast.success(m.device_group_detail_desc_updated());
					}
					if (wantsRule) {
						group = (await apiClient.updateDeviceGroupQuery(groupId, true, query)) ?? group;
						toast.success(m.device_group_detail_query_updated());
					}
					editingIdentity = false;
					await loadData();
				} catch (error) {
					toast.error(getLocalizedError(error));
					console.error(error);
				}
			});
		} finally {
			savingIdentity = false;
		}
	}

	async function deleteGroup() {
		try {
			await apiClient.deleteDeviceGroup(groupId);
			toast.success(m.device_groups_deleted());
			goto('/device-groups');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function addDevices() {
		if (selectedDeviceIds.length === 0) return;
		try {
			await apiClient.addDeviceToGroup(groupId, selectedDeviceIds);
			addDeviceDialogOpen = false;
			selectedDeviceIds = [];
			toast.success(m.device_group_detail_device_added());
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function removeDevice(deviceId: string) {
		try {
			await apiClient.removeDeviceFromGroup(groupId, deviceId);
			toast.success(m.device_group_detail_device_removed());
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function evaluateGroup() {
		evaluating = true;
		try {
			const result = await apiClient.evaluateDynamicGroup(groupId);
			toast.success(
				m.device_group_detail_evaluated({
					added: result.devicesAdded,
					removed: result.devicesRemoved
				})
			);
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			evaluating = false;
		}
	}

	async function updateSyncInterval(minutes: number) {
		try {
			group = (await apiClient.setDeviceGroupSyncInterval(groupId, minutes)) ?? null;
			toast.success(m.device_group_detail_sync_updated());
			syncIntervalDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateInventoryInterval(minutes: number) {
		try {
			group = (await apiClient.setDeviceGroupInventoryInterval(groupId, minutes)) ?? null;
			toast.success(m.device_group_detail_inventory_updated());
			inventoryIntervalDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateMaintenanceWindow(entries: MaintenanceWindowEntryInput[]) {
		try {

			const window =
				entries.length === 0
					? undefined
					: create(MaintenanceWindowSchema, {
							schedule: entries.map((e) =>
								create(MaintenanceWindowEntrySchema, { days: e.days, allow: e.allow })
							)
						});
			group = (await apiClient.setDeviceGroupMaintenanceWindow(groupId, window)) ?? null;
			toast.success(m.device_group_detail_window_updated());
			maintenanceWindowDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}
</script>

<PageShell contentClass="space-y-4">
	{#snippet header()}
		<div class="flex items-center gap-2">
			<Button variant="ghost" size="icon" aria-label={m.common_back()} onclick={() => history.back()}>
				<ArrowLeft class="h-4 w-4" />
			</Button>

			<div class="min-w-0 flex-1">
				<h1 class="truncate text-2xl font-bold">{group?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{groupId}</p>
			</div>
			<div class="ml-auto flex gap-2">
				{#if group?.isDynamic}
					<Button variant="outline" size="sm" onclick={evaluateGroup} disabled={evaluating}>
						<span class="mr-2 h-4 w-4" class:animate-spin={evaluating}><Play class="h-4 w-4" /></span>
						{m.device_group_detail_re_evaluate()}
					</Button>
				{/if}
				<Button variant="outline" size="sm" onclick={loadData} disabled={loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={loading}><RefreshCw class="h-4 w-4" /></span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	{#if loading && !group}
		<div class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate">
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if group}

		<div
			class="rounded-xl border border-hair bg-surface p-4 shadow-plate"
			data-tour="group-header"
			data-testid="group-header"
		>
			<div class="flex flex-wrap items-start gap-3">
				<span class="mt-1 w-4 shrink-0"><Tile tone={group.isDynamic ? 'info' : 'idle'} label={group.name} /></span>
				<div class="min-w-0 flex-1">
					{#if editingIdentity}
						<div class="space-y-1.5" data-testid="identity-edit">

							<p class="text-xs text-muted-foreground">{m.device_groups_identity_pill_hint()}</p>
							<Input
								bind:value={draftName}
								aria-label={m.common_name()}
								aria-invalid={!!nameValidation.errors.name}
								class="h-8 font-mono text-sm"
							/>
							<FieldError error={nameValidation.errors.name} />
							<Textarea
								bind:value={draftDescription}
								aria-label={m.common_description()}
								rows={2}
								placeholder={m.device_groups_desc_placeholder()}
								class="text-sm"
							/>
						</div>
					{:else}
						<div class="flex items-center gap-2">
							<h2 class="truncate font-mono text-lg font-semibold">{group.name}</h2>
							<Button
								variant="ghost"
								size="icon-sm"
								aria-label={m.device_groups_edit_identity()}
								onclick={startIdentityEdit}
							>
								<Pencil class="h-3.5 w-3.5" />
							</Button>
						</div>
						<p class="font-mono text-xs text-faint">{group.id}</p>
						<p class="mt-1 text-sm text-muted-foreground">
							{group.description || m.common_no_description()}
						</p>
					{/if}
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<Chip
						tone={group.isDynamic ? 'info' : 'idle'}
						label={group.isDynamic ? m.device_groups_dynamic() : m.device_groups_static()}
					/>
					<Stat
						tone={group.isDynamic ? 'info' : 'ok'}
						value={group.memberCount}
						label={m.device_group_detail_devices()}
					/>

				</div>
			</div>
		</div>

		<Tabs.Root value={activeTab} onValueChange={(v) => (activeTab = v)}>
			<Tabs.List>
				<Tabs.Trigger value="members">
					{m.device_groups_members_tab({ count: group.memberCount })}
				</Tabs.Trigger>
				<Tabs.Trigger value="rule">{m.query_tab_rule()}</Tabs.Trigger>
				<Tabs.Trigger value="schedules">{m.device_groups_tab_schedules()}</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="members" class="mt-3">
				<MembersTab
					members={memberDevices}
					devices={deviceById}
					isDynamic={group.isDynamic}
					canAdd={availableDevices.length > 0}
					onadd={() => {
						selectedDeviceIds = [];
						addDeviceDialogOpen = true;
					}}
					onremove={removeDevice}
				/>
			</Tabs.Content>

			<Tabs.Content value="rule" class="mt-3">
				<DynamicRuleEditor
					kind="device"
					savedQuery={group.dynamicQuery}
					bind:draft={draftQuery}
					isDynamic={group.isDynamic}
					rows={previewRows}
					total={group.memberCount}
					onstate={(state) => (ruleState = state)}
				/>
			</Tabs.Content>

			<Tabs.Content value="schedules" class="mt-3">
				<div class="divide-y rounded-xl border border-hair bg-surface shadow-plate" data-testid="schedules-tab">
					<div class="flex flex-wrap items-center gap-3 p-3">
						<div class="min-w-0 flex-1">
							<p class="text-sm font-medium">{m.sync_label()}</p>
							<p class="text-xs text-muted-foreground">{m.device_group_detail_sync_hint()}</p>
						</div>
						<Chip tone="idle">
							<Clock class="h-3 w-3" />{formatSyncInterval(group.syncIntervalMinutes)}
						</Chip>
						<Button variant="outline" size="sm" onclick={() => (syncIntervalDialogOpen = true)}>
							{m.common_edit()}
						</Button>
					</div>
					<div class="flex flex-wrap items-center gap-3 p-3">
						<div class="min-w-0 flex-1">
							<p class="text-sm font-medium">{m.inventory_interval_label()}</p>
						</div>
						<Chip tone="idle">
							<Clock class="h-3 w-3" />{formatInventoryInterval(group.inventoryIntervalMinutes)}
						</Chip>
						<Button variant="outline" size="sm" onclick={() => (inventoryIntervalDialogOpen = true)}>
							{m.common_edit()}
						</Button>
					</div>
				</div>
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</PageShell>

<FutureScopeDialog
	bind:open={ruleConfirmOpen}
	queryText={ruleState.text}
	count={ruleState.count}
	kind="device"
	converting={!(group?.isDynamic ?? false)}
	currentMembers={group?.memberCount ?? 0}
	onconfirm={() => {
		ruleConfirmOpen = false;
		void saveGroup();
	}}
	oncancel={() => (ruleConfirmOpen = false)}
/>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.device_groups_delete_dialog_title()}
	description={m.device_groups_delete_dialog_description({ name: group?.name ?? '' })}
	onconfirm={deleteGroup}
/>

<Dialog.Root bind:open={addDeviceDialogOpen}>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{m.device_group_detail_add_dialog_title()}</Dialog.Title>
			<Dialog.Description>{m.device_group_detail_add_dialog_description()}</Dialog.Description>
		</Dialog.Header>

		<ItemTablePicker
			items={availableDevices.map((d) => ({ id: (d.id?.value ?? ''), hostname: d.hostname, status: d.status }))}
			bind:selected={selectedDeviceIds}
			searchPlaceholder={m.picker_search_devices()}
			emptyMessage={m.picker_no_devices()}
			searchFilter={(item, query) =>
				item.hostname.toLowerCase().includes(query.toLowerCase()) ||
				item.id.toLowerCase().includes(query.toLowerCase())}
		>
			{#snippet headerRow()}
				<Table.Head>{m.devices_table_hostname()}</Table.Head>
			{/snippet}
			{#snippet itemRow(item)}
				<Table.Cell><span class="font-mono text-sm">{item.hostname}</span></Table.Cell>
			{/snippet}
		</ItemTablePicker>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (addDeviceDialogOpen = false)}>{m.common_cancel()}</Button>
			<Button onclick={addDevices} disabled={selectedDeviceIds.length === 0}>{m.common_add()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<SyncIntervalDialog
	bind:open={syncIntervalDialogOpen}
	currentMinutes={group?.syncIntervalMinutes ?? 0}
	title={m.device_group_detail_sync_dialog_title()}
	description={m.device_group_detail_sync_dialog_description()}
	onsave={updateSyncInterval}
/>

<InventoryIntervalDialog
	bind:open={inventoryIntervalDialogOpen}
	currentMinutes={group?.inventoryIntervalMinutes ?? 0}
	title={m.device_group_detail_inventory_dialog_title()}
	description={m.device_group_detail_inventory_dialog_description()}
	onsave={updateInventoryInterval}
/>

<MaintenanceWindowDialog
	bind:open={maintenanceWindowDialogOpen}
	entries={entriesFromWindow(group?.maintenanceWindow)}
	title={m.device_group_detail_window_dialog_title()}
	description={m.device_group_detail_window_dialog_description()}
	onsave={updateMaintenanceWindow}
/>
