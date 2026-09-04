<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { ActionScheduleSchema, PackageActionParamsSchema, ShellActionParamsSchema, UpdateActionParamsSchema } from '$contract/cadestro/v1/actions_pb';
	import { DesiredState } from '$contract/cadestro/v1/common_pb';
	import {
		ConfigureActionRequestSchema,
		CreateActionRequestSchema,
		DeleteActionRequestSchema,
		ListActionsRequestSchema,
		Permission,
		RenameActionRequestSchema,
		SetActionDescriptionRequestSchema,
		type CreateActionRequest,
		type ManagedAction
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage } from '$lib/api';
	import { cursorHref } from '$lib/console';

	type ActionKind = 'package' | 'update' | 'shell';

	let { can }: { can: (permission: Permission) => boolean } = $props();
	let actions = $state<ManagedAction[]>([]);
	let totalCount = $state(0);
	let nextPageToken = $state('');
	let loading = $state(false);
	let creating = $state(false);
	let renameBusy = $state(false);
	let descriptionBusy = $state(false);
	let configurationBusy = $state(false);
	let deleting = $state('');
	let error = $state('');
	let notice = $state('');
	let name = $state('');
	let description = $state('');
	let kind = $state<ActionKind>('package');
	let desiredState = $state<DesiredState>(DesiredState.PRESENT);
	let timeoutSeconds = $state(0);
	let intervalHours = $state(0);
	let packageName = $state('');
	let packageVersion = $state('');
	let script = $state('');
	let detectionScript = $state('');
	let interpreter = $state('');
	let workingDirectory = $state('');
	let environment = $state('');
	let isCompliance = $state(false);
	let editing = $state<ManagedAction>();
	let editName = $state('');
	let editDescription = $state('');
	let editDesiredState = $state<DesiredState>(DesiredState.PRESENT);
	let editTimeoutSeconds = $state(0);
	let editIntervalHours = $state(0);
	let editPackageName = $state('');
	let editPackageVersion = $state('');
	let editScript = $state('');
	let editDetectionScript = $state('');
	let editInterpreter = $state('');
	let editWorkingDirectory = $state('');
	let editEnvironment = $state('');
	let editIsCompliance = $state(false);
	const pageToken = $derived(page.url.searchParams.get('actionsCursor') ?? '');

	function parseEnvironment(value: string): Record<string, string> {
		const variables: Record<string, string> = {};
		for (const line of value.split('\n')) {
			if (!line.trim()) continue;
			const split = line.indexOf('=');
			if (split < 1) throw new Error(`Invalid environment entry: ${line}`);
			variables[line.slice(0, split).trim()] = line.slice(split + 1);
		}
		return variables;
	}

	function actionParams(actionKind: ActionKind, packageValue: string, version: string, shellScript: string, detection: string, shellInterpreter: string, directory: string, variables: string, complianceAction: boolean): CreateActionRequest['params'] {
		if (actionKind === 'package') return { case: 'package', value: create(PackageActionParamsSchema, { name: packageValue, version }) };
		if (actionKind === 'update') return { case: 'update', value: create(UpdateActionParamsSchema) };
		return { case: 'shell', value: create(ShellActionParamsSchema, { script: shellScript, detectionScript: detection, interpreter: shellInterpreter, workingDirectory: directory, environment: parseEnvironment(variables), isCompliance: complianceAction }) };
	}

	function timeoutHelp(actionKind: ActionKind): string {
		return actionKind === 'shell' ? '0 uses the 1 hour default.' : '0 uses the 30 minute default.';
	}

	function scheduleLabel(value: number): string {
		return value ? `Every ${value} hours` : 'Every 8 hours (default)';
	}

	function chooseKind(event: Event) {
		kind = (event.currentTarget as HTMLSelectElement).value as ActionKind;
		if (kind !== 'package') desiredState = DesiredState.PRESENT;
	}

	async function load(): Promise<boolean> {
		if (!can(Permission.LIST_ACTIONS)) return true;
		loading = true;
		error = '';
		try {
			const response = await api.listActions(create(ListActionsRequestSchema, { pageSize: 50, pageToken }));
			actions = response.actions;
			totalCount = response.totalCount;
			nextPageToken = response.nextPageToken;
			return true;
		} catch (cause) {
			error = errorMessage(cause);
			return false;
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function refreshed(message: string) {
		const loaded = await load();
		notice = loaded ? message : `${message} Refresh failed; the displayed state may be stale.`;
	}

	async function createAction() {
		creating = true;
		error = '';
		notice = '';
		try {
			await api.createAction(create(CreateActionRequestSchema, {
				name,
				description,
				desiredState: kind === 'package' ? desiredState : DesiredState.PRESENT,
				timeoutSeconds,
				schedule: create(ActionScheduleSchema, { intervalHours }),
				params: actionParams(kind, packageName, packageVersion, script, detectionScript, interpreter, workingDirectory, environment, isCompliance)
			}));
			name = '';
			description = '';
			packageName = '';
			packageVersion = '';
			script = '';
			detectionScript = '';
			interpreter = '';
			workingDirectory = '';
			environment = '';
			await refreshed('Action created.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			creating = false;
		}
	}

	function edit(action: ManagedAction) {
		editing = action;
		editName = action.name;
		editDescription = action.description;
		editDesiredState = action.desiredState;
		editTimeoutSeconds = action.timeoutSeconds;
		editIntervalHours = action.schedule?.intervalHours ?? 0;
		editPackageName = action.params.case === 'package' ? action.params.value.name : '';
		editPackageVersion = action.params.case === 'package' ? action.params.value.version : '';
		editScript = action.params.case === 'shell' ? action.params.value.script : '';
		editDetectionScript = action.params.case === 'shell' ? action.params.value.detectionScript : '';
		editInterpreter = action.params.case === 'shell' ? action.params.value.interpreter : '';
		editWorkingDirectory = action.params.case === 'shell' ? action.params.value.workingDirectory : '';
		editEnvironment = action.params.case === 'shell' ? Object.entries(action.params.value.environment).map(([key, value]) => `${key}=${value}`).join('\n') : '';
		editIsCompliance = action.params.case === 'shell' && action.params.value.isCompliance;
	}

	async function reloadEditing(): Promise<boolean> {
		const loaded = await load();
		const current = actions.find((action) => action.id?.value === editing?.id?.value);
		if (current) editing = current;
		return loaded && Boolean(current);
	}

	async function renameAction() {
		if (!editing?.id) return;
		renameBusy = true;
		error = '';
		notice = '';
		try {
			const response = await api.renameAction(create(RenameActionRequestSchema, { id: editing.id, name: editName }));
			if (response.action) editing = response.action;
			await refreshed('Action renamed.');
		} catch (cause) {
			const mutationError = errorMessage(cause);
			const reloaded = await reloadEditing();
			error = reloaded ? mutationError : `${mutationError} The current action could not be reloaded.`;
		} finally {
			renameBusy = false;
		}
	}

	async function describeAction() {
		if (!editing?.id) return;
		descriptionBusy = true;
		error = '';
		notice = '';
		try {
			const response = await api.setActionDescription(create(SetActionDescriptionRequestSchema, { id: editing.id, description: editDescription }));
			if (response.action) editing = response.action;
			await refreshed('Action description saved.');
		} catch (cause) {
			const mutationError = errorMessage(cause);
			const reloaded = await reloadEditing();
			error = reloaded ? mutationError : `${mutationError} The current action could not be reloaded.`;
		} finally {
			descriptionBusy = false;
		}
	}

	async function configureAction() {
		if (!editing?.id || !editing.params.case) return;
		configurationBusy = true;
		error = '';
		notice = '';
		try {
			const response = await api.configureAction(create(ConfigureActionRequestSchema, {
				id: editing.id,
				desiredState: editDesiredState,
				timeoutSeconds: editTimeoutSeconds,
				schedule: create(ActionScheduleSchema, { intervalHours: editIntervalHours }),
				params: actionParams(editing.params.case, editPackageName, editPackageVersion, editScript, editDetectionScript, editInterpreter, editWorkingDirectory, editEnvironment, editIsCompliance)
			}));
			if (response.action) editing = response.action;
			await refreshed('Action configuration saved.');
		} catch (cause) {
			const mutationError = errorMessage(cause);
			const reloaded = await reloadEditing();
			error = reloaded ? mutationError : `${mutationError} The current action could not be reloaded.`;
		} finally {
			configurationBusy = false;
		}
	}

	async function deleteAction(action: ManagedAction) {
		if (!action.id || !confirm(`Delete ${action.name}?`)) return;
		deleting = action.id.value;
		error = '';
		notice = '';
		try {
			await api.deleteAction(create(DeleteActionRequestSchema, { id: action.id }));
			if (editing?.id?.value === action.id.value) editing = undefined;
			await refreshed('Action deleted.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			deleting = '';
		}
	}
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title">
		<div><p class="eyebrow">Desired state</p><h1>Actions</h1></div>
		{#if can(Permission.LIST_ACTIONS)}<span>{totalCount} actions</span>{/if}
	</div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if notice}<p class="notice" role="status">{notice}</p>{/if}
	{#if can(Permission.CREATE_ACTION)}
		<form class="editor" onsubmit={(event) => { event.preventDefault(); createAction(); }}>
			<fieldset disabled={creating}>
				<h2>Create action</h2>
				<label>Name<input bind:value={name} required maxlength="255" /></label>
				<label>Description<input bind:value={description} maxlength="1024" /></label>
				<label>Type<select value={kind} onchange={chooseKind}><option value="package">Package</option><option value="update">Full system update</option><option value="shell">Shell</option></select></label>
				<label>Desired state<select bind:value={desiredState}><option value={DesiredState.PRESENT}>Present</option>{#if kind === 'package'}<option value={DesiredState.ABSENT}>Absent</option>{/if}</select></label>
				<label>Interval hours<input type="number" min="0" max="8760" bind:value={intervalHours} /><small>0 runs every 8 hours.</small></label>
				<label>Timeout seconds<input type="number" min="0" max="3600" bind:value={timeoutSeconds} /><small>{timeoutHelp(kind)}</small></label>
				{#if kind === 'package'}
					<label>Package name<input bind:value={packageName} required maxlength="255" /></label>
					<label>Version<input bind:value={packageVersion} maxlength="128" /></label>
				{:else if kind === 'shell'}
					<label>Interpreter<input bind:value={interpreter} maxlength="255" placeholder="/bin/sh" /></label>
					<label>Working directory<input bind:value={workingDirectory} pattern="/.*" title="Use an absolute path" placeholder="/opt/example" /></label>
					<label class="wide-field">Environment<textarea bind:value={environment} rows="4" placeholder="NAME=value"></textarea></label>
					<label class="wide-field">Detection script<textarea bind:value={detectionScript} required={isCompliance || !script} rows="5"></textarea></label>
					<label class="wide-field">Remediation script<textarea bind:value={script} required={!isCompliance && !detectionScript} rows="7"></textarea></label>
					<label class="check"><input type="checkbox" bind:checked={isCompliance} /> Compliance check</label>
				{/if}
				<button class="primary">Create action</button>
			</fieldset>
		</form>
	{/if}
	{#if can(Permission.LIST_ACTIONS)}
		{#if loading}<p role="status">Loading actions…</p>{:else if actions.length === 0}<p>No actions.</p>{:else}
			<div class="table-wrap"><table><thead><tr><th>Name</th><th>Type</th><th>State</th><th>Schedule</th><th>Timeout</th><th></th></tr></thead><tbody>
				{#each actions as action (action.id?.value)}
					<tr>
						<td><strong>{action.name}</strong><small>{action.description}</small></td>
						<td>{action.params.case || 'Unknown'}</td>
						<td>{DesiredState[action.desiredState]}</td>
						<td>{scheduleLabel(action.schedule?.intervalHours ?? 0)}</td>
						<td>{action.timeoutSeconds ? `${action.timeoutSeconds}s` : timeoutHelp(action.params.case || 'package')}</td>
						<td class="row-actions">
							{#if can(Permission.RENAME_ACTION) || can(Permission.UPDATE_ACTION_DESCRIPTION) || can(Permission.UPDATE_ACTION_PARAMS)}
								<button type="button" class="quiet" onclick={() => edit(action)} disabled={renameBusy || descriptionBusy || configurationBusy || Boolean(deleting)}>Edit</button>
							{/if}
							{#if can(Permission.DELETE_ACTION)}<button type="button" class="danger" onclick={() => deleteAction(action)} disabled={Boolean(deleting)}>Delete</button>{/if}
						</td>
					</tr>
				{/each}
			</tbody></table></div>
		{/if}
		<nav class="pagination" aria-label="Action pages">{#if pageToken}<a class="button quiet" href={cursorHref(page.url, 'actionsCursor', '')}>First page</a>{/if}{#if nextPageToken}<a class="button" href={cursorHref(page.url, 'actionsCursor', nextPageToken)}>Next page</a>{/if}</nav>
	{/if}
	</section>

{#if editing}
		<section class="card"><div class="section-title"><div><p class="eyebrow">Action editor</p><h2>{editing.name}</h2></div><button type="button" class="quiet" onclick={() => editing = undefined} disabled={renameBusy || descriptionBusy || configurationBusy}>Close</button></div>
		{#if can(Permission.RENAME_ACTION)}<form onsubmit={(event) => { event.preventDefault(); renameAction(); }}><fieldset disabled={renameBusy}><label>Name<input bind:value={editName} required maxlength="255" /></label><button class="primary">Save name</button></fieldset></form>{/if}
		{#if can(Permission.UPDATE_ACTION_DESCRIPTION)}<form onsubmit={(event) => { event.preventDefault(); describeAction(); }}><fieldset disabled={descriptionBusy}><label>Description<input bind:value={editDescription} maxlength="1024" /></label><button class="primary">Save description</button></fieldset></form>{/if}
		{#if can(Permission.UPDATE_ACTION_PARAMS) && editing.params.case}
			<form class="editor" onsubmit={(event) => { event.preventDefault(); configureAction(); }}><fieldset disabled={configurationBusy}>
				<h3>Configuration</h3>
				<p>Type: <strong>{editing.params.case}</strong></p>
				<label>Desired state<select bind:value={editDesiredState}><option value={DesiredState.PRESENT}>Present</option>{#if editing.params.case === 'package'}<option value={DesiredState.ABSENT}>Absent</option>{/if}</select></label>
				<label>Interval hours<input type="number" min="0" max="8760" bind:value={editIntervalHours} /><small>0 runs every 8 hours.</small></label>
				<label>Timeout seconds<input type="number" min="0" max="3600" bind:value={editTimeoutSeconds} /><small>{timeoutHelp(editing.params.case)}</small></label>
				{#if editing.params.case === 'package'}
					<label>Package name<input bind:value={editPackageName} required maxlength="255" /></label><label>Version<input bind:value={editPackageVersion} maxlength="128" /></label>
				{:else if editing.params.case === 'shell'}
					<label>Interpreter<input bind:value={editInterpreter} maxlength="255" /></label>
					<label>Working directory<input bind:value={editWorkingDirectory} pattern="/.*" title="Use an absolute path" /></label>
					<label class="wide-field">Environment<textarea bind:value={editEnvironment} rows="4"></textarea></label>
					<label class="wide-field">Detection script<textarea bind:value={editDetectionScript} required={editIsCompliance || !editScript} rows="5"></textarea></label>
					<label class="wide-field">Remediation script<textarea bind:value={editScript} required={!editIsCompliance && !editDetectionScript} rows="7"></textarea></label>
					<label class="check"><input type="checkbox" bind:checked={editIsCompliance} /> Compliance check</label>
				{/if}
				<button class="primary">Save configuration</button>
			</fieldset></form>
		{/if}
	</section>
{/if}
