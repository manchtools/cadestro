<script lang="ts">
 import { onMount } from 'svelte';
 import { page } from '$app/state';
 import { goto } from '$lib/navigation';
 import { toast } from 'svelte-sonner';
 import { parseIds } from '$lib/id-list';
 import { addMemberships } from '$lib/membership';
 import { api } from '$lib/api';
 import { Permission, type Device, type DeviceGroup, type DeviceGroupMember } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { collectPages } from '$lib/console';
 import { getLocalizedError } from '$lib/errors';
 import * as m from '$lib/paraglide/messages';
 import { Button } from '$lib/components/ui/button';
 import { Input } from '$lib/components/ui/input';
 import { Textarea } from '$lib/components/ui/textarea';
 import { FieldError } from '$lib/components/ui/field-error';
 import * as Table from '$lib/components/ui/table';
 import * as Dialog from '$lib/components/ui/dialog';
 import { Tile, Chip, Stat } from '$lib/components/fleet';
 import PageShell from '$lib/components/page-shell.svelte';
 import ItemTablePicker from '$lib/components/item-table-picker.svelte';
 import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
 import MembersTab from './members-tab.svelte';
 import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
 import { ArrowLeft, RefreshCw, Pencil } from '@lucide/svelte';
 const { can } = consoleContext();
 const groupId = $derived(page.params.id ?? '');
 let group = $state<DeviceGroup | null>(null);
 let members = $state<DeviceGroupMember[]>([]);
 let allDevices = $state<Device[]>([]);
 let loading = $state(true);
 let editingIdentity = $state(false);
 let saving = $state(false);
 let draftName = $state('');
 let draftDescription = $state('');
 let deleteDialogOpen = $state(false);
 let addDeviceDialogOpen = $state(false);
 let selectedDeviceIds = $state<string[]>([]);
 let deviceIdsText = $state('');
 let adding = $state(false);
 let membershipReady = $state(true);
 const nameError = $derived(draftName.trim() ? undefined : 'Enter a group name');
 const memberDevices = $derived(members.map(member => ({ deviceId: member.deviceId?.value ?? '', hostname: member.hostname, agentVersion: member.agentVersion })));
 const deviceById = $derived(new Map(allDevices.map(device => [device.id?.value ?? '', device])));
 const availableDevices = $derived(allDevices.filter(device => !members.some(member => member.deviceId?.value === device.id?.value)));
 bindBuilderContext(`device-group:${page.params.id}`, () => editingIdentity && !saving ? { title: draftName, route: `/device-groups/${groupId}`, dirty: draftName !== group?.name || draftDescription !== group?.description, valid: !nameError, commitLabel: m.common_save(), onCommit: () => void saveIdentity(), onCancel: () => { editingIdentity = false; } } : null);
 function startIdentityEdit() { if (!group) return; draftName = group.name; draftDescription = group.description; editingIdentity = true; }
 async function loadData() {
  loading = true;
  try {
   if (can(Permission.GET_DEVICE_GROUP)) { const r = await api.getDeviceGroup({ id: { value: groupId } }); group = r.group ?? null; members = r.devices; membershipReady = true; selectedDeviceIds = selectedDeviceIds.filter(id => !members.some(member => member.deviceId?.value === id)); } else if (can(Permission.LIST_DEVICE_GROUPS)) group = (await collectPages(async pageToken => { const r = await api.listDeviceGroups({ pageSize: 100, pageToken }); return { items: r.groups, nextPageToken: r.nextPageToken }; })).find(item => item.id?.value === groupId) ?? null;
   if (can(Permission.LIST_DEVICES)) allDevices = await collectPages(async pageToken => { const r = await api.listDevices({ pageSize: 100, pageToken }); return { items: r.devices, nextPageToken: r.nextPageToken }; });
  } catch(error) { toast.error(getLocalizedError(error)); } finally { loading = false; }
 }
 async function saveIdentity() {
  if (!group || nameError) return; saving = true;
  try { if (draftName !== group.name && can(Permission.RENAME_DEVICE_GROUP)) group = (await api.renameDeviceGroup({ id: group.id, name: draftName.trim() })).group ?? group;
   if (draftDescription !== group.description && can(Permission.UPDATE_DEVICE_GROUP_DESCRIPTION)) group = (await api.setDeviceGroupDescription({ id: group.id, description: draftDescription.trim() })).group ?? group; editingIdentity = false;
  } catch(error) { toast.error(getLocalizedError(error)); } finally { saving = false; }
 }
 async function addDevices() {
  if (adding || !membershipReady) return;
  adding = true;
  const result = await addMemberships(selectedDeviceIds, id => api.addDeviceToGroup({ groupId: { value: groupId }, deviceId: { value: id } }), async () => {
   if (!can(Permission.GET_DEVICE_GROUP)) throw new Error('Group membership cannot be refreshed');
   const response = await api.getDeviceGroup({ id: { value: groupId } });
   members = response.devices;
   group = response.group ?? group;
   return members.map(member => member.deviceId?.value ?? '');
  });
  selectedDeviceIds = result.remaining; deviceIdsText = result.remaining.join(',');
  membershipReady = result.ready;
  adding = false;
  if (result.error) toast.error(`${getLocalizedError(result.error)}${result.ready ? '' : ' The result is uncertain; verify group membership before starting another addition.'}`);
  else { addDeviceDialogOpen = false; await loadData(); }
 }

 async function removeDevice(id: string) { try { await api.removeDeviceFromGroup({ groupId: { value: groupId }, deviceId: { value: id } }); await loadData(); } catch(error) { toast.error(getLocalizedError(error)); } }
 async function deleteGroup() { try { await api.deleteDeviceGroup({ id: { value: groupId } }); await goto('/device-groups'); } catch(error) { toast.error(getLocalizedError(error)); } }
 onMount(loadData);
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
				<span class="mt-1 w-4 shrink-0"><Tile tone={'idle'} label={group.name} /></span>
				<div class="min-w-0 flex-1">
					{#if editingIdentity}
						<div class="space-y-1.5" data-testid="identity-edit">

							<p class="text-xs text-muted-foreground">{m.device_groups_identity_pill_hint()}</p>
							<Input
								disabled={!can(Permission.RENAME_DEVICE_GROUP)} bind:value={draftName}
								aria-label={m.common_name()}
								aria-invalid={!!nameError}
								class="h-8 font-mono text-sm"
							/>
							<FieldError error={nameError} />
							<Textarea
								disabled={!can(Permission.UPDATE_DEVICE_GROUP_DESCRIPTION)} bind:value={draftDescription}
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
								disabled={!can(Permission.RENAME_DEVICE_GROUP) && !can(Permission.UPDATE_DEVICE_GROUP_DESCRIPTION)} onclick={startIdentityEdit}
							>
								<Pencil class="h-3.5 w-3.5" />
							</Button>
						</div>
						<p class="font-mono text-xs text-faint">{group.id?.value ?? ''}</p>
						<p class="mt-1 text-sm text-muted-foreground">
							{group.description || m.common_no_description()}
						</p>
					{/if}
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<Chip
						tone={'idle'}
						label={m.device_groups_static()}
					/>
					<Stat
						tone={'ok'}
						value={group.memberCount}
						label={m.device_group_detail_devices()}
					/>

				</div>
			</div>
		</div>

 <MembersTab members={memberDevices} devices={deviceById} canAdd={can(Permission.ADD_DEVICE_TO_GROUP)} onadd={() => { selectedDeviceIds = []; deviceIdsText = ''; addDeviceDialogOpen = true; }} onremove={removeDevice} />
 {#if can(Permission.CREATE_ASSIGNMENT)}<Button variant="outline" onclick={() => goto(`/assignments?group=${groupId}`)}>Assign actions</Button>{/if}
 {#if can(Permission.DELETE_DEVICE_GROUP)}<Button variant="destructive" onclick={() => { deleteDialogOpen = true; }}>{m.common_delete()}</Button>{/if}
 {/if}
</PageShell>
<Dialog.Root bind:open={addDeviceDialogOpen}>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{m.device_group_detail_add_dialog_title()}</Dialog.Title>
			<Dialog.Description>{m.device_group_detail_add_dialog_description()}</Dialog.Description>
		</Dialog.Header>

		{#if can(Permission.LIST_DEVICES)}<ItemTablePicker
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
		</ItemTablePicker>{:else}<Input aria-label="Device ID" placeholder="Device ID" value={deviceIdsText} oninput={e => { deviceIdsText = e.currentTarget.value; selectedDeviceIds = parseIds(deviceIdsText); }} />{/if}

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (addDeviceDialogOpen = false)}>{m.common_cancel()}</Button>
			<Button onclick={addDevices} disabled={selectedDeviceIds.length === 0 || adding || !membershipReady}>{m.common_add()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<ConfirmDeleteDialog bind:open={deleteDialogOpen} title={m.device_groups_delete_dialog_title()} description={m.device_groups_delete_dialog_description({ name: group?.name ?? '' })} onconfirm={deleteGroup} />
