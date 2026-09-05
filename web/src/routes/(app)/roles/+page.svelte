<script lang="ts">
	import { collectPages } from '$lib/console';
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import { Permission, type Role } from '$contract/cadestro/v1/control_pb';
	import { consoleContext } from '$lib/console-context.svelte';
	const { can } = consoleContext();
	import { Button } from '$lib/components/ui/button';
	import { Chip } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import { Shield, Plus, MoreHorizontal, RefreshCw, Trash2 } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { RowList, DataTablePagination, createClientListState } from '$lib/components/data-table';

	type SortKey = 'name' | 'permissions';

	const table = createClientListState<Role, SortKey>({
		load: async () => can(Permission.LIST_ROLES) ? await collectPages(async (pageToken) => { const r = await api.listRoles({ pageSize: 100, pageToken }); return { items: r.roles, nextPageToken: r.nextPageToken }; }) : [],
		searchFields: (r) => [r.name],
		sortKeys: ['name', 'permissions'],
		sortComparators: {
			name: (a, b) => a.name.localeCompare(b.name),
			permissions: (a, b) => a.permissions.length - b.permissions.length
		},
		defaultSort: 'name'
	});

	let deleteDialogOpen = $state(false);
	let roleToDelete = $state<Role | null>(null);

	const sortOptions = [
		{ key: 'name' as const, label: m.roles_name() },
		{ key: 'permissions' as const, label: m.roles_permission_count() }
	];

	function confirmDelete(role: Role) {

		roleToDelete = role;
		deleteDialogOpen = true;
	}

	async function deleteRole() {
		if (!roleToDelete) return;
		try {
			await api.deleteRole({ id: roleToDelete.id });
			table.patchRows((rows) => rows.filter((r) => r.id?.value !== roleToDelete!.id?.value));
			toast.success(m.roles_deleted());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			roleToDelete = null;
		}
	}

	$effect(() =>
		registerPageSearch({
			scope: null,
			label: m.nav_roles,
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
				<h1 class="truncate text-2xl font-bold">{m.roles_title()}</h1>
				<p class="text-sm text-muted-foreground">{m.roles_description()}</p>
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
				<Button disabled={!can(Permission.CREATE_ROLE)} size="sm" onclick={() => goto('/roles/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.roles_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	<RowList {table} {sortOptions} rowKey={(r) => (r.id?.value ?? '')} href={(can(Permission.GET_ROLE) || can(Permission.LIST_ROLES)) ? (r) => `${base}/roles/${r.id?.value ?? ''}` : undefined}>
		{#snippet row(role)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<Shield class="h-3.5 w-3.5 text-accent-ink" />
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{role.name}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{role.id?.value ?? ''}</span>
					<span class="truncate text-xs text-muted-foreground">
						{role.description || m.common_no_description()}
					</span>
				</span>
			</span>
			<span class="ml-auto flex shrink-0 items-center gap-1.5">

				<span title={m.roles_permission_count()}>
					<Chip
						tone="idle"
						label={m.roles_index_permissions_count({ count: role.permissions.length })}
					/>
				</span>
			</span>
		{/snippet}

		{#snippet rowEnd(role)}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
							<MoreHorizontal class="h-4 w-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					<DropdownMenu.Item disabled={!(can(Permission.GET_ROLE) || can(Permission.LIST_ROLES))} onclick={() => goto(`/roles/${role.id?.value ?? ''}`)}>
						<Shield class="mr-2 h-4 w-4" />
						{m.roles_edit()}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item
						onclick={() => confirmDelete(role)}
						class="text-destructive"
						disabled={!can(Permission.DELETE_ROLE)}
					>
						<Trash2 class="mr-2 h-4 w-4" />
						{m.common_delete()}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<Shield class="mb-4 h-10 w-10 text-faint" />
				<h3 class="text-sm font-semibold">{m.roles_no_roles()}</h3>
				<p class="text-sm text-muted-foreground">
					{table.query ? m.common_try_different_search() : m.roles_empty_hint()}
				</p>
				{#if !table.query}
					<Button disabled={!can(Permission.CREATE_ROLE)} class="mt-4" size="sm" onclick={() => goto('/roles/new')}>
						<Plus class="mr-2 h-4 w-4" />
						{m.roles_create()}
					</Button>
				{/if}
			</div>
		{/snippet}
	</RowList>

	<DataTablePagination {table} />
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.roles_delete()}
	description={m.roles_delete_confirm()}
	onconfirm={deleteRole}
/>
