<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { api } from '$lib/api';
 import { type RegistrationToken, Permission } from '$contract/cadestro/v1/control_pb';
 import { collectPages, formatDate as formatTimestampDateTime } from '$lib/console';
 import { consoleContext } from '$lib/console-context.svelte';
 import * as Dialog from '$lib/components/ui/dialog';
 import { Input } from '$lib/components/ui/input';
 import { Label } from '$lib/components/ui/label';
 const { can } = consoleContext();
 let renaming = $state<RegistrationToken | null>(null);
 let rename = $state('');
 let renameOpen = $state(false);
 async function saveName() { if (!renaming || !can(Permission.RENAME_TOKEN)) return; try { await api.renameToken({ id: renaming.id, name: rename }); renameOpen = false; table.refresh(); } catch (error) { toast.error(getLocalizedError(error)); } }
	import { Button } from '$lib/components/ui/button';
	import { Chip } from '$lib/components/fleet';
	import type { FleetTone } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
	import { Key, Plus, MoreHorizontal, RefreshCw, Trash2, Ban, Check } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { codecs } from '$lib/url-state';
	import { RowList, DataTablePagination, createClientListState } from '$lib/components/data-table';

	type SortKey = 'name' | 'status' | 'created';
	type Filters = { status: string[] };

	function tokenStatusId(token: RegistrationToken): string {
		if (token.expiresAt && Number(token.expiresAt.seconds) * 1000 <= Date.now()) return 'expired';
		if (token.maxUses > 0 && token.currentUses >= token.maxUses) return 'exhausted';
		return 'active';
	}

	const table = createClientListState<RegistrationToken, SortKey, Filters>({
		load: async () => can(Permission.LIST_TOKENS) ? await collectPages(async (pageToken) => { const r = await api.listTokens({ pageSize: 100, pageToken }); return { items: r.tokens, nextPageToken: r.nextPageToken }; }) : [],
		searchFields: (t) => [t.name, (t.id?.value ?? '')],
		sortKeys: ['name', 'status', 'created'],
		sortComparators: {
			name: (a, b) => a.name.localeCompare(b.name),
			status: (a, b) => tokenStatusId(a).localeCompare(tokenStatusId(b)),
			created: (a, b) => Number((a.createdAt?.seconds ?? 0n) - (b.createdAt?.seconds ?? 0n))
		},
		defaultSort: 'created',
		sortDir: (key) => (key === 'created' ? 'desc' : 'asc'),
		filters: { status: { key: 'status', codec: codecs.stringArray([]) } },
		filterRow: (t, f) => f.status.length === 0 || f.status.includes(tokenStatusId(t))
	});

	let deleteDialogOpen = $state(false);
	let tokenToDelete = $state<RegistrationToken | null>(null);

	const statusFilterItems = [
		{ id: 'active', label: m.tokens_status_active() },
		{ id: 'expired', label: 'Expired' },
		{ id: 'exhausted', label: m.tokens_status_exhausted() },
	];

	const sortOptions = [
		{ key: 'name' as const, label: m.tokens_table_name() },
		{ key: 'status' as const, label: m.tokens_table_status() },
		{ key: 'created' as const, label: m.tokens_table_created() }
	];

	const hasNarrowedView = $derived(
		 table.query !== '' || table.filters.status.length > 0
	);

	function confirmDelete(token: RegistrationToken) {
		tokenToDelete = token;
		deleteDialogOpen = true;
	}

	async function deleteToken() {
		if (!tokenToDelete) return;
		try {
			await api.deleteToken({ id: tokenToDelete.id });
			table.patchRows((rows) => rows.filter((t) => t.id?.value !== tokenToDelete!.id?.value));
			toast.success(m.tokens_deleted());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			tokenToDelete = null;
		}
	}

	function getTokenStatus(token: RegistrationToken): { label: string; tone: FleetTone } {
		switch (tokenStatusId(token)) {
			case 'expired':
				return { label: 'Expired', tone: 'crit' };
			case 'exhausted':
				return { label: m.tokens_status_exhausted(), tone: 'warn' };
			default:
				return { label: m.tokens_status_active(), tone: 'ok' };
		}
	}

	$effect(() =>
		registerPageSearch({
			scope: null,
			label: m.nav_tokens,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);
</script>

<PageShell contentClass="space-y-4">

	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="truncate text-2xl font-bold">{m.tokens_title()}</h1>
				<p class="text-sm text-muted-foreground">{m.tokens_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">

				<Button
					onclick={() => table.refresh()}
					variant="outline"
					size="sm"
					disabled={table.loading}
				>
					<span class="mr-2 h-4 w-4" class:animate-spin={table.loading}>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
				<Button disabled={!can(Permission.CREATE_TOKEN)} size="sm" onclick={() => goto('/tokens/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.tokens_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	<div data-tour="tokens-list">
		<RowList {table} {sortOptions} rowKey={(t) => (t.id?.value ?? '')}>
			{#snippet filters()}
				<MultiSelectCombobox
					items={statusFilterItems}
					selected={table.filters.status}
					onSelectedChange={(next) => table.setFilter('status', next)}
					placeholder={m.tokens_filter_all_statuses()}
					searchPlaceholder={m.common_search()}
					class="w-44"
				/>
			{/snippet}
			{#snippet row(token)}
				{@const status = getTokenStatus(token)}
				<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
					<Key class="h-3.5 w-3.5 text-accent-ink" />
				</div>
				<span class="min-w-0">
					<span class="block truncate font-mono text-sm font-semibold">{token.name}</span>
					<span class="block truncate font-mono text-[0.66rem] text-faint">{token.id?.value ?? ''}</span>
				</span>
				<span class="flex shrink-0 items-center gap-1.5">
					<span title={m.tokens_table_status()}>
						<Chip tone={status.tone} label={status.label} />
					</span>
				</span>
				<span class="shrink-0 font-mono text-xs tabular-nums" title={m.tokens_table_uses()}>
					{token.currentUses}{#if token.maxUses > 0}&nbsp;/&nbsp;{token.maxUses}{/if}
				</span>

				<span
					class="ml-auto shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
					 title="{m.tokens_table_expires()}: {formatTimestampDateTime(token.expiresAt)} · {m.tokens_table_created()}: {formatTimestampDateTime(
						token.createdAt
					)}"
				>
					{formatTimestampDateTime(token.expiresAt)}
				</span>
			{/snippet}

			{#snippet rowEnd(token)}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
								<MoreHorizontal class="h-4 w-4" />
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end">
                        <DropdownMenu.Item disabled={!can(Permission.RENAME_TOKEN)} onclick={() => { renaming = token; rename = token.name; renameOpen = true; }}>Rename</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item disabled={!can(Permission.DELETE_TOKEN)} onclick={() => confirmDelete(token)} class="text-destructive">
							<Trash2 class="mr-2 h-4 w-4" />
							{m.common_delete()}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{/snippet}

			{#snippet empty()}
				<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
					<Key class="mb-4 h-10 w-10 text-faint" />
					<h3 class="text-sm font-semibold">{m.tokens_empty()}</h3>
					<p class="text-sm text-muted-foreground">
						{hasNarrowedView ? m.common_try_different_search() : m.tokens_empty_hint()}
					</p>
					{#if !hasNarrowedView}
						<Button disabled={!can(Permission.CREATE_TOKEN)} class="mt-4" size="sm" onclick={() => goto('/tokens/new')}>
							<Plus class="mr-2 h-4 w-4" />
							{m.tokens_create()}
						</Button>
					{/if}
				</div>
			{/snippet}
		</RowList>
	</div>

	<DataTablePagination {table} />
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.tokens_delete_dialog_title()}
	description={m.tokens_delete_dialog_description({ name: tokenToDelete?.name ?? '' })}
	onconfirm={deleteToken}
/>

<Dialog.Root bind:open={renameOpen}><Dialog.Content><Dialog.Header><Dialog.Title>Rename token</Dialog.Title><Dialog.Description>{renaming?.name}</Dialog.Description></Dialog.Header><form onsubmit={(event) => { event.preventDefault(); void saveName(); }}><Label for="rename-token">Name</Label><Input id="rename-token" bind:value={rename} required /><Dialog.Footer><Button type="submit">Save</Button></Dialog.Footer></form></Dialog.Content></Dialog.Root>
