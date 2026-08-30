<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
	import { ActionScheduleSchema, ActionType, PackageParamsSchema, ShellParamsSchema, UpdateParamsSchema } from '$contract/cadestro/v1/actions_pb';
	import { AssignmentTargetType, ComplianceStatus, DesiredState, DeviceStatus } from '$contract/cadestro/v1/common_pb';
	import {
		AddDeviceToGroupRequestSchema,
		CreateActionRequestSchema,
		CreateAssignmentRequestSchema,
		CreateDeviceGroupRequestSchema,
		CreateTokenRequestSchema,
		DeleteActionRequestSchema,
		DeleteAssignmentRequestSchema,
		GetDeviceComplianceRequestSchema,
		ListActionsRequestSchema,
		ListAssignmentsRequestSchema,
		ListDeviceGroupsRequestSchema,
		ListDevicesRequestSchema,
		ListExecutionResultsRequestSchema,
		ListTokensRequestSchema,
		RevokeUserSessionsRequestSchema,
		ListUsersRequestSchema,
		ListRolesRequestSchema,
		ListPermissionsRequestSchema,
		CreateRoleRequestSchema,
		UpdateRoleRequestSchema,
		DeleteRoleRequestSchema,
		AssignRoleToUserRequestSchema,
		RevokeRoleFromUserRequestSchema,
		Permission,
		type Assignment,
		type ComplianceCheckResult,
		type Device,
		type DeviceGroup,
		type ExecutionResult,
		type ManagedAction,
		type RegistrationToken,
		type User,
		type Role
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage, logout } from '$lib/api';
	import { readSession } from '$lib/session';

	let devices = $state<Device[]>([]);
	let actions = $state<ManagedAction[]>([]);
	let groups = $state<DeviceGroup[]>([]);
	let assignments = $state<Assignment[]>([]);
	let tokens = $state<RegistrationToken[]>([]);
	let results = $state<ExecutionResult[]>([]);
	let compliance = $state<ComplianceCheckResult[]>([]);
	let currentUser = $state<User | undefined>();
	let users = $state<User[]>([]);
	let roles = $state<Role[]>([]);
	let permissions = $state<Permission[]>([]);
	let selectedDevice = $state('');
	let error = $state('');
	let busy = $state(false);
	let revealedToken = $state('');
	let revealedPin = $state('');
	let roleName = $state('');
	let roleDescription = $state('');
	let rolePermissions = $state<Permission[]>([]);
	let selectedRole = $state('');
	let selectedUser = $state('');
	let editingRole = $state<Role | undefined>();

	function can(permission: Permission): boolean {
		return currentUser?.permissions.includes(permission) ?? false;
	}

	function canAny(...permissions: Permission[]): boolean {
		return permissions.some((permission) => can(permission));
	}

	let tokenName = $state('Enrollment');
	let tokenExpiry = $state(new Date(Date.now() + 7 * 86400_000).toISOString().slice(0, 16));
	let groupName = $state('');
	let membershipGroup = $state('');
	let membershipDevice = $state('');
	let actionName = $state('');
	let actionDescription = $state('');
	let actionType = $state<'package' | 'update' | 'shell'>('package');
	let packageName = $state('');
	let packageVersion = $state('');
	let packageRemove = $state(false);
	let shellScript = $state('');
	let detectionScript = $state('');
	let complianceAction = $state(false);
	let intervalHours = $state(0);
	let assignmentAction = $state('');
	let assignmentTargetType = $state<AssignmentTargetType>(AssignmentTargetType.DEVICE);
	let assignmentTarget = $state('');

	function formatDate(value: Parameters<typeof timestampDate>[0] | undefined): string {
		return value ? timestampDate(value).toLocaleString() : 'Never';
	}

	function actionTypeName(type: ActionType): string {
		return type === ActionType.PACKAGE ? 'Package' : type === ActionType.UPDATE ? 'System update' : 'Shell';
	}

	async function load() {
		if (!readSession()) {
			await goto('/login');
			return;
		}
		error = '';
		try {
			currentUser = (await api.getCurrentUser({})).user;
			const [devicePage, actionPage, groupPage, assignmentPage, tokenPage, userPage, rolePage, permissionPage] = await Promise.all([
				can(Permission.LIST_DEVICES) ? api.listDevices(create(ListDevicesRequestSchema, { pageSize: 100 })) : null,
				can(Permission.LIST_ACTIONS) ? api.listActions(create(ListActionsRequestSchema, { pageSize: 100 })) : null,
				can(Permission.LIST_DEVICE_GROUPS) ? api.listDeviceGroups(create(ListDeviceGroupsRequestSchema, { pageSize: 100 })) : null,
				can(Permission.LIST_ASSIGNMENTS) ? api.listAssignments(create(ListAssignmentsRequestSchema, {})) : null,
				can(Permission.LIST_TOKENS) ? api.listTokens(create(ListTokensRequestSchema, { pageSize: 100 })) : null,
				can(Permission.LIST_USERS) ? api.listUsers(create(ListUsersRequestSchema, { pageSize: 100 })) : null,
				can(Permission.LIST_ROLES) ? api.listRoles(create(ListRolesRequestSchema, { pageSize: 100 })) : null,
				can(Permission.LIST_PERMISSIONS) ? api.listPermissions(create(ListPermissionsRequestSchema, {})) : null
			]);
			devices = devicePage?.devices ?? [];
			actions = actionPage?.actions ?? [];
			groups = groupPage?.groups ?? [];
			assignments = assignmentPage?.assignments ?? [];
			tokens = tokenPage?.tokens ?? [];
			users = userPage?.users ?? [];
			roles = rolePage?.roles ?? [];
			permissions = permissionPage?.permissions ?? [];
		} catch (cause) {
			error = errorMessage(cause);
		}
	}

	onMount(load);

	async function run(task: () => Promise<unknown>) {
		busy = true;
		error = '';
		try {
			await task();
			await load();
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			busy = false;
		}
	}

	async function createToken() {
		await run(async () => {
			const response = await api.createToken(create(CreateTokenRequestSchema, {
				name: tokenName,
				expiresAt: timestampFromDate(new Date(tokenExpiry))
			}));
			revealedToken = response.token?.value ?? '';
			revealedPin = response.caFingerprintPin;
		});
	}

	async function createGroup() {
		await run(() => api.createDeviceGroup(create(CreateDeviceGroupRequestSchema, { name: groupName })));
		groupName = '';
	}

	async function addGroupMember() {
		await run(() => api.addDeviceToGroup(create(AddDeviceToGroupRequestSchema, {
			groupId: { value: membershipGroup },
			deviceId: { value: membershipDevice }
		})));
	}

	async function createAction() {
		const schedule = create(ActionScheduleSchema, { intervalHours, runOnAssign: true });
		const request = create(CreateActionRequestSchema, {
			name: actionName,
			description: actionDescription,
			type: ActionType.PACKAGE,
			desiredState: packageRemove ? DesiredState.ABSENT : DesiredState.PRESENT,
			schedule,
			params: { case: 'package', value: create(PackageParamsSchema, { name: packageName, version: packageVersion }) }
		});
		if (actionType === 'update') {
			request.type = ActionType.UPDATE;
			request.desiredState = DesiredState.PRESENT;
			request.params = { case: 'update', value: create(UpdateParamsSchema) };
		}
		if (actionType === 'shell') {
			request.type = ActionType.SHELL;
			request.desiredState = DesiredState.PRESENT;
			request.params = {
				case: 'shell',
				value: create(ShellParamsSchema, { script: shellScript, detectionScript, isCompliance: complianceAction })
			};
		}
		await run(() => api.createAction(request));
		actionName = '';
		actionDescription = '';
		packageName = '';
		packageVersion = '';
		shellScript = '';
		detectionScript = '';
	}

	async function createAssignment() {
		await run(() => api.createAssignment(create(CreateAssignmentRequestSchema, {
			actionId: { value: assignmentAction },
			targetType: assignmentTargetType,
			targetId: { value: assignmentTarget }
		})));
	}

	async function inspectDevice(deviceID: string) {
		selectedDevice = deviceID;
		error = '';
		try {
			const [history, current] = await Promise.all([
				can(Permission.LIST_EXECUTION_RESULTS) ? api.listExecutionResults(create(ListExecutionResultsRequestSchema, { deviceId: { value: deviceID }, pageSize: 50 })) : null,
				can(Permission.GET_DEVICE_COMPLIANCE) ? api.getDeviceCompliance(create(GetDeviceComplianceRequestSchema, { deviceId: { value: deviceID } })) : null
			]);
			results = history?.results ?? [];
			compliance = current?.checks ?? [];
		} catch (cause) {
			error = errorMessage(cause);
		}
	}

	async function signOut() {
		await logout();
		await goto('/login');
	}

	async function revokeSessions(user: User) {
		if (!user.id) return;
		await run(async () => {
			await api.revokeUserSessions(create(RevokeUserSessionsRequestSchema, { userId: user.id }));
			if (user.id?.value === currentUser?.id?.value) await signOut();
		});
	}

	async function createRole() {
		await run(() => api.createRole(create(CreateRoleRequestSchema, { name: roleName, description: roleDescription, permissions: rolePermissions })));
		roleName = '';
		roleDescription = '';
		rolePermissions = [];
	}

	async function assignRole() {
		await run(() => api.assignRoleToUser(create(AssignRoleToUserRequestSchema, { userId: { value: selectedUser }, roleId: { value: selectedRole } })));
	}

	async function updateRole() {
		const id = editingRole?.id;
		if (!id) return;
		await run(() => api.updateRole(create(UpdateRoleRequestSchema, { id, name: roleName, description: roleDescription, permissions: rolePermissions })));
		editingRole = undefined;
	}

	async function deleteRole(role: Role) {
		if (!role.id) return;
		await run(() => api.deleteRole(create(DeleteRoleRequestSchema, { id: role.id })));
	}

	async function revokeRole() {
		await run(() => api.revokeRoleFromUser(create(RevokeRoleFromUserRequestSchema, { userId: { value: selectedUser }, roleId: { value: selectedRole } })));
	}
