<script lang="ts">
 import { onMount } from 'svelte';
 import { page } from '$app/state';
 import { pushState } from '$app/navigation';
 import { toast } from 'svelte-sonner';
 import { parseIds } from '$lib/id-list';
 import { addMemberships } from '$lib/membership';
 import { api } from '$lib/api';
 import { Permission, type Device, type DeviceGroup, type ManagedAction, type Assignment } from '$contract/cadestro/v1/control_pb';
 import { AssignmentTargetType } from '$contract/cadestro/v1/common_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { collectPages, formatDate } from '$lib/console';
 import { getLocalizedError } from '$lib/errors';
 import { Button } from '$lib/components/ui/button';
 import { Input } from '$lib/components/ui/input';
 import { Label } from '$lib/components/ui/label';
 import * as Table from '$lib/components/ui/table';
 import ItemTablePicker from '$lib/components/item-table-picker.svelte';
 import PageShell from '$lib/components/page-shell.svelte';
 import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
 import ActionSheet from './action-sheet.svelte';
 import CarriedStage from './carried-stage.svelte';
 import { deviceTone } from '../devices/fleet-model';
 const { can } = consoleContext();
 let devices = $state<Device[]>([]);
 let groups = $state<DeviceGroup[]>([]);
 let actions = $state<ManagedAction[]>([]);
 let assignments = $state<Assignment[]>([]);
 let loading = $state(true);
 let saving = $state(false);
 let assignmentReady = $state(true);
 let error = $state('');
 let failures = $state<{ targetId: string; error: string }[]>([]);
 const mode = $derived(page.url.searchParams.has('group') ? 'groups' : 'devices');
 const targetIds = $derived(parseIds(page.url.searchParams.get(mode === 'groups' ? 'group' : 'devices') ?? ''));
 const actionId = $derived(page.url.searchParams.get('action') ?? '');
 const selectedTargets = $derived(targetIds.map(id => { const device = devices.find(device => device.id?.value === id); const group = groups.find(group => group.id?.value === id); return { id, hostname: mode === 'groups' ? group?.name ?? id : device?.hostname ?? id, tone: device ? deviceTone(device) : 'idle' as const }; }));
 function update(key: string, value: string) { const url = new URL(page.url); url.searchParams.set(key, value); pushState(url, {}); }
 function changeMode(next: string) { const url = new URL(page.url); url.searchParams.delete('devices'); url.searchParams.delete('group'); url.searchParams.set(next === 'groups' ? 'group' : 'devices', ''); pushState(url, {}); }
 bindBuilderContext('assign', () => saving || !can(Permission.CREATE_ASSIGNMENT) ? null : { title: actions.find(action => action.id?.value === actionId)?.name ?? 'Assign actions', route: page.url.pathname + page.url.search, dirty: !!actionId || !!targetIds.length, valid: assignmentReady && !!actionId && !!targetIds.length, commitLabel: 'Assign', subtext: `${targetIds.length} ${mode} selected`, onCommit: () => void commit(), onCancel: () => { update('action', ''); update(mode === 'groups' ? 'group' : 'devices', ''); } });
 async function load() { loading = true; error = ''; const failed: string[] = [];
 const results = await Promise.allSettled([
  can(Permission.LIST_DEVICES) ? collectPages(async pageToken => { const r = await api.listDevices({ pageSize: 100, pageToken }); return { items: r.devices, nextPageToken: r.nextPageToken }; }) : Promise.resolve([]),
  can(Permission.LIST_DEVICE_GROUPS) ? collectPages(async pageToken => { const r = await api.listDeviceGroups({ pageSize: 100, pageToken }); return { items: r.groups, nextPageToken: r.nextPageToken }; }) : Promise.resolve([]),
  can(Permission.LIST_ACTIONS) ? collectPages(async pageToken => { const r = await api.listActions({ pageSize: 100, pageToken }); return { items: r.actions, nextPageToken: r.nextPageToken }; }) : Promise.resolve([]),
  can(Permission.LIST_ASSIGNMENTS) ? api.listAssignments({}).then(response => response.assignments) : Promise.resolve([])
 ]);
 if(results[0].status === 'fulfilled') devices = results[0].value; else failed.push(getLocalizedError(results[0].reason));
 if(results[1].status === 'fulfilled') groups = results[1].value; else failed.push(getLocalizedError(results[1].reason));
 if(results[2].status === 'fulfilled') actions = results[2].value; else failed.push(getLocalizedError(results[2].reason));
 if(results[3].status === 'fulfilled') { assignments = results[3].value; if (can(Permission.LIST_ASSIGNMENTS)) { assignmentReady = true; if (failures.length) { const assigned = assignments.filter(assignment => assignment.actionId?.value === actionId && assignment.targetType === (mode === 'groups' ? AssignmentTargetType.DEVICE_GROUP : AssignmentTargetType.DEVICE)).map(assignment => assignment.targetId?.value); update(mode === 'groups' ? 'group' : 'devices', targetIds.filter(id => !assigned.includes(id)).join(',')); } } } else failed.push(getLocalizedError(results[3].reason));
 error = failed.join(' '); loading = false;
 }
 async function commit() {
  if (!can(Permission.CREATE_ASSIGNMENT) || !actionId || !targetIds.length || !assignmentReady || saving) return;
  saving = true; failures = [];
  const targetType = mode === 'groups' ? AssignmentTargetType.DEVICE_GROUP : AssignmentTargetType.DEVICE;
  const result = await addMemberships(targetIds, id => api.createAssignment({ actionId: { value: actionId }, targetType, targetId: { value: id } }), async () => {
   if (!can(Permission.LIST_ASSIGNMENTS)) throw new Error('Assignments cannot be refreshed');
   assignments = (await api.listAssignments({})).assignments;
   return assignments.filter(assignment => assignment.actionId?.value === actionId && assignment.targetType === targetType).map(assignment => assignment.targetId?.value ?? '');
  });
  update(mode === 'groups' ? 'group' : 'devices', result.remaining.join(','));
  assignmentReady = result.ready;
  if (result.error) { failures = result.remaining.map(targetId => ({ targetId, error: getLocalizedError(result.error) })); toast.error(`${getLocalizedError(result.error)}${result.ready ? '' : ' The result is uncertain; verify assignments before starting another assignment.'}`); }
  else toast.success('Actions assigned');
  saving = false; await load();
 }

 async function remove(assignment: Assignment) { if (!can(Permission.DELETE_ASSIGNMENT)) return; try { await api.deleteAssignment({ id: assignment.id }); await load(); } catch(cause) { toast.error(getLocalizedError(cause)); } }
 onMount(load);
