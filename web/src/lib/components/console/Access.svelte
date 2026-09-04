<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import {
		AssignRoleToUserRequestSchema,
		CreateRoleRequestSchema,
		DeleteRoleRequestSchema,
		GrantRolePermissionRequestSchema,
		ListPermissionsRequestSchema,
		ListRolesRequestSchema,
		ListUsersRequestSchema,
		Permission,
		RenameRoleRequestSchema,
		RevokeRoleFromUserRequestSchema,
		RevokeRolePermissionRequestSchema,
		RevokeUserSessionsRequestSchema,
		SetRoleDescriptionRequestSchema,
		type Role,
		type User
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage, logout } from '$lib/api';
	import { cursorHref, roleChanges } from '$lib/console';

	let { can, currentUser }: { can: (permission: Permission) => boolean; currentUser: User } = $props();
	let users = $state<User[]>([]);
	let roles = $state<Role[]>([]);
	let permissions = $state<Permission[]>([]);
	let permissionsLoaded = $state(false);
	let rolesLoadSucceeded = $state(false);
	let nextUsersToken = $state('');
	let nextRolesToken = $state('');
	let loading = $state(true);
	let createBusy = $state(false);
	let editBusy = $state(false);
	let assignmentBusy = $state(false);
	let rowBusy = $state('');
	let error = $state('');
	let notice = $state('');
	let roleName = $state('');
	let roleDescription = $state('');
	let rolePermissions = $state<Permission[]>([]);
	let selectedUser = $state('');
	let selectedRole = $state('');
	let editing = $state<Role>();
	let editName = $state('');
	let editDescription = $state('');
	let editPermissions = $state<Permission[]>([]);
	let editBaselineReady = $state(true);
	const usersToken = $derived(page.url.searchParams.get('usersCursor') ?? '');
	const rolesToken = $derived(page.url.searchParams.get('rolesCursor') ?? '');

	async function load(): Promise<boolean> {
		loading = true;
		error = '';
		rolesLoadSucceeded = !can(Permission.LIST_ROLES);
		const loaded = await Promise.allSettled([
			can(Permission.LIST_USERS) ? api.listUsers(create(ListUsersRequestSchema, { pageSize: 50, pageToken: usersToken })) : Promise.resolve(null),
			can(Permission.LIST_ROLES) ? api.listRoles(create(ListRolesRequestSchema, { pageSize: 50, pageToken: rolesToken })) : Promise.resolve(null),
			can(Permission.LIST_PERMISSIONS) ? api.listPermissions(create(ListPermissionsRequestSchema)) : Promise.resolve(null)
		]);
		const failures: string[] = [];
		if (loaded[0].status === 'fulfilled') {
			users = loaded[0].value?.users ?? [];
			nextUsersToken = loaded[0].value?.nextPageToken ?? '';
		} else failures.push(`Users: ${errorMessage(loaded[0].reason)}`);
		if (loaded[1].status === 'fulfilled') {
			roles = loaded[1].value?.roles ?? [];
			nextRolesToken = loaded[1].value?.nextPageToken ?? '';
			rolesLoadSucceeded = true;
		} else failures.push(`Roles: ${errorMessage(loaded[1].reason)}`);
		if (loaded[2].status === 'fulfilled') {
			permissions = loaded[2].value?.permissions ?? [];
			permissionsLoaded = can(Permission.LIST_PERMISSIONS) && loaded[2].value !== null;
		}
		else failures.push(`Permissions: ${errorMessage(loaded[2].reason)}`);
		error = failures.join(' ');
		loading = false;
		return failures.length === 0;
	}

	onMount(load);

	async function refreshNotice(message: string) {
		const refreshed = await load();
		notice = refreshed ? message : `${message} Refresh failed; the displayed state may be stale.`;
	}

	async function createRole() {
		createBusy = true;
		error = '';
		notice = '';
		try {
			await api.createRole(create(CreateRoleRequestSchema, { name: roleName, description: roleDescription, permissions: rolePermissions }));
			roleName = '';
			roleDescription = '';
			rolePermissions = [];
			await refreshNotice('Role created.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			createBusy = false;
		}
	}

	function edit(role: Role) {
		editing = role;
		editName = role.name;
		editDescription = role.description;
		editPermissions = [...role.permissions];
		editBaselineReady = true;
	}

	async function reloadEditing(): Promise<boolean> {
		await load();
		const current = roles.find((role) => role.id?.value === editing?.id?.value);
		if (rolesLoadSucceeded && current) {
			editing = current;
			editBaselineReady = true;
		}
		return rolesLoadSucceeded && Boolean(current);
	}

	async function saveRole() {
		if (!editing?.id || !editBaselineReady) return;
		editBusy = true;
		error = '';
		notice = '';
		const id = editing.id;
		const changes = roleChanges(editing, editName, editDescription, permissionsLoaded ? editPermissions : editing.permissions);
		try {
			if (changes.rename) {
				const response = await api.renameRole(create(RenameRoleRequestSchema, { id, name: editName }));
				if (response.role) editing = response.role;
			}
			if (changes.describe) {
				const response = await api.setRoleDescription(create(SetRoleDescriptionRequestSchema, { id, description: editDescription }));
				if (response.role) editing = response.role;
			}
			for (const permission of changes.grant) {
				const response = await api.grantRolePermission(create(GrantRolePermissionRequestSchema, { id, permission }));
				if (response.role) editing = response.role;
			}
			for (const permission of changes.revoke) {
				const response = await api.revokeRolePermission(create(RevokeRolePermissionRequestSchema, { id, permission }));
				if (response.role) editing = response.role;
			}
			const refreshed = await reloadEditing();
			notice = refreshed ? 'Role saved.' : 'Role saved. Refresh failed; the displayed state may be stale.';
		} catch (cause) {
			const mutationError = errorMessage(cause);
			editBaselineReady = false;
			const refreshed = await reloadEditing();
			error = refreshed ? mutationError : `${mutationError} The role may be partly updated and its latest state could not be loaded.`;
		} finally {
			editBusy = false;
		}
	}

	async function deleteRole(role: Role) {
		if (!role.id || !confirm(`Delete ${role.name}?`)) return;
		rowBusy = role.id.value;
		error = '';
		notice = '';
		try {
			await api.deleteRole(create(DeleteRoleRequestSchema, { id: role.id }));
			if (editing?.id?.value === role.id.value) editing = undefined;
			await refreshNotice('Role deleted.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			rowBusy = '';
		}
	}

	async function changeAssignment(assign: boolean) {
		assignmentBusy = true;
		error = '';
		notice = '';
		try {
			const values = { userId: { value: selectedUser }, roleId: { value: selectedRole } };
			if (assign) await api.assignRoleToUser(create(AssignRoleToUserRequestSchema, values));
			else await api.revokeRoleFromUser(create(RevokeRoleFromUserRequestSchema, values));
			selectedUser = '';
			selectedRole = '';
			await refreshNotice(assign ? 'Role assigned.' : 'Role revoked.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			assignmentBusy = false;
		}
	}

	async function revokeSessions(user: User) {
		if (!user.id || !confirm(`Revoke all sessions for ${user.email}?`)) return;
		rowBusy = user.id.value;
		error = '';
		notice = '';
		try {
			await api.revokeUserSessions(create(RevokeUserSessionsRequestSchema, { userId: user.id }));
			if (user.id.value === currentUser.id?.value) {
				await logout();
				await goto('/login');
				return;
			}
			await refreshNotice('User sessions revoked.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			rowBusy = '';
		}
	}
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title"><div><p class="eyebrow">Authorization</p><h1>Access control</h1></div></div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if notice}<p class="notice" role="status">{notice}</p>{/if}
	{#if can(Permission.CREATE_ROLE)}
		<form class="editor" onsubmit={(event) => { event.preventDefault(); createRole(); }}><fieldset disabled={createBusy}>
			<h2>Create role</h2>
			<label>Name<input bind:value={roleName} required maxlength="128" /></label>
			<label>Description<input bind:value={roleDescription} maxlength="1024" /></label>
			{#if permissionsLoaded}
				<label>Permissions<select multiple bind:value={rolePermissions}>{#each permissions as permission}<option value={permission}>{Permission[permission]}</option>{/each}</select></label>
			{/if}
			<button class="primary">Create role</button>
		</fieldset></form>
	{/if}
	{#if (can(Permission.ASSIGN_ROLE_TO_USER) || can(Permission.REVOKE_ROLE_FROM_USER)) && can(Permission.LIST_USERS) && can(Permission.LIST_ROLES)}
		<form onsubmit={(event) => { event.preventDefault(); changeAssignment((event.submitter as HTMLButtonElement).value === 'assign'); }}><fieldset disabled={assignmentBusy}>
			<label>User<select bind:value={selectedUser} required><option value="">Select user</option>{#each users as user}<option value={user.id?.value}>{user.email}</option>{/each}</select></label>
			<label>Role<select bind:value={selectedRole} required><option value="">Select role</option>{#each roles as role}<option value={role.id?.value}>{role.name}</option>{/each}</select></label>
			{#if can(Permission.ASSIGN_ROLE_TO_USER)}<button value="assign">Assign role</button>{/if}
			{#if can(Permission.REVOKE_ROLE_FROM_USER)}<button value="revoke">Revoke role</button>{/if}
		</fieldset></form>
	{/if}
</section>

{#if can(Permission.LIST_USERS)}
	<section class="card"><h2>Users</h2>
		{#if loading}<p role="status">Loading users…</p>{:else if users.length === 0}<p>No users.</p>{:else}
			<div class="table-wrap"><table><thead><tr><th>Email</th><th>Roles</th><th>Permissions</th><th></th></tr></thead><tbody>
				{#each users as user (user.id?.value)}
					<tr><td><strong>{user.displayName || user.email}</strong><small>{user.email}</small></td><td>{user.roles.map((role) => role.name).join(', ') || 'None'}</td><td>{user.permissions.length}</td><td>
						{#if can(Permission.REVOKE_USER_SESSIONS)}<button type="button" class="danger" onclick={() => revokeSessions(user)} disabled={Boolean(rowBusy)}>Revoke sessions</button>{/if}
					</td></tr>
				{/each}
			</tbody></table></div>
		{/if}
		<nav class="pagination" aria-label="User pages">
			{#if usersToken}<a class="button quiet" href={cursorHref(page.url, 'usersCursor', '')}>First page</a>{/if}
			{#if nextUsersToken}<a class="button" href={cursorHref(page.url, 'usersCursor', nextUsersToken)}>Next page</a>{/if}
		</nav>
	</section>
{/if}

{#if can(Permission.LIST_ROLES)}
	<section class="card"><h2>Roles</h2>
		{#if loading}<p role="status">Loading roles…</p>{:else if roles.length === 0}<p>No roles.</p>{:else}
			<div class="table-wrap"><table><thead><tr><th>Name</th><th>Description</th><th>Permissions</th><th></th></tr></thead><tbody>
				{#each roles as role (role.id?.value)}
					<tr><td>{role.name}</td><td>{role.description}</td><td>{role.permissions.length}</td><td class="row-actions">
						{#if can(Permission.UPDATE_ROLE)}<button type="button" class="quiet" onclick={() => edit(role)} disabled={editBusy || Boolean(rowBusy)}>Edit</button>{/if}
						{#if can(Permission.DELETE_ROLE)}<button type="button" class="danger" onclick={() => deleteRole(role)} disabled={Boolean(rowBusy)}>Delete</button>{/if}
					</td></tr>
				{/each}
			</tbody></table></div>
		{/if}
		<nav class="pagination" aria-label="Role pages">
			{#if rolesToken}<a class="button quiet" href={cursorHref(page.url, 'rolesCursor', '')}>First page</a>{/if}
			{#if nextRolesToken}<a class="button" href={cursorHref(page.url, 'rolesCursor', nextRolesToken)}>Next page</a>{/if}
		</nav>
	</section>
{/if}

{#if editing && can(Permission.UPDATE_ROLE)}
		<section class="card"><div class="section-title"><div><p class="eyebrow">Role editor</p><h2>{editing.name}</h2></div><button type="button" class="quiet" onclick={() => editing = undefined} disabled={editBusy}>Close</button></div>
		<form class="editor" onsubmit={(event) => { event.preventDefault(); saveRole(); }}><fieldset disabled={editBusy || !editBaselineReady}>
			<label>Name<input bind:value={editName} required maxlength="128" /></label>
			<label>Description<input bind:value={editDescription} maxlength="1024" /></label>
			{#if permissionsLoaded}
				<label>Permissions<select multiple bind:value={editPermissions}>{#each permissions as permission}<option value={permission}>{Permission[permission]}</option>{/each}</select></label>
			{/if}
			<button class="primary">Save role</button>
		</fieldset></form>
		{#if !editBaselineReady}<button type="button" onclick={reloadEditing}>Reload role before retry</button>{/if}
	</section>
{/if}
