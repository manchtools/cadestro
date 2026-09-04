<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { AssignmentTargetType, ComplianceStatus, DeviceStatus, ExecutionStatus } from '$contract/cadestro/v1/common_pb';
	import {
		DeleteDeviceRequestSchema,
		GetDeviceAssignmentsRequestSchema,
		GetDeviceComplianceRequestSchema,
		GetDeviceRequestSchema,
		ListDeviceGroupsForDeviceRequestSchema,
		ListDevicesRequestSchema,
		ListExecutionResultsRequestSchema,
		Permission,
		type ComplianceCheckResult,
		type Device,
		type DeviceGroup,
		type ExecutionResult,
		type ManagedAction
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage } from '$lib/api';
	import { cursorHref, formatDate } from '$lib/console';

	let { can }: { can: (permission: Permission) => boolean } = $props();
	let devices = $state<Device[]>([]);
	let totalCount = $state(0);
	let nextPageToken = $state('');
	let selected = $state<Device>();
	let groups = $state<DeviceGroup[]>([]);
	let assignments = $state<ManagedAction[]>([]);
	let compliance = $state<ComplianceCheckResult[]>([]);
	let results = $state<ExecutionResult[]>([]);
	let loading = $state(true);
	let detailLoading = $state(false);
	let busy = $state(false);
	let error = $state('');
	let detailErrors = $state<string[]>([]);
	const pageToken = $derived(page.url.searchParams.get('devicesCursor') ?? '');
	const deviceID = $derived(page.url.searchParams.get('device') ?? '');

	function selectedHref(id: string): string {
		const target = new URL(page.url);
		target.searchParams.delete('resultsCursor');
		if (id) target.searchParams.set('device', id);
		else {
			target.searchParams.delete('device');
		}
		return `${target.pathname}${target.search}`;
	}

	async function loadDevices() {
		if (!can(Permission.LIST_DEVICES)) {
			loading = false;
			return;
		}
		loading = true;
		error = '';
		try {
			const response = await api.listDevices(create(ListDevicesRequestSchema, { pageSize: 50, pageToken }));
			devices = response.devices;
			totalCount = response.totalCount;
			nextPageToken = response.nextPageToken;
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	async function loadDetails() {
		if (!deviceID) return;
		detailLoading = true;
		detailErrors = [];
		const id = { value: deviceID };
		const requests = [
			can(Permission.GET_DEVICE) ? api.getDevice(create(GetDeviceRequestSchema, { id })) : Promise.resolve(null),
			can(Permission.LIST_DEVICE_GROUPS_FOR_DEVICE) ? api.listDeviceGroupsForDevice(create(ListDeviceGroupsForDeviceRequestSchema, { deviceId: id })) : Promise.resolve(null),
			can(Permission.GET_DEVICE_ASSIGNMENTS) ? api.getDeviceAssignments(create(GetDeviceAssignmentsRequestSchema, { deviceId: id })) : Promise.resolve(null),
			can(Permission.GET_DEVICE_COMPLIANCE) ? api.getDeviceCompliance(create(GetDeviceComplianceRequestSchema, { deviceId: id })) : Promise.resolve(null),
			can(Permission.LIST_EXECUTION_RESULTS) ? api.listExecutionResults(create(ListExecutionResultsRequestSchema, { deviceId: id, pageSize: 25, pageToken: page.url.searchParams.get('resultsCursor') ?? '' })) : Promise.resolve(null)
		] as const;
		const loaded = await Promise.allSettled(requests);
		if (loaded[0].status === 'fulfilled') selected = loaded[0].value?.device ?? devices.find((device) => device.id?.value === deviceID);
		else detailErrors.push(`Device: ${errorMessage(loaded[0].reason)}`);
		if (loaded[1].status === 'fulfilled') groups = loaded[1].value?.groups ?? [];
		else detailErrors.push(`Groups: ${errorMessage(loaded[1].reason)}`);
		if (loaded[2].status === 'fulfilled') assignments = loaded[2].value?.actions ?? [];
		else detailErrors.push(`Assignments: ${errorMessage(loaded[2].reason)}`);
		if (loaded[3].status === 'fulfilled') compliance = loaded[3].value?.checks ?? [];
		else detailErrors.push(`Compliance: ${errorMessage(loaded[3].reason)}`);
		if (loaded[4].status === 'fulfilled') {
			results = loaded[4].value?.results ?? [];
			nextResultsToken = loaded[4].value?.nextPageToken ?? '';
		} else detailErrors.push(`History: ${errorMessage(loaded[4].reason)}`);
		detailLoading = false;
	}

	let nextResultsToken = $state('');

	onMount(async () => {
		await loadDevices();
		await loadDetails();
	});

	async function deleteDevice() {
		if (!selected?.id || !confirm(`Delete ${selected.hostname}?`)) return;
		busy = true;
		error = '';
		try {
			await api.deleteDevice(create(DeleteDeviceRequestSchema, { id: selected.id }));
			await goto(selectedHref(''));
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			busy = false;
		}
	}
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title">
		<div><p class="eyebrow">Fleet</p><h1>Devices</h1></div>
		<span>{totalCount} enrolled</span>
	</div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if loading}
		<p role="status">Loading devices…</p>
	{:else if devices.length === 0}
		<p>No devices are enrolled.</p>
	{:else}
		<div class="table-wrap"><table><thead><tr><th>Hostname</th><th>Agent</th><th>Status</th><th>Compliance</th><th>Last seen</th><th></th></tr></thead><tbody>
			{#each devices as device (device.id?.value)}
				<tr>
					<td>{device.hostname}</td>
					<td>{device.agentVersion}</td>
					<td><span class:ok={device.status === DeviceStatus.ONLINE}>{DeviceStatus[device.status]}</span></td>
					<td>{ComplianceStatus[device.complianceStatus]} ({device.compliancePassing}/{device.complianceTotal})</td>
					<td>{formatDate(device.lastSeenAt)}</td>
					<td>{#if can(Permission.GET_DEVICE) || can(Permission.DELETE_DEVICE) || can(Permission.LIST_DEVICE_GROUPS_FOR_DEVICE) || can(Permission.GET_DEVICE_ASSIGNMENTS) || can(Permission.GET_DEVICE_COMPLIANCE) || can(Permission.LIST_EXECUTION_RESULTS)}<a class="button quiet" href={selectedHref(device.id?.value ?? '')}>Details</a>{/if}</td>
				</tr>
			{/each}
		</tbody></table></div>
		<nav class="pagination" aria-label="Device pages">
			{#if pageToken}<a class="button quiet" href={cursorHref(page.url, 'devicesCursor', '', ['device', 'resultsCursor'])}>First page</a>{/if}
			{#if nextPageToken}<a class="button" href={cursorHref(page.url, 'devicesCursor', nextPageToken, ['device', 'resultsCursor'])}>Next page</a>{/if}
		</nav>
	{/if}
</section>

{#if deviceID}
	<section class="card" aria-busy={detailLoading}>
		<div class="section-title"><div><p class="eyebrow">Device detail</p><h2>{selected?.hostname ?? deviceID}</h2></div><a class="button quiet" href={selectedHref('')}>Close</a></div>
		{#if detailLoading}<p role="status">Loading device details…</p>{/if}
		{#each detailErrors as detailError}<p class="error banner" role="alert">{detailError}</p>{/each}
		{#if selected}
			<dl class="facts"><div><dt>Status</dt><dd>{DeviceStatus[selected.status]}</dd></div><div><dt>Registered</dt><dd>{formatDate(selected.registeredAt)}</dd></div><div><dt>Certificate expires</dt><dd>{formatDate(selected.certExpiresAt)}</dd></div></dl>
		{/if}
		<div class="detail-grid">
			{#if can(Permission.LIST_DEVICE_GROUPS_FOR_DEVICE)}
				<div><h3>Groups</h3>{#if groups.length}<ul>{#each groups as group}<li>{group.name}</li>{/each}</ul>{:else}<p>No group memberships.</p>{/if}</div>
			{/if}
			{#if can(Permission.GET_DEVICE_ASSIGNMENTS)}
				<div><h3>Assigned actions</h3>{#if assignments.length}<ul>{#each assignments as action}<li>{action.name}</li>{/each}</ul>{:else}<p>No assigned actions.</p>{/if}</div>
			{/if}
			{#if can(Permission.GET_DEVICE_COMPLIANCE)}
				<div><h3>Compliance</h3>
					{#if compliance.length}
						<ul>{#each compliance as check}<li>
							<strong>{check.actionName}</strong> — {ComplianceStatus[check.status]} at {formatDate(check.checkedAt)}
							{#if check.detectionOutput}<details><summary>Detection output (exit {check.detectionOutput.exitCode})</summary>{#if check.detectionOutput.stdout}<pre>{check.detectionOutput.stdout}</pre>{/if}{#if check.detectionOutput.stderr}<pre>{check.detectionOutput.stderr}</pre>{/if}</details>{/if}
						</li>{/each}</ul>
					{:else}<p>No compliance checks.</p>{/if}
				</div>
			{/if}
			{#if can(Permission.LIST_EXECUTION_RESULTS)}
				<div><h3>Execution history</h3>
					{#if results.length}
						<ul>{#each results as result}<li>
							<strong>{result.actionName}</strong> — {ExecutionStatus[result.status]} · {ComplianceStatus[result.complianceStatus]} at {formatDate(result.completedAt)}
							{#if result.output}<details><summary>Command output (exit {result.output.exitCode})</summary>{#if result.output.stdout}<pre>{result.output.stdout}</pre>{/if}{#if result.output.stderr}<pre>{result.output.stderr}</pre>{/if}</details>{/if}
							{#if result.detectionOutput}<details><summary>Detection output (exit {result.detectionOutput.exitCode})</summary>{#if result.detectionOutput.stdout}<pre>{result.detectionOutput.stdout}</pre>{/if}{#if result.detectionOutput.stderr}<pre>{result.detectionOutput.stderr}</pre>{/if}</details>{/if}
						</li>{/each}</ul>
					{:else}<p>No execution results.</p>{/if}
					<nav class="pagination" aria-label="Execution result pages">{#if page.url.searchParams.has('resultsCursor')}<a class="button quiet" href={cursorHref(page.url, 'resultsCursor', '')}>First page</a>{/if}{#if nextResultsToken}<a class="button" href={cursorHref(page.url, 'resultsCursor', nextResultsToken)}>Next page</a>{/if}</nav>
				</div>
			{/if}
		</div>
		{#if can(Permission.DELETE_DEVICE)}<button type="button" class="danger" onclick={deleteDevice} disabled={busy}>Delete device</button>{/if}
	</section>
{/if}
