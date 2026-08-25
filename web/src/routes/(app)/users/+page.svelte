<script lang="ts">
	import { base } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		authStore,
		type User,
		type IdentityLink,
		formatTimestampDateTime
	} from '$lib/sdk';
	import { SearchScope, SortField, RoleGrantScopeKind } from '$contract/cadestro/v1/common_pb';
	import { codecs } from '$lib/url-state';
	import {
		searchResultToUser,
		searchResultUserRoles,
		type SearchRoleRef
	} from '$lib/search-adapters';
	import { RowList, DataTablePagination, createSearchListState } from '$lib/components/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Chip } from '$lib/components/fleet';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import PageShell from '$lib/components/page-shell.svelte';
	import RoleAssignDialog from '$lib/components/role-assign-dialog.svelte';
	import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
	import { FieldError } from '$lib/components/ui/field-error';
	import {
		Users,
		MoreHorizontal,
		RefreshCw,
		Trash2,
		Ban,
		Check,
		Shield,
		UserPlus,
		UsersRound,
		Pencil,
		Unlink,
		Globe
	} from '@lucide/svelte';
	import { createFormValidation } from '$lib/forms';
	import { updateUserEmailSchema } from '$lib/forms/schemas/users';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';

	// Issue #84/#325. The list is a scoped PostgreSQL search query; sort and
	// pagination are server-side.
	type SortKey = 'email' | 'status' | 'created' | 'lastLogin';

	/** The row: the typed User plus the document's role chips. Grant SCOPE
	 *  KINDS are not in the document, so the row's typed `roleGrants` stay
	 *  EMPTY on purpose — everything scope-aware (revoke, the assign dialog's
	 *  unscoped-exclusion) reads the full GetUser record instead of consuming
	 *  a fabricated UNSPECIFIED scope. */
	type UserRow = User & { docRoles: { direct: SearchRoleRef[]; inherited: SearchRoleRef[] } };

	const table = createSearchListState<UserRow, SortKey, { status: string[] }>({
		scope: SearchScope.USERS,
		adapter: (r) => Object.assign(searchResultToUser(r), { docRoles: searchResultUserRoles(r) }),
		sortKeys: ['email', 'status', 'created', 'lastLogin'],
		defaultSort: 'email',
		// Subset of the server's scopeSortableFields["users"]; the "status"
		// column is the active/disabled flag → the `disabled` field.
		sortFieldMap: {
			email: SortField.EMAIL,
			status: SortField.DISABLED,
			created: SortField.CREATED_AT,
			lastLogin: SortField.LAST_LOGIN_AT
		},
		sortDir: (key) => (key === 'created' || key === 'lastLogin' ? 'desc' : 'asc'),
		filters: { status: { key: 'status', codec: codecs.stringArray([]) } },
		// The indexed `disabled` TAG takes one value: active = false,
		// disabled = true. Selecting both (or neither) means no filter.
		filterToTags: (f) =>
			f.status.length === 1 ? { disabled: String(f.status[0] === 'disabled') } : undefined
	});

	let eraseDialogOpen = $state(false);
	let userToErase = $state<User | null>(null);
	let roleDialogOpen = $state(false);
	let roleDialogUser = $state<User | null>(null);

	// Edit email dialog
	let editEmailDialogOpen = $state(false);
	let editEmailUser = $state<User | null>(null);
	let editEmail = $state('');

	// Identity links dialog
	let identityLinksDialogOpen = $state(false);
	let identityLinksUser = $state<User | null>(null);
	let identityLinks = $state<IdentityLink[]>([]);
	let identityLinksLoading = $state(false);
	let unlinkConfirmOpen = $state(false);
	let unlinkingLinkId = $state('');

	const emailFv = createFormValidation(updateUserEmailSchema);

	const statusFilterItems = [
		{ id: 'active', label: m.common_active() },
		{ id: 'disabled', label: m.common_disabled() }
	];

	// Headerless rows: the sort keys that were column headers now ride the row
	// list's sort bar, reusing the same labels.
	const sortOptions = [
		{ key: 'email' as const, label: m.users_table_email() },
		{ key: 'status' as const, label: m.users_table_status() },
		{ key: 'created' as const, label: m.users_table_created() },
		{ key: 'lastLogin' as const, label: m.users_table_last_login() }
	];

	function displayNameOf(user: User): string {
		return user.displayName || [user.givenName, user.familyName].filter(Boolean).join(' ') || '—';
	}

	// Avatar tile: initials from the profile name, falling back to the email's
	// local part. Never fabricates a name — it only abbreviates one already shown.
	function initialsOf(user: User): string {
		const named = user.displayName || [user.givenName, user.familyName].filter(Boolean).join(' ');
		const parts = (named || user.email.split('@')[0] || '').split(/[\s._-]+/).filter(Boolean);
		return (
			parts
				.slice(0, 2)
				.map((p) => p.charAt(0).toUpperCase())
				.join('') || '?'
		);
	}

	function confirmErase(user: User) {
		userToErase = user;
		eraseDialogOpen = true;
	}

	// The server refuses SCIM-managed subjects with `scim_managed_resource`; the
	// row stays because only a resolved erase removes it.
	async function eraseUser() {
		if (!userToErase) return;

		try {
			await apiClient.eraseJITUser((userToErase.id?.value ?? ''));
			table.patchRows((rows) => rows.filter((u) => u.id !== userToErase!.id));
			toast.success(m.users_erased());
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			eraseDialogOpen = false;
			userToErase = null;
		}
	}

	async function toggleUserDisabled(user: User) {
		try {
			const updated = await apiClient.setUserDisabled((user.id?.value ?? ''), !user.disabled);
			if (updated) {
				// The RPC returns a typed User; the row keeps its document chips.
				table.patchRows((rows) =>
					rows.map((u) => (u.id === user.id ? Object.assign(updated, { docRoles: u.docRoles }) : u))
				);
				toast.success(updated.disabled ? m.users_disabled() : m.users_enabled());
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function openRoleDialog(user: User) {
		// The dialog opens on the row for immediate feedback, then swaps in the
		// full GetUser record: the row's typed grants are empty (the document has
		// no scope kinds), so only the detail record can say which roles are
		// already granted unscoped.
		roleDialogUser = user;
		roleDialogOpen = true;
		try {
			const detail = await apiClient.getUser((user.id?.value ?? ''));
			if (detail) roleDialogUser = detail;
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function revokeRole(user: User, roleId: string) {
		try {
			// Scope kinds are NOT in the search document. The revoke resolves the
			// role's REAL grants from GetUser and revokes each with its true scope,
			// never a fabricated UNSPECIFIED one.
			const detail = await apiClient.getUser((user.id?.value ?? ''));
			const grants = (detail?.roleGrants ?? []).filter((grant) => grant.role?.id?.value === roleId);
			for (const grant of grants) {
				await apiClient.revokeRoleFromUser((user.id?.value ?? ''), roleId, grant.scopeKind, grant.scopeId?.value);
			}
			if (grants.length > 0) toast.success(m.roles_revoked());
			table.refresh();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	/** One revoke item per DISTINCT direct role. `isSystem` is not in the
	 *  document; the system User role is the contract's FIXED id, so filtering
	 *  it by id fabricates nothing. (Admin, also a system role, stays revocable
	 *  — exactly as the grant-based menu behaved.) */
	const SYSTEM_ROLE_USER_ID = '00000000000000000000000002';
	function revocableRoles(user: UserRow): SearchRoleRef[] {
		const seen = new Set<string>();
		return user.docRoles.direct.filter((role) => {
			if (role.id === SYSTEM_ROLE_USER_ID || seen.has(role.id)) return false;
			seen.add(role.id);
			return true;
		});
	}

	function openEditEmailDialog(user: User) {
		editEmailUser = user;
		editEmail = user.email;
		emailFv.clearErrors();
		editEmailDialogOpen = true;
	}

	async function updateEmail() {
		await emailFv.handleSubmit({ email: editEmail }, async () => {
			if (!editEmailUser) return;
			try {
				const updated = await apiClient.updateUserEmail((editEmailUser.id?.value ?? ''), editEmail);
				if (updated) {
					// The RPC returns a typed User; the row keeps its document chips.
					table.patchRows((rows) =>
						rows.map((u) =>
							u.id === editEmailUser!.id ? Object.assign(updated, { docRoles: u.docRoles }) : u
						)
					);
					editEmailDialogOpen = false;
					toast.success(m.users_email_updated());
				}
			} catch (error) {
				toast.error(getLocalizedError(error));
			}
		});
	}

	function unscopedRoleIds(user: User): string[] {
		return user.roleGrants
			.filter((grant) => grant.scopeKind === RoleGrantScopeKind.UNSPECIFIED && grant.role)
			.map((grant) => grant.role!.id?.value ?? '');
	}

	async function openIdentityLinksDialog(user: User) {
		identityLinksUser = user;
		identityLinks = [];
		identityLinksLoading = true;
		identityLinksDialogOpen = true;
		try {
			const userDetail = await apiClient.getUser((user.id?.value ?? ''));
			identityLinks = userDetail?.identityLinks ?? [];
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			identityLinksLoading = false;
		}
	}

	async function unlinkIdentity() {
		if (!unlinkingLinkId) return;
		try {
			await apiClient.unlinkIdentity(unlinkingLinkId);
			identityLinks = identityLinks.filter((l) => (l.id?.value ?? '') !== unlinkingLinkId);
			unlinkConfirmOpen = false;
			unlinkingLinkId = '';
			toast.success(m.users_identity_unlinked());
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	// The query lives in the pill now: ⌘K opens search already on this facet and
	// its keystrokes land on the same setSearch the removed input drove. The
	// registration is withdrawn on unmount so the next page never inherits it.
	$effect(() =>
		registerPageSearch({
			scope: SearchScope.USERS,
			label: m.nav_users,
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
				<h1 class="truncate text-2xl font-bold">{m.users_title()}</h1>
				<p class="text-sm text-muted-foreground">{m.users_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">
				<!-- The list's filters ride IN the list's own toolbar, next to sort:
				     narrowing a list is one act, so it reads as one bar. The page band
				     keeps only what acts on the page itself. -->
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

	<!-- The people list in the drafts' row grammar: initials tile, email over its
	     ULID and profile name, membership chips, a right-aligned last-login stamp —
	     no column headers, no table. Only the render layer changed. -->
	<RowList {table} {sortOptions} rowKey={(u) => (u.id?.value ?? '')} href={(u) => `${base}/users/${(u.id?.value ?? '')}`}>
		{#snippet filters()}
			<MultiSelectCombobox
				items={statusFilterItems}
				selected={table.filters.status}
				onSelectedChange={(next) => table.setFilter('status', next)}
				placeholder={m.users_filter_all_statuses()}
				searchPlaceholder={m.common_search()}
				class="w-44"
			/>
		{/snippet}
		{#snippet row(user)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<span class="font-mono text-[0.6rem] font-semibold text-accent-ink">
					{initialsOf(user)}
				</span>
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{user.email}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{user.id}</span>
					<span class="truncate text-xs text-muted-foreground">{displayNameOf(user)}</span>
				</span>
			</span>
			<span class="flex shrink-0 items-center gap-1.5">
				{#if user.id === authStore.user?.id}
					<Chip tone="info" label={m.users_you()} />
				{/if}
				<span title={m.users_table_status()}>
					<Chip
						tone={user.disabled ? 'crit' : 'ok'}
						label={user.disabled ? m.common_disabled() : m.common_active()}
					/>
				</span>
			</span>
			<!-- Granted roles first, then the ones inherited through a group; the
			     header that used to name this cluster now rides its tooltip. Both
			     clusters come off the SEARCH DOCUMENT (role_names/ids and their
			     inherited_ twins) — the adapter already deduped inherited pairs by
			     role id, and a role held directly is not repeated as inherited. -->
			<span class="flex min-w-0 items-center gap-1.5 overflow-hidden" title={m.users_table_role()}>
				{#each user.docRoles.direct as role}
					<Chip tone={role.name === 'Admin' ? 'info' : 'idle'} label={role.name} />
				{/each}
				{#each user.docRoles.inherited as role}
					{#if !user.docRoles.direct.some((direct) => direct.id === role.id)}
						<Chip tone="idle">
							<UsersRound class="h-3 w-3" />
							{role.name}
						</Chip>
					{/if}
				{/each}
			</span>
			<!-- One stamp keeps the row dense; created stays reachable in the tooltip
			     and as a sort key. -->
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
			{#if user.id !== authStore.user?.id}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
								<MoreHorizontal class="h-4 w-4" />
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end">
						<DropdownMenu.Item onclick={() => openEditEmailDialog(user)}>
							<Pencil class="mr-2 h-4 w-4" />
							{m.users_edit_email_title()}
						</DropdownMenu.Item>
						<DropdownMenu.Item onclick={() => openIdentityLinksDialog(user)}>
							<Globe class="mr-2 h-4 w-4" />
							{m.users_manage_identity_links_title()}
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item onclick={() => openRoleDialog(user)}>
							<UserPlus class="mr-2 h-4 w-4" />
							{m.roles_assign_to_user()}
						</DropdownMenu.Item>
						{#each revocableRoles(user) as role (role.id)}
							<DropdownMenu.Item onclick={() => revokeRole(user, role.id)}>
								<Shield class="mr-2 h-4 w-4" />
								{m.roles_revoke_from_user()}: {role.name}
							</DropdownMenu.Item>
						{/each}
						<DropdownMenu.Separator />
						<DropdownMenu.Item onclick={() => toggleUserDisabled(user)}>
							{#if user.disabled}
								<Check class="mr-2 h-4 w-4" />
								{m.common_enable()}
							{:else}
								<Ban class="mr-2 h-4 w-4" />
								{m.common_disable()}
							{/if}
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item onclick={() => confirmErase(user)} class="text-destructive">
							<Trash2 class="mr-2 h-4 w-4" />
							{m.users_erase_action()}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{/if}
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<Users class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.users_empty()}</h3>
				<p class="text-muted-foreground">
					{table.query || table.filters.status.length > 0
						? m.common_try_different_search()
						: m.users_provisioning_hint()}
				</p>
			</div>
		{/snippet}
	</RowList>

	<DataTablePagination {table} />
</PageShell>

{#if roleDialogUser}
	<RoleAssignDialog
		bind:open={roleDialogOpen}
		title={m.roles_assign_to_user()}
		subtitle={roleDialogUser.email}
		excludeRoleIds={unscopedRoleIds(roleDialogUser)}
		assign={(roleIds, scopeKind, scopeId) =>
			apiClient.assignRoleToUser(roleDialogUser!.id?.value ?? '', roleIds, scopeKind, scopeId)}
		onAssigned={() => table.refresh()}
	/>
{/if}

<AlertDialog.Root bind:open={eraseDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.users_erase_dialog_title()}</AlertDialog.Title>
			<AlertDialog.Description>
				{m.users_erase_dialog_description({ email: userToErase?.email ?? '' })}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<p class="text-sm text-muted-foreground">{m.users_erase_dialog_scim_note()}</p>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={eraseUser} variant="destructive">
				{m.users_erase_action()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<Dialog.Root bind:open={editEmailDialogOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.users_edit_email_title()}</Dialog.Title>
			<Dialog.Description
				>{m.users_edit_email_description({ email: editEmailUser?.email ?? '' })}</Dialog.Description
			>
		</Dialog.Header>
		<form
			onsubmit={(e) => {
				e.preventDefault();
				updateEmail();
			}}
			class="space-y-4"
		>
			<div class="space-y-2">
				<Label for="edit-email">{m.users_table_email()}</Label>
				<Input
					id="edit-email"
					type="email"
					placeholder={m.users_email_placeholder()}
					bind:value={editEmail}
					required
					aria-invalid={!!emailFv.errors.email}
				/>
				<FieldError error={emailFv.errors.email} />
			</div>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (editEmailDialogOpen = false)}>
					{m.common_cancel()}
				</Button>
				<Button type="submit">{m.common_save()}</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={identityLinksDialogOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.users_manage_identity_links_title()}</Dialog.Title>
			<Dialog.Description
				>{m.users_manage_identity_links_description({
					email: identityLinksUser?.email ?? ''
				})}</Dialog.Description
			>
		</Dialog.Header>
		<div class="py-4 space-y-3">
			{#if identityLinksLoading}
				<p class="text-sm text-muted-foreground">{m.common_loading()}</p>
			{:else if identityLinks.length === 0}
				<p class="text-sm text-muted-foreground">{m.users_no_identity_links()}</p>
			{:else}
				{#each identityLinks as link}
					<div class="flex items-center justify-between rounded-lg border p-3">
						<div>
							<p class="font-medium">{link.providerName}</p>
							<p class="text-sm text-muted-foreground">{link.externalEmail}</p>
						</div>
						<Button
							variant="ghost"
							size="sm"
							onclick={() => {
								unlinkingLinkId = (link.id?.value ?? '');
								unlinkConfirmOpen = true;
							}}
						>
							<Unlink class="mr-2 h-4 w-4" />
							{m.common_delete()}
						</Button>
					</div>
				{/each}
			{/if}
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (identityLinksDialogOpen = false)}>
				{m.common_done()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<AlertDialog.Root bind:open={unlinkConfirmOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.common_delete()}</AlertDialog.Title>
			<AlertDialog.Description>{m.users_identity_unlink_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel
				onclick={() => {
					unlinkConfirmOpen = false;
					unlinkingLinkId = '';
				}}>{m.common_cancel()}</AlertDialog.Cancel
			>
			<AlertDialog.Action onclick={unlinkIdentity}>{m.common_delete()}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