</script>

<header>
	<div><strong>Cadestro</strong><span>Linux management core</span></div>
	<nav><button class="quiet" onclick={load} disabled={busy}>Refresh</button><button class="quiet" onclick={signOut}>Sign out</button></nav>
</header>

	<main class="stack">
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}

	{#if canAny(Permission.LIST_USERS, Permission.LIST_ROLES, Permission.LIST_PERMISSIONS, Permission.CREATE_ROLE, Permission.UPDATE_ROLE, Permission.DELETE_ROLE, Permission.ASSIGN_ROLE_TO_USER, Permission.REVOKE_ROLE_FROM_USER, Permission.REVOKE_USER_SESSIONS)}
		<section class="card">
			<div class="section-title"><div><p class="eyebrow">Access control</p><h2>Users</h2></div><span>{users.length} users · {roles.length} roles</span></div>
			{#if can(Permission.LIST_USERS)}<div class="table-wrap"><table><thead><tr><th>Email</th><th>Roles</th><th>Permissions</th><th></th></tr></thead><tbody>
				{#each users as user (user.id?.value)}<tr><td>{user.email}</td><td>{user.roles.map((role) => role.name).join(', ')}</td><td>{user.permissions.length}</td><td>{#if can(Permission.REVOKE_USER_SESSIONS)}<button class="danger" onclick={() => revokeSessions(user)} disabled={busy}>Revoke sessions</button>{/if}</td></tr>{/each}
			</tbody></table></div>{/if}
			{#if can(Permission.CREATE_ROLE) && can(Permission.LIST_PERMISSIONS)}<form onsubmit={(event) => { event.preventDefault(); createRole(); }}><label>Role name<input bind:value={roleName} required maxlength="128" /></label><label>Description<input bind:value={roleDescription} maxlength="1024" /></label><label>Permissions<select multiple bind:value={rolePermissions}>{#each permissions as permission}<option value={permission}>{Permission[permission]}</option>{/each}</select></label><button class="primary" disabled={busy}>Create role</button></form>{/if}
			{#if can(Permission.LIST_ROLES)}<ul class="compact">{#each roles as role (role.id?.value)}<li><span>{role.name}</span><small>{role.permissions.length} permissions</small>{#if can(Permission.UPDATE_ROLE) && can(Permission.LIST_PERMISSIONS)}<button class="quiet" onclick={() => { editingRole = role; roleName = role.name; roleDescription = role.description; rolePermissions = [...role.permissions]; }}>Edit</button>{/if}{#if can(Permission.DELETE_ROLE)}<button class="danger" onclick={() => deleteRole(role)} disabled={busy}>Delete</button>{/if}</li>{/each}</ul>{/if}
			{#if editingRole && can(Permission.UPDATE_ROLE) && can(Permission.LIST_PERMISSIONS)}<form onsubmit={(event) => { event.preventDefault(); updateRole(); }}><label>Role name<input bind:value={roleName} required maxlength="128" /></label><label>Description<input bind:value={roleDescription} maxlength="1024" /></label><label>Permissions<select multiple bind:value={rolePermissions}>{#each permissions as permission}<option value={permission}>{Permission[permission]}</option>{/each}</select></label><button class="primary" disabled={busy}>Save role</button></form>{/if}
			{#if can(Permission.ASSIGN_ROLE_TO_USER) && can(Permission.LIST_USERS) && can(Permission.LIST_ROLES)}<form onsubmit={(event) => { event.preventDefault(); assignRole(); }}><label>User<select bind:value={selectedUser} required><option value="">Select user</option>{#each users as user}<option value={user.id?.value}>{user.email}</option>{/each}</select></label><label>Role<select bind:value={selectedRole} required><option value="">Select role</option>{#each roles as role}<option value={role.id?.value}>{role.name}</option>{/each}</select></label><button disabled={busy}>Assign role</button></form>{/if}
			{#if can(Permission.REVOKE_ROLE_FROM_USER) && can(Permission.LIST_USERS) && can(Permission.LIST_ROLES)}<form onsubmit={(event) => { event.preventDefault(); revokeRole(); }}><label>User<select bind:value={selectedUser} required><option value="">Select user</option>{#each users as user}<option value={user.id?.value}>{user.email}</option>{/each}</select></label><label>Role<select bind:value={selectedRole} required><option value="">Select role</option>{#each roles as role}<option value={role.id?.value}>{role.name}</option>{/each}</select></label><button type="submit" disabled={busy}>Revoke role</button></form>{/if}
		</section>
	{/if}

	{#if can(Permission.LIST_DEVICES)}
	<section class="card">
		<div class="section-title"><div><p class="eyebrow">Fleet</p><h2>Devices</h2></div><span>{devices.length} enrolled</span></div>
		<div class="table-wrap">
			<table>
				<thead><tr><th>Hostname</th><th>Agent</th><th>Status</th><th>Compliance</th><th>Last seen</th><th></th></tr></thead>
				<tbody>
					{#each devices as device (device.id?.value)}
						<tr><td>{device.hostname}</td><td>{device.agentVersion}</td><td><span class:ok={device.status === DeviceStatus.ONLINE}>{DeviceStatus[device.status]}</span></td><td>{ComplianceStatus[device.complianceStatus]} ({device.compliancePassing}/{device.complianceTotal})</td><td>{formatDate(device.lastSeenAt)}</td><td>{#if canAny(Permission.LIST_EXECUTION_RESULTS, Permission.GET_DEVICE_COMPLIANCE)}<button class="quiet" onclick={() => inspectDevice(device.id?.value ?? '')}>Results</button>{/if}</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
		{#if selectedDevice}
			<div class="detail-grid">
				{#if can(Permission.GET_DEVICE_COMPLIANCE)}<div><h3>Compliance</h3>{#if compliance.length === 0}<p>No compliance results.</p>{:else}<ul>{#each compliance as check}<li><strong>{check.actionName}</strong> — {check.compliant ? 'passing' : 'failing'} at {formatDate(check.checkedAt)}</li>{/each}</ul>{/if}</div>{/if}
				{#if can(Permission.LIST_EXECUTION_RESULTS)}<div><h3>Execution history</h3>{#if results.length === 0}<p>No execution results.</p>{:else}<ul>{#each results as result}<li><strong>{result.actionName}</strong> — {result.status} at {formatDate(result.completedAt)}{#if result.error}: {result.error}{/if}</li>{/each}</ul>{/if}</div>{/if}
			</div>
		{/if}
	</section>
	{/if}

	<div class="grid">
		{#if can(Permission.CREATE_TOKEN) || can(Permission.LIST_TOKENS)}
		<section class="card">
			<p class="eyebrow">Enrollment</p><h2>Registration tokens</h2>
			{#if can(Permission.CREATE_TOKEN)}<form onsubmit={(event) => { event.preventDefault(); createToken(); }}>
				<label>Name<input bind:value={tokenName} required maxlength="128" /></label>
				<label>Expires<input type="datetime-local" bind:value={tokenExpiry} required /></label>
				<button class="primary" disabled={busy}>Create token</button>
			</form>{/if}
			{#if revealedToken}<div class="secret"><strong>Save this now</strong><code>{revealedToken}</code><small>CA pin</small><code>{revealedPin}</code></div>{/if}
			{#if can(Permission.LIST_TOKENS)}<ul class="compact">{#each tokens as token (token.id?.value)}<li><span>{token.name}</span><small>{token.currentUses}/{token.maxUses || '∞'} uses · expires {formatDate(token.expiresAt)}</small></li>{/each}</ul>{/if}
		</section>
		{/if}

		{#if can(Permission.CREATE_DEVICE_GROUP) || can(Permission.LIST_DEVICE_GROUPS)}
		<section class="card">
			<p class="eyebrow">Targeting</p><h2>Static groups</h2>
			{#if can(Permission.CREATE_DEVICE_GROUP)}<form onsubmit={(event) => { event.preventDefault(); createGroup(); }}><label>Name<input bind:value={groupName} required /></label><button class="primary" disabled={busy}>Create group</button></form>{/if}
			{#if can(Permission.ADD_DEVICE_TO_GROUP) && can(Permission.LIST_DEVICE_GROUPS) && can(Permission.LIST_DEVICES)}<form onsubmit={(event) => { event.preventDefault(); addGroupMember(); }}><label>Group<select bind:value={membershipGroup} required><option value="">Select group</option>{#each groups as group}<option value={group.id?.value}>{group.name}</option>{/each}</select></label><label>Device<select bind:value={membershipDevice} required><option value="">Select device</option>{#each devices as device}<option value={device.id?.value}>{device.hostname}</option>{/each}</select></label><button disabled={busy}>Add member</button></form>{/if}
			{#if can(Permission.LIST_DEVICE_GROUPS)}<ul class="compact">{#each groups as group (group.id?.value)}<li><span>{group.name}</span><small>{group.memberCount} devices</small></li>{/each}</ul>{/if}
		</section>
		{/if}
	</div>

	{#if can(Permission.CREATE_ACTION) || can(Permission.LIST_ACTIONS)}
	<section class="card">
		<p class="eyebrow">Desired state</p><h2>Actions</h2>
		{#if can(Permission.CREATE_ACTION)}<form class="action-form" onsubmit={(event) => { event.preventDefault(); createAction(); }}>
			<label>Name<input bind:value={actionName} required /></label>
			<label>Description<input bind:value={actionDescription} maxlength="1024" /></label>
			<label>Type<select bind:value={actionType}><option value="package">Package</option><option value="update">Full system update</option><option value="shell">Shell</option></select></label>
			<label>Interval hours<input type="number" min="0" max="8760" bind:value={intervalHours} /></label>
			{#if actionType === 'package'}
				<label>Package name<input bind:value={packageName} required /></label><label>Version<input bind:value={packageVersion} /></label><label class="check"><input type="checkbox" bind:checked={packageRemove} /> Remove package</label>
			{:else if actionType === 'shell'}
				<label class="wide-field">Detection script<textarea bind:value={detectionScript} rows="4"></textarea></label><label class="wide-field">Remediation script<textarea bind:value={shellScript} rows="6"></textarea></label><label class="check"><input type="checkbox" bind:checked={complianceAction} /> Detection-only compliance check</label>
			{/if}
			<button class="primary" disabled={busy}>Create action</button>
		</form>{/if}
		{#if can(Permission.LIST_ACTIONS)}<div class="table-wrap"><table><thead><tr><th>Name</th><th>Type</th><th>Schedule</th><th></th></tr></thead><tbody>{#each actions as action (action.id?.value)}<tr><td><strong>{action.name}</strong><small>{action.description}</small></td><td>{actionTypeName(action.type)}</td><td>{action.schedule?.intervalHours ? `Every ${action.schedule.intervalHours}h` : 'On sync'}</td><td>{#if can(Permission.DELETE_ACTION)}<button class="danger" onclick={() => run(() => api.deleteAction(create(DeleteActionRequestSchema, { id: action.id })))} disabled={busy}>Delete</button>{/if}</td></tr>{/each}</tbody></table></div>{/if}
	</section>
	{/if}

	{#if can(Permission.CREATE_ASSIGNMENT) || can(Permission.LIST_ASSIGNMENTS)}
	<section class="card">
		<p class="eyebrow">Delivery</p><h2>Assignments</h2>
		{#if can(Permission.CREATE_ASSIGNMENT) && can(Permission.LIST_ACTIONS) && canAny(Permission.LIST_DEVICES, Permission.LIST_DEVICE_GROUPS)}<form class="assignment-form" onsubmit={(event) => { event.preventDefault(); createAssignment(); }}>
			<label>Action<select bind:value={assignmentAction} required><option value="">Select action</option>{#each actions as action}<option value={action.id?.value}>{action.name}</option>{/each}</select></label>
			<label>Target type<select bind:value={assignmentTargetType} onchange={() => assignmentTarget = ''}>{#if can(Permission.LIST_DEVICES)}<option value={AssignmentTargetType.DEVICE}>Device</option>{/if}{#if can(Permission.LIST_DEVICE_GROUPS)}<option value={AssignmentTargetType.DEVICE_GROUP}>Group</option>{/if}</select></label>
			<label>Target<select bind:value={assignmentTarget} required><option value="">Select target</option>{#if Number(assignmentTargetType) === AssignmentTargetType.DEVICE && can(Permission.LIST_DEVICES)}{#each devices as device}<option value={device.id?.value}>{device.hostname}</option>{/each}{:else if Number(assignmentTargetType) === AssignmentTargetType.DEVICE_GROUP && can(Permission.LIST_DEVICE_GROUPS)}{#each groups as group}<option value={group.id?.value}>{group.name}</option>{/each}{/if}</select></label>
			<button class="primary" disabled={busy}>Assign</button>
		</form>{/if}
		{#if can(Permission.LIST_ASSIGNMENTS)}<div class="table-wrap"><table><thead><tr><th>Action</th><th>Target</th><th>Created</th><th></th></tr></thead><tbody>{#each assignments as assignment (assignment.id?.value)}<tr><td>{assignment.actionName}</td><td>{assignment.targetName}</td><td>{formatDate(assignment.createdAt)}</td><td>{#if can(Permission.DELETE_ASSIGNMENT)}<button class="danger" onclick={() => run(() => api.deleteAssignment(create(DeleteAssignmentRequestSchema, { id: assignment.id })))} disabled={busy}>Delete</button>{/if}</td></tr>{/each}</tbody></table></div>{/if}
	</section>
	{/if}
</main>
