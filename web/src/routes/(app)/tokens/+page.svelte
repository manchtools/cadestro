<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient, type RegistrationToken, formatTimestampDateTime } from '$lib/sdk';
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

	// The Search RPC has no tokens scope: the list RPC returns every token
	// (including disabled ones) and the table matches / filters / sorts / pages
	// them client-side. Status is not a stored field — it is derived from
	// disabled and the derived use count, here and in the status filter alike.
	type SortKey = 'name' | 'status' | 'created';
	type Filters = { status: string[] };

	function tokenStatusId(token: RegistrationToken): string {
		if (token.disabled) return 'disabled';
		if (token.maxUses > 0 && token.currentUses >= token.maxUses) return 'exhausted';
		return 'active';
	}

	const table = createClientListState<RegistrationToken, SortKey, Filters>({
		load: async () => (await apiClient.listTokens(50, '', true)).tokens,
		searchFields: (t) => [t.name, t.id],
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
	// Creation lives on /tokens/new, a pill-committed route: a modal could not be
	// stashed, and the secret shown once by the create response would be destroyed
	// by the navigation that dismisses a dialog. This page only lists.

	const statusFilterItems = [
		{ id: 'active', label: m.tokens_status_active() },
		{ id: 'disabled', label: m.tokens_status_disabled() },
		{ id: 'exhausted', label: m.tokens_status_exhausted() },
	];

	// Headerless rows: the sort keys that were column headers now ride the row
	// list's sort bar, reusing the same labels.
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
			await apiClient.deleteToken(tokenToDelete.id);
			table.patchRows((rows) => rows.filter((t) => t.id !== tokenToDelete!.id));
			toast.success(m.tokens_deleted());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			tokenToDelete = null;
		}
	}

	async function toggleTokenDisabled(token: RegistrationToken) {
		try {
			const updated = await apiClient.setTokenDisabled(token.id, !token.disabled);
			if (updated) {
				table.patchRows((rows) => rows.map((t) => (t.id === token.id ? updated : t)));
				toast.success(updated.disabled ? m.tokens_disabled() : m.tokens_enabled());
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	// The derived status, rendered in the concepts' tone vocabulary. Same four
	// states as the filter — both read `tokenStatusId`, so they cannot drift.
	function getTokenStatus(token: RegistrationToken): { label: string; tone: FleetTone } {
		switch (tokenStatusId(token)) {
			case 'disabled':
				return { label: m.tokens_status_disabled(), tone: 'crit' };
			case 'exhausted':
				return { label: m.tokens_status_exhausted(), tone: 'warn' };
			default:
				return { label: m.tokens_status_active(), tone: 'ok' };
		}
	}

	// The query lives in the pill now: ⌘K opens search already on this page's
	// facet and its keystrokes land on the same setSearch the removed input
	// drove. These rows come from a plain list RPC, so the Search RPC has no
	// scope for them — `null` says so instead of pretending. The registration is
	// withdrawn on unmount so the next page never inherits it.
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
	<!-- The header band keeps only what acts on the page itself. The search box is
	     gone — ⌘K is the search, already scoped to this page. -->
	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="truncate text-2xl font-bold">{m.tokens_title()}</h1>
				<p class="text-sm text-muted-foreground">{m.tokens_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">
				<!-- The list's filters ride IN the list's own toolbar, next to sort:
				     narrowing a list is one act, so it reads as one bar. The page band
				     keeps only what acts on the page itself. -->
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
				<Button size="sm" onclick={() => goto('/tokens/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.tokens_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	<!-- The token list in the drafts' row grammar: key tile, name over its ULID,
		     derived-status and a right-aligned expiry
	     stamp. Registration tokens have no detail route, so the row is not a link
	     — every affordance stays in the trailing menu. -->
	<div data-tour="tokens-list">
		<RowList {table} {sortOptions} rowKey={(t) => t.id}>
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
					<span class="block truncate font-mono text-[0.66rem] text-faint">{token.id}</span>
				</span>
				<span class="flex shrink-0 items-center gap-1.5">
					<span title={m.tokens_table_status()}>
						<Chip tone={status.tone} label={status.label} />
					</span>
				</span>
				<span class="shrink-0 font-mono text-xs tabular-nums" title={m.tokens_table_uses()}>
					{token.currentUses}{#if token.maxUses > 0}&nbsp;/&nbsp;{token.maxUses}{/if}
				</span>
				<!-- One stamp keeps the row dense; created stays in the tooltip and as
				     a sort key. -->
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
						<DropdownMenu.Item onclick={() => toggleTokenDisabled(token)}>
							{#if token.disabled}
								<Check class="mr-2 h-4 w-4" />
								{m.common_enable()}
							{:else}
								<Ban class="mr-2 h-4 w-4" />
								{m.common_disable()}
							{/if}
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item onclick={() => confirmDelete(token)} class="text-destructive">
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
						<Button class="mt-4" size="sm" onclick={() => goto('/tokens/new')}>
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