</script>

<PageShell contentClass="space-y-4">
 {#snippet header()}<div class="flex flex-wrap items-center gap-3"><h1 class="text-2xl font-bold">Assign actions</h1><Button class="ml-auto" variant="outline" disabled={loading} onclick={load}>Refresh</Button></div>{/snippet}
 {#if error}<p role="alert" class="rounded-lg border border-crit bg-crit-soft p-3 text-sm text-crit">{error}</p>{/if}
 {#if can(Permission.CREATE_ASSIGNMENT)}
 <div class="flex gap-2"><Button variant={mode === 'devices' ? 'default' : 'outline'} onclick={() => changeMode('devices')}>Devices</Button><Button variant={mode === 'groups' ? 'default' : 'outline'} onclick={() => changeMode('groups')}>Device groups</Button></div>
 <div class="grid min-h-0 grid-cols-1 overflow-hidden rounded-xl border border-border shadow-plate md:grid-cols-[1fr_18rem]">
  <div><CarriedStage label={`${targetIds.length} selected`} devices={selectedTargets} {loading} {failures} />
   <div class="bg-sunken p-4">
    {#if mode === 'devices' && can(Permission.LIST_DEVICES) || mode === 'groups' && can(Permission.LIST_DEVICE_GROUPS)}
     <ItemTablePicker items={mode === 'devices' ? devices.map(device => ({ id: device.id?.value ?? '', name: device.hostname })) : groups.map(group => ({ id: group.id?.value ?? '', name: group.name }))} bind:selected={() => targetIds, value => update(mode === 'groups' ? 'group' : 'devices', value.join(','))} searchFilter={(item, query) => item.name.toLowerCase().includes(query.toLowerCase())} emptyMessage="No targets available">
      {#snippet headerRow()}<Table.Head>{mode === 'groups' ? 'Group' : 'Device'}</Table.Head>{/snippet}{#snippet itemRow(item)}<Table.Cell>{item.name}</Table.Cell>{/snippet}
     </ItemTablePicker>
    {:else}<Label for="assignment-target">Target IDs</Label><Input id="assignment-target" value={page.url.searchParams.get(mode === 'groups' ? 'group' : 'devices') ?? ''} oninput={event => update(mode === 'groups' ? 'group' : 'devices', event.currentTarget.value)} />{/if}
   </div>
  </div>
  {#if can(Permission.LIST_ACTIONS)}<ActionSheet {actions} {loading} selectedId={actionId} onselect={id => update('action', id)} />{:else}<div class="space-y-2 bg-surface p-4"><Label for="assignment-action">Action ID</Label><Input id="assignment-action" value={actionId} oninput={event => update('action', event.currentTarget.value)} /></div>{/if}
 </div>
 {/if}
 {#if can(Permission.LIST_ASSIGNMENTS)}
 <section class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate"><div class="border-b px-4 py-3 font-mono text-xs uppercase text-faint">Assignments</div>
 {#each assignments.filter(assignment => !actionId || assignment.actionId?.value === actionId) as assignment (assignment.id?.value)}<div class="flex flex-wrap items-center gap-3 border-b border-hair px-4 py-2 last:border-b-0"><span class="text-sm font-medium">{assignment.actionName}</span><span class="font-mono text-xs text-faint">{assignment.targetName}</span><span class="ml-auto text-xs text-muted-foreground">{formatDate(assignment.createdAt)}</span>{#if can(Permission.DELETE_ASSIGNMENT)}<Button variant="ghost" size="sm" onclick={() => remove(assignment)}>Remove</Button>{/if}</div>{:else}<p class="px-4 py-8 text-center text-sm text-muted-foreground">No assignments.</p>{/each}
 </section>{/if}
</PageShell>
