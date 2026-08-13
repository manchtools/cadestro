<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import { apiClient, type Role } from '$lib/sdk';
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

	// The Search RPC has no roles scope, so the list RPC returns every role and
	// the table matches / sorts / pages them client-side.
	type SortKey = 'name' | 'permissions';

	const table = createClientListState<Role, SortKey>({
		load: async () => (await apiClient.listRoles()).roles,
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

	// Creation lives on /roles/new, a pill-committed route: a modal cannot be
	// stashed, so a half-written role could never be parked and picked up again.

	// No assigned-to reading: Role carries no grant count on the wire, and an
	// invented number is worse than an absent one.
	//
	// Headerless rows: the sort keys that were column headers now ride the row
	// list's sort bar, reusing the same labels.
	const sortOptions = [
		{ key: 'name' as const, label: m.roles_name() },
		{ key: 'permissions' as const, label: m.roles_permission_count() }
	];

	function confirmDelete(role: Role) {
		// System roles are server-refused; say so before opening the dialog.
		if (role.isSystem) {
			toast.error(m.roles_cannot_delete_system());
			return;
		}
		roleToDelete = role;
		deleteDialogOpen = true;
	}

	async function deleteRole() {
		if (!roleToDelete) return;
		try {
			await apiClient.deleteRole(roleToDelete.id);
			table.patchRows((rows) => rows.filter((r) => r.id !== roleToDelete!.id));
			toast.success(m.roles_deleted());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			roleToDelete = null;
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
	<!-- ONE toolbar line: the filters ride the header beside Refresh/Create. The
	     search box is gone — ⌘K is the search, already scoped to this page. -->
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
				<Button size="sm" onclick={() => goto('/roles/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.roles_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	<!-- The role list in the drafts' row grammar: shield tile, name over its ULID
	     and description, system + permission-count chips — no headers, no table.
	     Roles carry no timestamp, so the chips take the trailing slot. -->
	<RowList {table} {sortOptions} rowKey={(r) => r.id} href={(r) => `${base}/roles/${r.id}`}>
		{#snippet row(role)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<Shield class="h-3.5 w-3.5 text-accent-ink" />
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{role.name}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{role.id}</span>
					<span class="truncate text-xs text-muted-foreground">
						{role.description || m.common_no_description()}
					</span>
				</span>
			</span>
			<span class="ml-auto flex shrink-0 items-center gap-1.5">
				{#if role.isSystem}
					<Chip tone="info" label={m.roles_system_badge()} />
				{/if}
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
					<DropdownMenu.Item onclick={() => goto(`/roles/${role.id}`)}>
						<Shield class="mr-2 h-4 w-4" />
						{m.roles_edit()}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item
						onclick={() => confirmDelete(role)}
						class="text-destructive"
						disabled={role.isSystem}
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
					<Button class="mt-4" size="sm" onclick={() => goto('/roles/new')}>
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
