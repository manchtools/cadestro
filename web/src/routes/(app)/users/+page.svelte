<script lang="ts">
 import { onMount } from 'svelte';
 import { toast } from 'svelte-sonner';
 import { parseIds } from '$lib/id-list';
 import { addMemberships } from '$lib/membership';
 import { api, logout } from '$lib/api';
 import { goto } from '$lib/navigation';
 import { Permission, type User, type Role } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { collectPages, formatDate as formatTimestampDateTime } from '$lib/console';
 import { RowList, DataTablePagination, createClientListState } from '$lib/components/data-table';
 import { Button } from '$lib/components/ui/button';
 import { Input } from '$lib/components/ui/input';
 import { Chip } from '$lib/components/fleet';
 import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
 import * as Dialog from '$lib/components/ui/dialog';
 import * as Table from '$lib/components/ui/table';
 import ItemTablePicker from '$lib/components/item-table-picker.svelte';
 import PageShell from '$lib/components/page-shell.svelte';
 import { Users, MoreHorizontal, RefreshCw } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
 import { getLocalizedError } from '$lib/errors';
 import { registerPageSearch } from '$lib/shell/page-search.svelte';
 const { can, currentUser } = consoleContext();
 const table = createClientListState<User, 'email' | 'created' | 'lastLogin'>({
  load: async () => can(Permission.LIST_USERS) ? collectPages(async pageToken => { const r = await api.listUsers({ pageSize: 100, pageToken }); return { items: r.users, nextPageToken: r.nextPageToken }; }) : [],
  searchFields: user => [user.email, user.displayName], sortKeys: ['email', 'created', 'lastLogin'], defaultSort: 'email',
  sortComparators: { email: (a,b) => a.email.localeCompare(b.email), created: (a,b) => Number((a.createdAt?.seconds ?? 0n) - (b.createdAt?.seconds ?? 0n)), lastLogin: (a,b) => Number((a.lastLoginAt?.seconds ?? 0n) - (b.lastLoginAt?.seconds ?? 0n)) }
 });
 const sortOptions = [{ key: 'email' as const, label: m.users_table_email() }, { key: 'created' as const, label: m.users_table_created() }, { key: 'lastLogin' as const, label: m.users_table_last_login() }];
 let roleDialogUser = $state<User | null>(null);
 let roleDialogOpen = $state(false);
 let roles = $state<Role[]>([]);
 let selectedRoleIds = $state<string[]>([]);
 let roleIdsText = $state('');
 let assigning = $state(false);
 let assignmentReady = $state(true);
 function displayNameOf(user: User) { return user.displayName || '—'; }
 function initialsOf(user: User) { return (user.displayName || user.email).split(/[\s._-]+/).slice(0,2).map(part => part[0].toUpperCase()).join(''); }
 async function revokeRole(user: User, id: string) { try { await api.revokeRoleFromUser({ userId: user.id, roleId: { value: id } }); table.refresh(); } catch(error) { toast.error(getLocalizedError(error)); } }
 async function assignRoles() {
  if (!roleDialogUser || assigning || !assignmentReady) return;
  const userId = roleDialogUser.id;
  assigning = true;
  const result = await addMemberships(selectedRoleIds, id => api.assignRoleToUser({ userId, roleId: { value: id } }), async () => {
   const users = await collectPages(async pageToken => { const r = await api.listUsers({ pageSize: 100, pageToken }); return { items: r.users, nextPageToken: r.nextPageToken }; });
   const current = users.find(user => user.id?.value === userId?.value);
   if (!current) throw new Error('The latest user could not be loaded');
   roleDialogUser = current;
   table.patchRows(rows => rows.map(user => user.id?.value === userId?.value ? current : user));
   return current.roles.map(role => role.id?.value ?? '');
  });
  selectedRoleIds = result.remaining; roleIdsText = result.remaining.join(',');
  assignmentReady = result.ready;
  assigning = false;
  if (result.error) toast.error(`${getLocalizedError(result.error)}${result.ready ? '' : ' Refresh the user before assigning again.'}`);
  else { roleDialogOpen = false; table.refresh(); }
 }

 async function revokeSessions(user: User) { try { await api.revokeUserSessions({ userId: user.id }); if (user.id?.value === currentUser()?.id?.value) { await logout(); await goto('/login'); } else toast.success('Sessions revoked'); } catch(error) { toast.error(getLocalizedError(error)); } }
 onMount(async () => { if (!can(Permission.LIST_ROLES)) return; try { roles = await collectPages(async pageToken => { const r = await api.listRoles({ pageSize: 100, pageToken }); return { items: r.roles, nextPageToken: r.nextPageToken }; }); } catch(error) { toast.error(getLocalizedError(error)); } });
 $effect(() => registerPageSearch({ scope: null, label: m.nav_users, get query() { return table.query; }, setQuery: table.setSearch, clear: () => table.setSearch('') }));
