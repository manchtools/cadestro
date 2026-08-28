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
		type Assignment,
		type ComplianceCheckResult,
		type Device,
		type DeviceGroup,
		type ExecutionResult,
		type ManagedAction,
		type RegistrationToken
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
	let selectedDevice = $state('');
	let error = $state('');
	let busy = $state(false);
	let revealedToken = $state('');
	let revealedPin = $state('');

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
			const [devicePage, actionPage, groupPage, assignmentPage, tokenPage] = await Promise.all([
				api.listDevices(create(ListDevicesRequestSchema, { pageSize: 100 })),
				api.listActions(create(ListActionsRequestSchema, { pageSize: 100 })),
				api.listDeviceGroups(create(ListDeviceGroupsRequestSchema, { pageSize: 100 })),
				api.listAssignments(create(ListAssignmentsRequestSchema, {})),
				api.listTokens(create(ListTokensRequestSchema, { pageSize: 100, includeDisabled: true }))
			]);
			devices = devicePage.devices;
			actions = actionPage.actions;
			groups = groupPage.groups;
			assignments = assignmentPage.assignments;
			tokens = tokenPage.tokens;
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
				api.listExecutionResults(create(ListExecutionResultsRequestSchema, { deviceId: { value: deviceID }, pageSize: 50 })),
				api.getDeviceCompliance(create(GetDeviceComplianceRequestSchema, { deviceId: { value: deviceID } }))
			]);
			results = history.results;
			compliance = current.checks;
		} catch (cause) {
			error = errorMessage(cause);
		}
	}

	async function signOut() {
		await logout();
		await goto('/login');
	}
</script>

<header>
	<div><strong>Cadestro</strong><span>Linux management core</span></div>
	<nav><button class="quiet" onclick={load} disabled={busy}>Refresh</button><button class="quiet" onclick={signOut}>Sign out</button></nav>
</header>

