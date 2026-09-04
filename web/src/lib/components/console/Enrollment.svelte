<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import {
		CreateTokenRequestSchema,
		DeleteTokenRequestSchema,
		ListTokensRequestSchema,
		Permission,
		RenameTokenRequestSchema,
		type RegistrationToken
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage } from '$lib/api';
	import { cursorHref, formatDate } from '$lib/console';

	let { can }: { can: (permission: Permission) => boolean } = $props();
	let tokens = $state<RegistrationToken[]>([]);
	let totalCount = $state(0);
	let nextPageToken = $state('');
	let loading = $state(false);
	let creating = $state(false);
	let editingBusy = $state(false);
	let deleting = $state('');
	let error = $state('');
	let notice = $state('');
	let name = $state('Enrollment');
	let maxUses = $state(0);
	const defaultExpiry = new Date(Date.now() + 7 * 86400_000);
	let expiry = $state(new Date(defaultExpiry.getTime() - defaultExpiry.getTimezoneOffset() * 60_000).toISOString().slice(0, 16));
	let revealedToken = $state('');
	let revealedPin = $state('');
	let editing = $state<RegistrationToken>();
	let editedName = $state('');
	const pageToken = $derived(page.url.searchParams.get('tokensCursor') ?? '');

	async function load(): Promise<boolean> {
		if (!can(Permission.LIST_TOKENS)) return true;
		loading = true;
		error = '';
		try {
			const response = await api.listTokens(create(ListTokensRequestSchema, { pageSize: 50, pageToken }));
			tokens = response.tokens;
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

	async function refreshAfterSaved(message: string) {
		const refreshed = await load();
		notice = refreshed ? message : `${message} Refresh failed; the displayed state may be stale.`;
	}

	async function createToken() {
		creating = true;
		error = '';
		notice = '';
		try {
			const response = await api.createToken(create(CreateTokenRequestSchema, { name, maxUses, expiresAt: timestampFromDate(new Date(expiry)) }));
			revealedToken = response.token?.value ?? '';
			revealedPin = response.caFingerprintPin;
			name = 'Enrollment';
			maxUses = 0;
			await refreshAfterSaved('Token created.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			creating = false;
		}
	}

	async function renameToken() {
		if (!editing?.id) return;
		editingBusy = true;
		error = '';
		notice = '';
		try {
			await api.renameToken(create(RenameTokenRequestSchema, { id: editing.id, name: editedName }));
			editing = undefined;
			await refreshAfterSaved('Token renamed.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			editingBusy = false;
		}
	}

	async function deleteToken(token: RegistrationToken) {
		if (!token.id || !confirm(`Delete ${token.name}?`)) return;
		deleting = token.id.value;
		error = '';
		notice = '';
		try {
			await api.deleteToken(create(DeleteTokenRequestSchema, { id: token.id }));
			await refreshAfterSaved('Token deleted.');
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			deleting = '';
		}
	}
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title"><div><p class="eyebrow">Enrollment</p><h1>Registration tokens</h1></div>{#if can(Permission.LIST_TOKENS)}<span>{totalCount} tokens</span>{/if}</div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if notice}<p class="notice" role="status">{notice}</p>{/if}
	{#if can(Permission.CREATE_TOKEN)}
		<form onsubmit={(event) => { event.preventDefault(); createToken(); }}>
			<fieldset disabled={creating}>
				<label>Name<input bind:value={name} required maxlength="128" /></label>
				<label>Maximum uses<input type="number" bind:value={maxUses} min="0" /><small>0 allows unlimited uses until expiry.</small></label>
				<label>Expires<input type="datetime-local" bind:value={expiry} required /></label>
				<button class="primary">Create token</button>
			</fieldset>
		</form>
	{/if}
	{#if revealedToken}<div class="secret"><strong>Save this token now</strong><code>{revealedToken}</code><small>CA fingerprint pin</small><code>{revealedPin}</code></div>{/if}
	{#if can(Permission.LIST_TOKENS)}
		{#if loading}
			<p role="status">Loading registration tokens…</p>
		{:else if tokens.length === 0}
			<p>No registration tokens.</p>
		{:else}
			<div class="table-wrap"><table><thead><tr><th>Name</th><th>Uses</th><th>Expires</th><th></th></tr></thead><tbody>
				{#each tokens as token (token.id?.value)}
					<tr><td>{token.name}</td><td>{token.currentUses}/{token.maxUses || '∞'}</td><td>{formatDate(token.expiresAt)}</td><td class="row-actions">
						{#if can(Permission.RENAME_TOKEN)}<button type="button" class="quiet" onclick={() => { editing = token; editedName = token.name; }} disabled={editingBusy || Boolean(deleting)}>Rename</button>{/if}
						{#if can(Permission.DELETE_TOKEN)}<button type="button" class="danger" onclick={() => deleteToken(token)} disabled={Boolean(deleting)}>Delete</button>{/if}
					</td></tr>
				{/each}
			</tbody></table></div>
		{/if}
		<nav class="pagination" aria-label="Registration token pages">{#if pageToken}<a class="button quiet" href={cursorHref(page.url, 'tokensCursor', '')}>First page</a>{/if}{#if nextPageToken}<a class="button" href={cursorHref(page.url, 'tokensCursor', nextPageToken)}>Next page</a>{/if}</nav>
	{/if}
	{#if editing && can(Permission.RENAME_TOKEN)}
		<form class="editor" onsubmit={(event) => { event.preventDefault(); renameToken(); }}><fieldset disabled={editingBusy}>
			<h2>Rename token</h2>
			<label>Name<input bind:value={editedName} required maxlength="128" /></label>
			<button class="primary">Save name</button>
			<button type="button" class="quiet" onclick={() => editing = undefined}>Cancel</button>
		</fieldset></form>
	{/if}
</section>
