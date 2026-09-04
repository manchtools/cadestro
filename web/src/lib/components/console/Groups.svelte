<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import {
		AddDeviceToGroupRequestSchema,
		CreateDeviceGroupRequestSchema,
		DeleteDeviceGroupRequestSchema,
		GetDeviceGroupRequestSchema,
		ListDeviceGroupsRequestSchema,
		ListDevicesRequestSchema,
		Permission,
		RemoveDeviceFromGroupRequestSchema,
		RenameDeviceGroupRequestSchema,
		SetDeviceGroupDescriptionRequestSchema,
		type Device,
		type DeviceGroup,
		type DeviceGroupMember
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage } from '$lib/api';
	import { collectPages, cursorHref, formatDate } from '$lib/console';

	let { can }: { can: (permission: Permission) => boolean } = $props();
	let groups = $state<DeviceGroup[]>([]);
	let allDevices = $state<Device[]>([]);
	let members = $state<DeviceGroupMember[]>([]);
	let selected = $state<DeviceGroup>();
	let totalCount = $state(0);
	let nextPageToken = $state('');
	let loading = $state(true);
	let detailLoading = $state(false);
	let creating = $state(false);
	let renameBusy = $state(false);
	let descriptionBusy = $state(false);
	let memberBusy = $state(false);
	let deleting = $state(false);
	let error = $state('');
	let detailError = $state('');
	let notice = $state('');
	let name = $state('');
	let description = $state('');
	let editName = $state('');
	let editDescription = $state('');
	let deviceID = $state('');
	const pageToken = $derived(page.url.searchParams.get('groupsCursor') ?? '');
	const groupID = $derived(page.url.searchParams.get('group') ?? '');

	function groupHref(id: string): string {
		const target = new URL(page.url);
		if (id) target.searchParams.set('group', id);
		else target.searchParams.delete('group');
		return `${target.pathname}${target.search}`;
	}

	async function load(): Promise<boolean> {
		loading = true;
		error = '';
		const requests = [
			can(Permission.LIST_DEVICE_GROUPS)
				? api.listDeviceGroups(create(ListDeviceGroupsRequestSchema, { pageSize: 50, pageToken }))
				: Promise.resolve(null),
			can(Permission.LIST_DEVICES) && can(Permission.ADD_DEVICE_TO_GROUP)
				? collectPages(async (token) => {
					const response = await api.listDevices(create(ListDevicesRequestSchema, { pageSize: 100, pageToken: token }));
					return { items: response.devices, nextPageToken: response.nextPageToken };
				})
				: Promise.resolve(null)
		] as const;
		const loaded = await Promise.allSettled(requests);
		const failures: string[] = [];
		if (loaded[0].status === 'fulfilled') {
			groups = loaded[0].value?.groups ?? [];
			totalCount = loaded[0].value?.totalCount ?? 0;
			nextPageToken = loaded[0].value?.nextPageToken ?? '';
		} else failures.push(`Groups: ${errorMessage(loaded[0].reason)}`);
		if (loaded[1].status === 'fulfilled') allDevices = loaded[1].value ?? [];
		else failures.push(`Device choices: ${errorMessage(loaded[1].reason)}`);
		error = failures.join(' ');
		loading = false;
		return failures.length === 0;
	}

	async function loadDetail(): Promise<boolean> {
		if (!groupID) return true;
		selected = groups.find((group) => group.id?.value === groupID) ?? selected;
		if (selected && editName === '') {
			editName = selected.name;
			editDescription = selected.description;
		}
		if (!can(Permission.GET_DEVICE_GROUP)) return Boolean(selected);
		detailLoading = true;
		detailError = '';
		try {
			const response = await api.getDeviceGroup(create(GetDeviceGroupRequestSchema, { id: { value: groupID } }));
			selected = response.group;
			members = response.devices;
			if (response.group && editName === '') {
				editName = response.group.name;
				editDescription = response.group.description;
			}
			return true;
		} catch (cause) {
			detailError = errorMessage(cause);
			return false;
		} finally {
			detailLoading = false;
		}
	}

	onMount(async () => {
		await load();
		await loadDetail();
	});

	async function refreshed(message: string, withDetail = false) {
		const listOK = await load();
		const detailOK = withDetail ? await loadDetail() : true;
		notice = listOK && detailOK ? message : `${message} Refresh failed; the displayed state may be stale.`;
	}

	async function createGroup() {
		creating = true;
		error = '';
		notice = '';
		try {
			await api.createDeviceGroup(create(CreateDeviceGroupRequestSchema, { name, description }));
			name = '';
			description = '';
			await refreshed('Group created.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			creating = false;
		}
	}

	async function renameGroup() {
		if (!selected?.id) return;
		renameBusy = true;
		detailError = '';
		notice = '';
		try {
			const response = await api.renameDeviceGroup(create(RenameDeviceGroupRequestSchema, { id: selected.id, name: editName }));
			selected = response.group;
			await refreshed('Group renamed.', true);
		} catch (cause) {
			const mutationError = errorMessage(cause);
			const reloaded = await loadDetail();
			detailError = reloaded ? mutationError : `${mutationError} The current group could not be reloaded.`;
		} finally {
			renameBusy = false;
		}
	}

	async function describeGroup() {
		if (!selected?.id) return;
		descriptionBusy = true;
		detailError = '';
		notice = '';
		try {
			const response = await api.setDeviceGroupDescription(create(SetDeviceGroupDescriptionRequestSchema, { id: selected.id, description: editDescription }));
			selected = response.group;
			await refreshed('Group description saved.', true);
		} catch (cause) {
			const mutationError = errorMessage(cause);
			const reloaded = await loadDetail();
			detailError = reloaded ? mutationError : `${mutationError} The current group could not be reloaded.`;
		} finally {
			descriptionBusy = false;
		}
	}

	async function addMember() {
		if (!selected?.id) return;
		memberBusy = true;
		detailError = '';
		notice = '';
		try {
			await api.addDeviceToGroup(create(AddDeviceToGroupRequestSchema, { groupId: selected.id, deviceId: { value: deviceID } }));
			deviceID = '';
			await refreshed('Device added to group.', true);
		} catch (cause) {
			detailError = errorMessage(cause);
		} finally {
			memberBusy = false;
		}
	}

	async function removeMember(member: DeviceGroupMember) {
		if (!selected?.id || !member.deviceId || !confirm(`Remove ${member.hostname} from ${selected.name}?`)) return;
		memberBusy = true;
		detailError = '';
		notice = '';
		try {
			await api.removeDeviceFromGroup(create(RemoveDeviceFromGroupRequestSchema, { groupId: selected.id, deviceId: member.deviceId }));
			await refreshed('Device removed from group.', true);
		} catch (cause) {
			detailError = errorMessage(cause);
		} finally {
			memberBusy = false;
		}
	}

	async function deleteGroup() {
		if (!selected?.id || !confirm(`Delete ${selected.name}?`)) return;
		deleting = true;
		detailError = '';
		notice = '';
		try {
			await api.deleteDeviceGroup(create(DeleteDeviceGroupRequestSchema, { id: selected.id }));
			await goto(groupHref(''));
		} catch (cause) {
			detailError = errorMessage(cause);
		} finally {
			deleting = false;
		}
	}
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title">
		<div><p class="eyebrow">Targeting</p><h1>Device groups</h1></div>
		{#if can(Permission.LIST_DEVICE_GROUPS)}<span>{totalCount} groups</span>{/if}
	</div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if notice}<p class="notice" role="status">{notice}</p>{/if}
	{#if can(Permission.CREATE_DEVICE_GROUP)}
		<form onsubmit={(event) => { event.preventDefault(); createGroup(); }}><fieldset disabled={creating}>
			<label>Name<input bind:value={name} required maxlength="255" /></label>
			<label>Description<input bind:value={description} maxlength="1024" /></label>
			<button class="primary">Create group</button>
		</fieldset>
		</form>
	{/if}
	{#if can(Permission.LIST_DEVICE_GROUPS)}
		{#if loading}
			<p role="status">Loading groups…</p>
		{:else if groups.length === 0}
			<p>No device groups.</p>
		{:else}
			<div class="table-wrap"><table><thead><tr><th>Name</th><th>Description</th><th>Members</th><th></th></tr></thead><tbody>
				{#each groups as group (group.id?.value)}
					<tr><td>{group.name}</td><td>{group.description}</td><td>{group.memberCount}</td><td>
						{#if can(Permission.GET_DEVICE_GROUP) || can(Permission.RENAME_DEVICE_GROUP) || can(Permission.UPDATE_DEVICE_GROUP_DESCRIPTION) || can(Permission.DELETE_DEVICE_GROUP) || can(Permission.ADD_DEVICE_TO_GROUP)}
							<a class="button quiet" href={groupHref(group.id?.value ?? '')}>Details</a>
						{/if}
					</td></tr>
				{/each}
			</tbody></table></div>
		{/if}
		<nav class="pagination" aria-label="Device group pages">
			{#if pageToken}<a class="button quiet" href={cursorHref(page.url, 'groupsCursor', '', ['group'])}>First page</a>{/if}
			{#if nextPageToken}<a class="button" href={cursorHref(page.url, 'groupsCursor', nextPageToken, ['group'])}>Next page</a>{/if}
		</nav>
	{/if}
</section>

{#if groupID && selected}
	<section class="card" aria-busy={detailLoading}>
		<div class="section-title">
			<div><p class="eyebrow">Group detail</p><h2>{selected?.name ?? groupID}</h2></div>
			<a class="button quiet" href={groupHref('')}>Close</a>
		</div>
		{#if detailLoading}<p role="status">Loading group…</p>{/if}
		{#if detailError}<p class="error banner" role="alert">{detailError}</p>{/if}
		{#if selected}
			{#if can(Permission.RENAME_DEVICE_GROUP)}
				<form onsubmit={(event) => { event.preventDefault(); renameGroup(); }}><fieldset disabled={renameBusy}><label>Name<input bind:value={editName} required maxlength="255" /></label><button class="primary">Save name</button></fieldset></form>
			{/if}
			{#if can(Permission.UPDATE_DEVICE_GROUP_DESCRIPTION)}
				<form onsubmit={(event) => { event.preventDefault(); describeGroup(); }}><fieldset disabled={descriptionBusy}><label>Description<input bind:value={editDescription} maxlength="1024" /></label><button class="primary">Save description</button></fieldset></form>
			{/if}
			{#if can(Permission.ADD_DEVICE_TO_GROUP) && can(Permission.LIST_DEVICES)}
				<form onsubmit={(event) => { event.preventDefault(); addMember(); }}><fieldset disabled={memberBusy}>
					<label>Device<select bind:value={deviceID} required><option value="">Select device</option>{#each allDevices.filter((device) => !members.some((member) => member.deviceId?.value === device.id?.value)) as device}<option value={device.id?.value}>{device.hostname}</option>{/each}</select></label>
					<button>Add member</button>
				</fieldset></form>
			{/if}
			{#if can(Permission.GET_DEVICE_GROUP)}
				<h3>Members</h3>
				{#if members.length === 0}<p>No devices in this group.</p>{:else}<div class="table-wrap"><table><thead><tr><th>Hostname</th><th>Agent</th><th>Last seen</th><th></th></tr></thead><tbody>
					{#each members as member (member.deviceId?.value)}<tr><td>{member.hostname}</td><td>{member.agentVersion}</td><td>{formatDate(member.lastSeenAt)}</td><td>{#if can(Permission.REMOVE_DEVICE_FROM_GROUP)}<button type="button" class="danger" onclick={() => removeMember(member)} disabled={memberBusy}>Remove</button>{/if}</td></tr>{/each}
				</tbody></table></div>{/if}
			{/if}
			{#if can(Permission.DELETE_DEVICE_GROUP)}<button type="button" class="danger" onclick={deleteGroup} disabled={deleting}>Delete group</button>{/if}
		{/if}
	</section>
{/if}