<main class="stack">
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}

	<section class="card">
		<div class="section-title"><div><p class="eyebrow">Fleet</p><h2>Devices</h2></div><span>{devices.length} enrolled</span></div>
		<div class="table-wrap">
			<table>
				<thead><tr><th>Hostname</th><th>Agent</th><th>Status</th><th>Compliance</th><th>Last seen</th><th></th></tr></thead>
				<tbody>
					{#each devices as device (device.id?.value)}
						<tr><td>{device.hostname}</td><td>{device.agentVersion}</td><td><span class:ok={device.status === DeviceStatus.ONLINE}>{DeviceStatus[device.status]}</span></td><td>{ComplianceStatus[device.complianceStatus]} ({device.compliancePassing}/{device.complianceTotal})</td><td>{formatDate(device.lastSeenAt)}</td><td><button class="quiet" onclick={() => inspectDevice(device.id?.value ?? '')}>Results</button></td></tr>
					{/each}
				</tbody>
			</table>
		</div>
		{#if selectedDevice}
			<div class="detail-grid">
				<div><h3>Compliance</h3>{#if compliance.length === 0}<p>No compliance results.</p>{:else}<ul>{#each compliance as check}<li><strong>{check.actionName}</strong> — {check.compliant ? 'passing' : 'failing'} at {formatDate(check.checkedAt)}</li>{/each}</ul>{/if}</div>
				<div><h3>Execution history</h3>{#if results.length === 0}<p>No execution results.</p>{:else}<ul>{#each results as result}<li><strong>{result.actionName}</strong> — {result.status} at {formatDate(result.completedAt)}{#if result.error}: {result.error}{/if}</li>{/each}</ul>{/if}</div>
			</div>
		{/if}
	</section>

	<div class="grid">
		<section class="card">
			<p class="eyebrow">Enrollment</p><h2>Registration tokens</h2>
			<form onsubmit={(event) => { event.preventDefault(); createToken(); }}>
				<label>Name<input bind:value={tokenName} required maxlength="128" /></label>
				<label>Expires<input type="datetime-local" bind:value={tokenExpiry} required /></label>
				<button class="primary" disabled={busy}>Create token</button>
			</form>
			{#if revealedToken}<div class="secret"><strong>Save this now</strong><code>{revealedToken}</code><small>CA pin</small><code>{revealedPin}</code></div>{/if}
			<ul class="compact">{#each tokens as token (token.id?.value)}<li><span>{token.name}</span><small>{token.currentUses}/{token.maxUses || '∞'} uses · expires {formatDate(token.expiresAt)}</small></li>{/each}</ul>
		</section>

		<section class="card">
			<p class="eyebrow">Targeting</p><h2>Static groups</h2>
			<form onsubmit={(event) => { event.preventDefault(); createGroup(); }}><label>Name<input bind:value={groupName} required /></label><button class="primary" disabled={busy}>Create group</button></form>
			<form onsubmit={(event) => { event.preventDefault(); addGroupMember(); }}><label>Group<select bind:value={membershipGroup} required><option value="">Select group</option>{#each groups as group}<option value={group.id?.value}>{group.name}</option>{/each}</select></label><label>Device<select bind:value={membershipDevice} required><option value="">Select device</option>{#each devices as device}<option value={device.id?.value}>{device.hostname}</option>{/each}</select></label><button disabled={busy}>Add member</button></form>
			<ul class="compact">{#each groups as group (group.id?.value)}<li><span>{group.name}</span><small>{group.memberCount} devices</small></li>{/each}</ul>
		</section>
	</div>

	<section class="card">
		<p class="eyebrow">Desired state</p><h2>Actions</h2>
		<form class="action-form" onsubmit={(event) => { event.preventDefault(); createAction(); }}>
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
		</form>
		<div class="table-wrap"><table><thead><tr><th>Name</th><th>Type</th><th>Schedule</th><th></th></tr></thead><tbody>{#each actions as action (action.id?.value)}<tr><td><strong>{action.name}</strong><small>{action.description}</small></td><td>{actionTypeName(action.type)}</td><td>{action.schedule?.intervalHours ? `Every ${action.schedule.intervalHours}h` : 'On sync'}</td><td><button class="danger" onclick={() => run(() => api.deleteAction(create(DeleteActionRequestSchema, { id: action.id })))} disabled={busy}>Delete</button></td></tr>{/each}</tbody></table></div>
	</section>

	<section class="card">
		<p class="eyebrow">Delivery</p><h2>Assignments</h2>
		<form class="assignment-form" onsubmit={(event) => { event.preventDefault(); createAssignment(); }}>
			<label>Action<select bind:value={assignmentAction} required><option value="">Select action</option>{#each actions as action}<option value={action.id?.value}>{action.name}</option>{/each}</select></label>
			<label>Target type<select bind:value={assignmentTargetType} onchange={() => assignmentTarget = ''}><option value={AssignmentTargetType.DEVICE}>Device</option><option value={AssignmentTargetType.DEVICE_GROUP}>Group</option></select></label>
			<label>Target<select bind:value={assignmentTarget} required><option value="">Select target</option>{#if Number(assignmentTargetType) === AssignmentTargetType.DEVICE}{#each devices as device}<option value={device.id?.value}>{device.hostname}</option>{/each}{:else}{#each groups as group}<option value={group.id?.value}>{group.name}</option>{/each}{/if}</select></label>
			<button class="primary" disabled={busy}>Assign</button>
		</form>
		<div class="table-wrap"><table><thead><tr><th>Action</th><th>Target</th><th>Created</th><th></th></tr></thead><tbody>{#each assignments as assignment (assignment.id?.value)}<tr><td>{assignment.actionName}</td><td>{assignment.targetName}</td><td>{formatDate(assignment.createdAt)}</td><td><button class="danger" onclick={() => run(() => api.deleteAssignment(create(DeleteAssignmentRequestSchema, { id: assignment.id })))} disabled={busy}>Delete</button></td></tr>{/each}</tbody></table></div>
	</section>
</main>