</script>
<PageShell contentClass="space-y-4">

	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="truncate text-2xl font-bold">{m.users_title()}</h1>
				<p class="text-sm text-muted-foreground">{m.users_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">

				<Button onclick={() => table.refresh()} variant="outline" size="sm" disabled={table.loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={table.loading}>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>

		<p class="text-sm text-muted-foreground">{m.users_provisioning_hint()}</p>
	{/snippet}

	<RowList {table} {sortOptions} rowKey={(u) => (u.id?.value ?? '')}>

		{#snippet row(user)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<span class="font-mono text-[0.6rem] font-semibold text-accent-ink">
					{initialsOf(user)}
				</span>
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{user.email}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{user.id?.value ?? ''}</span>
					<span class="truncate text-xs text-muted-foreground">{displayNameOf(user)}</span>
				</span>
			</span>
			<span class="flex shrink-0 items-center gap-1.5">
				{#if user.id?.value === currentUser()?.id?.value}
					<Chip tone="info" label={m.users_you()} />
				{/if}

			</span>

			<span class="flex min-w-0 items-center gap-1.5 overflow-hidden" title={m.users_table_role()}>
				{#each user.roles as role}
					<Chip tone={role.name === 'Admin' ? 'info' : 'idle'} label={role.name} />
				{/each}

			</span>

			<span
				class="ml-auto shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
				title="{m.users_table_last_login()}: {formatTimestampDateTime(
					user.lastLoginAt
				)} · {m.users_table_created()}: {formatTimestampDateTime(user.createdAt)}"
			>
				{formatTimestampDateTime(user.lastLoginAt)}
			</span>
		{/snippet}

        {#snippet rowEnd(user)}
        <DropdownMenu.Root><DropdownMenu.Trigger>{#snippet child({ props })}<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}><MoreHorizontal class="h-4 w-4" /></Button>{/snippet}</DropdownMenu.Trigger>
         <DropdownMenu.Content align="end">
          {#if can(Permission.ASSIGN_ROLE_TO_USER)}<DropdownMenu.Item onclick={() => { roleDialogUser = user; roleDialogOpen = true; selectedRoleIds = []; roleIdsText = ''; assignmentReady = true; }}>{m.roles_assign_to_user()}</DropdownMenu.Item>{/if}
          {#if can(Permission.REVOKE_ROLE_FROM_USER)}{#each user.roles as role}<DropdownMenu.Item onclick={() => revokeRole(user, role.id?.value ?? '')}>{m.roles_revoke_from_user()}: {role.name}</DropdownMenu.Item>{/each}{/if}
          {#if can(Permission.REVOKE_USER_SESSIONS)}<DropdownMenu.Item onclick={() => revokeSessions(user)}>Revoke sessions</DropdownMenu.Item>{/if}
         </DropdownMenu.Content></DropdownMenu.Root>
        {/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<Users class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.users_empty()}</h3>
				<p class="text-muted-foreground">
					{table.query
						? m.common_try_different_search()
						: m.users_provisioning_hint()}
				</p>
			</div>
		{/snippet}
	</RowList>

	<DataTablePagination {table} />
</PageShell>

<Dialog.Root bind:open={roleDialogOpen}><Dialog.Content class="max-w-2xl"><Dialog.Header><Dialog.Title>{m.roles_assign_to_user()}</Dialog.Title><Dialog.Description>{roleDialogUser?.email}</Dialog.Description></Dialog.Header>
 {#if can(Permission.LIST_ROLES)}<ItemTablePicker items={roles.filter(role => !roleDialogUser?.roles.some(assigned => assigned.id?.value === role.id?.value)).map(role => ({ id: role.id?.value ?? '', name: role.name, description: role.description }))} bind:selected={selectedRoleIds} searchPlaceholder={m.picker_search_roles()} emptyMessage={m.picker_no_roles()} searchFilter={(role, query) => role.name.toLowerCase().includes(query.toLowerCase())}>
 {#snippet headerRow()}<Table.Head>{m.roles_name()}</Table.Head><Table.Head>{m.roles_description_field()}</Table.Head>{/snippet}
 {#snippet itemRow(role)}<Table.Cell>{role.name}</Table.Cell><Table.Cell>{role.description}</Table.Cell>{/snippet}
 </ItemTablePicker>{:else}<Input aria-label="Role IDs" value={roleIdsText} oninput={event => { roleIdsText = event.currentTarget.value; selectedRoleIds = parseIds(roleIdsText); }} />{/if}
 <Dialog.Footer><Button variant="outline" onclick={() => { roleDialogOpen = false; }}>{m.common_cancel()}</Button><Button disabled={!selectedRoleIds.length || assigning || !assignmentReady} onclick={assignRoles}>{m.common_assign()}</Button></Dialog.Footer>
</Dialog.Content></Dialog.Root>
