<script lang="ts">
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { AssignmentTargetType } from '$contract/cadestro/v1/common_pb';
	import {
		CreateAssignmentRequestSchema,
		DeleteAssignmentRequestSchema,
		ListActionsRequestSchema,
		ListAssignmentsRequestSchema,
		ListDeviceGroupsRequestSchema,
		ListDevicesRequestSchema,
		Permission,
		type Assignment,
		type Device,
		type DeviceGroup,
		type ManagedAction
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage } from '$lib/api';
	import { collectPages, formatDate } from '$lib/console';

	let { can }: { can: (permission: Permission) => boolean } = $props();
	let assignments = $state<Assignment[]>([]);
	let actions = $state<ManagedAction[]>([]);
	let devices = $state<Device[]>([]);
	let groups = $state<DeviceGroup[]>([]);
	let loading = $state(true);
	let creating = $state(false);
	let deleting = $state('');
	let error = $state('');
	let notice = $state('');
	let actionID = $state('');
	let targetType = $state<AssignmentTargetType>(AssignmentTargetType.UNSPECIFIED);
	let targetID = $state('');

	async function load(): Promise<boolean> {
		loading = true;
		error = '';
		const requests = [
			can(Permission.LIST_ASSIGNMENTS) ? api.listAssignments(create(ListAssignmentsRequestSchema)) : Promise.resolve(null),
			can(Permission.CREATE_ASSIGNMENT) && can(Permission.LIST_ACTIONS)
				? collectPages(async (pageToken) => {
					const response = await api.listActions(create(ListActionsRequestSchema, { pageSize: 100, pageToken }));
					return { items: response.actions, nextPageToken: response.nextPageToken };
				})
				: Promise.resolve(null),
			can(Permission.CREATE_ASSIGNMENT) && can(Permission.LIST_DEVICES)
				? collectPages(async (pageToken) => {
					const response = await api.listDevices(create(ListDevicesRequestSchema, { pageSize: 100, pageToken }));
					return { items: response.devices, nextPageToken: response.nextPageToken };
				})
				: Promise.resolve(null),
			can(Permission.CREATE_ASSIGNMENT) && can(Permission.LIST_DEVICE_GROUPS)
				? collectPages(async (pageToken) => {
					const response = await api.listDeviceGroups(create(ListDeviceGroupsRequestSchema, { pageSize: 100, pageToken }));
					return { items: response.groups, nextPageToken: response.nextPageToken };
				})
				: Promise.resolve(null)
		] as const;
		const loaded = await Promise.allSettled(requests);
		const failures: string[] = [];
		if (loaded[0].status === 'fulfilled') assignments = loaded[0].value?.assignments ?? [];
		else failures.push(`Assignments: ${errorMessage(loaded[0].reason)}`);
		if (loaded[1].status === 'fulfilled') actions = loaded[1].value ?? [];
		else failures.push(`Actions: ${errorMessage(loaded[1].reason)}`);
		if (loaded[2].status === 'fulfilled') devices = loaded[2].value ?? [];
		else failures.push(`Devices: ${errorMessage(loaded[2].reason)}`);
		if (loaded[3].status === 'fulfilled') groups = loaded[3].value ?? [];
		else failures.push(`Groups: ${errorMessage(loaded[3].reason)}`);
		error = failures.join(' ');
		loading = false;
		return failures.length === 0;
	}

	onMount(() => {
		targetType = can(Permission.LIST_DEVICES) ? AssignmentTargetType.DEVICE : AssignmentTargetType.DEVICE_GROUP;
		load();
	});

	async function createAssignment() {
		creating = true;
		error = '';
		notice = '';
		try {
			await api.createAssignment(create(CreateAssignmentRequestSchema, {
				actionId: { value: actionID },
				targetType,
				targetId: { value: targetID }
			}));
			actionID = '';
			targetID = '';
			const refreshed = await load();
			notice = refreshed ? 'Assignment created.' : 'Assignment created. Refresh failed; the displayed state may be stale.';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			creating = false;
		}
	}

	async function deleteAssignment(assignment: Assignment) {
		if (!assignment.id || !confirm(`Delete the ${assignment.actionName} assignment for ${assignment.targetName}?`)) return;
		deleting = assignment.id.value;
		error = '';
		notice = '';
		try {
			await api.deleteAssignment(create(DeleteAssignmentRequestSchema, { id: assignment.id }));
			const refreshed = await load();
			notice = refreshed ? 'Assignment deleted.' : 'Assignment deleted. Refresh failed; the displayed state may be stale.';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			deleting = '';
		}
	}
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title"><div><p class="eyebrow">Delivery</p><h1>Assignments</h1></div></div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if notice}<p class="notice" role="status">{notice}</p>{/if}
	{#if can(Permission.CREATE_ASSIGNMENT) && can(Permission.LIST_ACTIONS) && (can(Permission.LIST_DEVICES) || can(Permission.LIST_DEVICE_GROUPS))}
		<form onsubmit={(event) => { event.preventDefault(); createAssignment(); }}>
			<fieldset disabled={creating}>
				<label>Action<select bind:value={actionID} required><option value="">Select action</option>{#each actions as action}<option value={action.id?.value}>{action.name}</option>{/each}</select></label>
				<label>Target type<select bind:value={targetType} onchange={() => targetID = ''}>{#if can(Permission.LIST_DEVICES)}<option value={AssignmentTargetType.DEVICE}>Device</option>{/if}{#if can(Permission.LIST_DEVICE_GROUPS)}<option value={AssignmentTargetType.DEVICE_GROUP}>Group</option>{/if}</select></label>
				<label>Target<select bind:value={targetID} required><option value="">Select target</option>{#if Number(targetType) === AssignmentTargetType.DEVICE}{#each devices as device}<option value={device.id?.value}>{device.hostname}</option>{/each}{:else}{#each groups as group}<option value={group.id?.value}>{group.name}</option>{/each}{/if}</select></label>
				<button class="primary">Create assignment</button>
			</fieldset>
		</form>
	{/if}
	{#if can(Permission.LIST_ASSIGNMENTS)}
		{#if loading}<p role="status">Loading assignments…</p>{:else if assignments.length === 0}<p>No assignments.</p>{:else}
			<div class="table-wrap"><table><thead><tr><th>Action</th><th>Target</th><th>Type</th><th>Created</th><th></th></tr></thead><tbody>
				{#each assignments as assignment (assignment.id?.value)}
					<tr><td>{assignment.actionName}</td><td>{assignment.targetName}</td><td>{AssignmentTargetType[assignment.targetType]}</td><td>{formatDate(assignment.createdAt)}</td><td>{#if can(Permission.DELETE_ASSIGNMENT)}<button type="button" class="danger" onclick={() => deleteAssignment(assignment)} disabled={Boolean(deleting)}>Delete</button>{/if}</td></tr>
				{/each}
			</tbody></table></div>
		{/if}
	{/if}
</section>
