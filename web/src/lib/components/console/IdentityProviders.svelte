<script lang="ts">
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import {
		ConfigureIdentityProviderRequestSchema,
		CreateIdentityProviderRequestSchema,
		DeleteIdentityProviderRequestSchema,
		DisableIdentityProviderRequestSchema,
		EnableIdentityProviderRequestSchema,
		ListIdentityProvidersRequestSchema,
		Permission,
		RenameIdentityProviderRequestSchema,
		type IdentityProvider
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage } from '$lib/api';

	let { can }: { can: (permission: Permission) => boolean } = $props();
	let providers = $state<IdentityProvider[]>([]);
	let loading = $state(false);
	let creating = $state(false);
	let editingBusy = $state(false);
	let rowBusy = $state('');
	let error = $state('');
	let notice = $state('');
	let name = $state('');
	let slug = $state('');
	let clientID = $state('');
	let issuerURL = $state('');
	let scopes = $state('openid\nprofile\nemail');
	let editing = $state<IdentityProvider>();
	let editName = $state('');
	let editClientID = $state('');
	let editIssuerURL = $state('');
	let editScopes = $state('');

	function scopeValues(value: string): string[] {
		return value.split(/[\n, ]+/).map((scope) => scope.trim()).filter(Boolean);
	}

	async function load(): Promise<boolean> {
		if (!can(Permission.LIST_IDENTITY_PROVIDERS)) return true;
		loading = true;
		error = '';
		try {
			providers = (await api.listIdentityProviders(create(ListIdentityProvidersRequestSchema))).providers;
			return true;
		} catch (cause) {
			error = errorMessage(cause);
			return false;
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function refreshNotice(message: string) {
		const refreshed = await load();
		notice = refreshed ? message : `${message} Refresh failed; the displayed state may be stale.`;
	}

	async function createProvider() {
		creating = true;
		error = '';
		notice = '';
		try {
			await api.createIdentityProvider(create(CreateIdentityProviderRequestSchema, {
				name,
				slug,
				clientId: { value: clientID },
				issuerUrl: issuerURL,
				scopes: scopeValues(scopes)
			}));
			name = '';
			slug = '';
			clientID = '';
			issuerURL = '';
			scopes = 'openid\nprofile\nemail';
			await refreshNotice('Identity provider created.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			creating = false;
		}
	}

	function edit(provider: IdentityProvider) {
		editing = provider;
		editName = provider.name;
		editClientID = provider.clientId?.value ?? '';
		editIssuerURL = provider.issuerUrl;
		editScopes = provider.scopes.join('\n');
	}

	async function reloadEditing() {
		await load();
		const current = providers.find((provider) => provider.id?.value === editing?.id?.value);
		if (current) editing = current;
	}

	async function renameProvider() {
		if (!editing?.id) return;
		editingBusy = true;
		error = '';
		notice = '';
		try {
			const response = await api.renameIdentityProvider(create(RenameIdentityProviderRequestSchema, { id: editing.id, name: editName }));
			if (response.provider) editing = response.provider;
			await refreshNotice('Identity provider renamed.');
		} catch (cause) {
			const mutationError = errorMessage(cause);
			await reloadEditing();
			error = mutationError;
		} finally {
			editingBusy = false;
		}
	}

	async function configureProvider() {
		if (!editing?.id) return;
		editingBusy = true;
		error = '';
		notice = '';
		try {
			const response = await api.configureIdentityProvider(create(ConfigureIdentityProviderRequestSchema, {
				id: editing.id,
				clientId: { value: editClientID },
				issuerUrl: editIssuerURL,
				scopes: scopeValues(editScopes)
			}));
			if (response.provider) editing = response.provider;
			await refreshNotice('Identity provider configuration saved.');
		} catch (cause) {
			const mutationError = errorMessage(cause);
			await reloadEditing();
			error = mutationError;
		} finally {
			editingBusy = false;
		}
	}

	async function setEnabled(provider: IdentityProvider, enabled: boolean) {
		if (!provider.id) return;
		rowBusy = provider.id.value;
		error = '';
		notice = '';
		try {
			if (enabled) await api.enableIdentityProvider(create(EnableIdentityProviderRequestSchema, { id: provider.id }));
			else await api.disableIdentityProvider(create(DisableIdentityProviderRequestSchema, { id: provider.id }));
			await refreshNotice(`Identity provider ${enabled ? 'enabled' : 'disabled'}.`);
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			rowBusy = '';
		}
	}

	async function deleteProvider(provider: IdentityProvider) {
		if (!provider.id || !confirm(`Delete ${provider.name}?`)) return;
		rowBusy = provider.id.value;
		error = '';
		notice = '';
		try {
			await api.deleteIdentityProvider(create(DeleteIdentityProviderRequestSchema, { id: provider.id }));
			if (editing?.id?.value === provider.id.value) editing = undefined;
			await refreshNotice('Identity provider deleted.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			rowBusy = '';
		}
	}
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title"><div><p class="eyebrow">Authentication</p><h1>Identity providers</h1></div></div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if notice}<p class="notice" role="status">{notice}</p>{/if}
	{#if can(Permission.CREATE_IDENTITY_PROVIDER)}
		<form class="editor" onsubmit={(event) => { event.preventDefault(); createProvider(); }}>
			<fieldset disabled={creating}>
				<h2>Create identity provider</h2>
				<label>Name<input bind:value={name} required maxlength="64" /></label>
				<label>Slug<input bind:value={slug} required maxlength="64" pattern="[A-Za-z0-9]+" /></label>
				<label>Client ID<input bind:value={clientID} required /></label>
				<label>Issuer URL<input type="url" bind:value={issuerURL} required /></label>
				<label class="wide-field">Scopes<textarea bind:value={scopes} rows="4"></textarea></label>
				<button class="primary">Create provider</button>
			</fieldset>
		</form>
	{/if}
	{#if can(Permission.LIST_IDENTITY_PROVIDERS)}
		{#if loading}
			<p role="status">Loading identity providers…</p>
		{:else if providers.length === 0}
			<p>No identity providers.</p>
		{:else}
			<div class="table-wrap"><table><thead><tr><th>Name</th><th>Slug</th><th>Issuer</th><th>Status</th><th></th></tr></thead><tbody>
				{#each providers as provider (provider.id?.value)}
					<tr>
						<td>{provider.name}</td><td><code>{provider.slug}</code></td><td>{provider.issuerUrl}</td>
						<td><span class:ok={provider.enabled}>{provider.enabled ? 'Enabled' : 'Disabled'}</span></td>
						<td class="row-actions">
							{#if can(Permission.UPDATE_IDENTITY_PROVIDER)}<button type="button" class="quiet" onclick={() => edit(provider)} disabled={editingBusy || Boolean(rowBusy)}>Edit</button><button type="button" onclick={() => setEnabled(provider, !provider.enabled)} disabled={Boolean(rowBusy)}>{provider.enabled ? 'Disable' : 'Enable'}</button>{/if}
							{#if can(Permission.DELETE_IDENTITY_PROVIDER)}<button type="button" class="danger" onclick={() => deleteProvider(provider)} disabled={Boolean(rowBusy)}>Delete</button>{/if}
						</td>
					</tr>
				{/each}
			</tbody></table></div>
		{/if}
	{/if}
</section>

{#if editing && can(Permission.UPDATE_IDENTITY_PROVIDER)}
	<section class="card">
		<div class="section-title"><div><p class="eyebrow">Provider editor</p><h2>{editing.name}</h2></div><button type="button" class="quiet" onclick={() => editing = undefined} disabled={editingBusy}>Close</button></div>
		<form onsubmit={(event) => { event.preventDefault(); renameProvider(); }}><fieldset disabled={editingBusy}>
			<label>Name<input bind:value={editName} required maxlength="64" /></label>
			<button class="primary">Save name</button>
		</fieldset></form>
		<form class="editor" onsubmit={(event) => { event.preventDefault(); configureProvider(); }}>
			<fieldset disabled={editingBusy}>
				<label>Client ID<input bind:value={editClientID} required /></label>
				<label>Issuer URL<input type="url" bind:value={editIssuerURL} required /></label>
				<label class="wide-field">Scopes<textarea bind:value={editScopes} rows="4"></textarea></label>
				<button class="primary">Save configuration</button>
			</fieldset>
		</form>
	</section>
{/if}
