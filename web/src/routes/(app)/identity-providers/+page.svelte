<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
 import { Permission, type IdentityProvider } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { formatDate as formatTimestampDateTime } from '$lib/console';
 const { can } = consoleContext();
	import { Button } from '$lib/components/ui/button';
	import { Chip } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import {
		Key,
		Fingerprint,
		Plus,
		MoreHorizontal,
		RefreshCw,
		Trash2,
		Pencil
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { RowList, DataTablePagination, createClientListState } from '$lib/components/data-table';

	type SortKey = 'name' | 'slug';

	const table = createClientListState<IdentityProvider, SortKey>({
		load: async () => can(Permission.LIST_IDENTITY_PROVIDERS) ? (await api.listIdentityProviders({})).providers : [],
		searchFields: (p) => [p.name],
		sortKeys: ['name', 'slug'],
		sortComparators: {
			name: (a, b) => a.name.localeCompare(b.name),
			slug: (a, b) => a.slug.localeCompare(b.slug)
		},
		defaultSort: 'name'
	});

	let deleteDialogOpen = $state(false);
	let providerToDelete = $state<IdentityProvider | null>(null);

	const sortOptions = [
		{ key: 'name' as const, label: m.idp_field_name() },
		{ key: 'slug' as const, label: m.idp_field_slug() }
	];

	function confirmDelete(provider: IdentityProvider) {
		providerToDelete = provider;
		deleteDialogOpen = true;
	}

	async function deleteProvider() {
		if (!providerToDelete) return;
		try {
			await api.deleteIdentityProvider({ id: providerToDelete.id });
			table.patchRows((rows) => rows.filter((p) => p.id?.value !== providerToDelete!.id?.value));
			toast.success(m.idp_detail_deleted());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			providerToDelete = null;
		}
	}

	$effect(() =>
		registerPageSearch({
			scope: null,
			label: m.nav_identity_providers,
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
				<h1 class="truncate text-2xl font-bold">{m.idp_title()}</h1>
				<p class="text-sm text-muted-foreground">{m.idp_description()}</p>
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
				<Button disabled={!can(Permission.CREATE_IDENTITY_PROVIDER)} size="sm" onclick={() => goto('/identity-providers/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.idp_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	<RowList
		{table}
		{sortOptions}
		rowKey={(p) => (p.id?.value ?? '')}
		href={(can(Permission.GET_IDENTITY_PROVIDER) || can(Permission.LIST_IDENTITY_PROVIDERS)) ? (p) => `${base}/identity-providers/${p.id?.value ?? ''}` : undefined}
	>
		{#snippet row(provider)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<Fingerprint class="h-3.5 w-3.5 text-accent-ink" />
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{provider.name}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{provider.id?.value ?? ''}</span>
					<span class="truncate font-mono text-xs text-muted-foreground">{provider.slug}</span>
				</span>
			</span>
			<span class="flex shrink-0 items-center gap-1.5">
				<span title={m.idp_field_provider_type()}>
					<Chip tone="idle" label={m.idp_type_oidc()} />
				</span>
				<span title={m.common_status()}>
					<Chip
						tone={provider.enabled ? 'ok' : 'idle'}
						label={provider.enabled ? m.idp_enabled() : m.idp_disabled()}
					/>
				</span>
			</span>
			<span
				class="ml-auto shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
				title={m.idp_field_created()}
			>
				{formatTimestampDateTime(provider.createdAt)}
			</span>
		{/snippet}

		{#snippet rowEnd(provider)}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
							<MoreHorizontal class="h-4 w-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					<DropdownMenu.Item onclick={() => goto(`/identity-providers/${provider.id?.value ?? ''}`)}>
						<Pencil class="mr-2 h-4 w-4" />
						{m.common_edit()}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item disabled={!can(Permission.DELETE_IDENTITY_PROVIDER)} onclick={() => confirmDelete(provider)} class="text-destructive">
						<Trash2 class="mr-2 h-4 w-4" />
						{m.common_delete()}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<Key class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.idp_empty()}</h3>
				<p class="text-muted-foreground">
					{table.query ? m.common_try_different_search() : m.idp_empty_description()}
				</p>
				{#if !table.query && can(Permission.CREATE_IDENTITY_PROVIDER)}
					<Button class="mt-4" onclick={() => goto('/identity-providers/new')}>
						<Plus class="mr-2 h-4 w-4" />
						{m.idp_create()}
					</Button>
				{/if}
			</div>
		{/snippet}
	</RowList>

	<DataTablePagination {table} />
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.common_delete()}
	description={m.idp_detail_confirm_delete()}
	onconfirm={deleteProvider}
/>
