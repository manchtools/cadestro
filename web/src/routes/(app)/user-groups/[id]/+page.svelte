<script lang="ts">

	import { onMount, onDestroy, untrack } from 'svelte';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		fetchAllPages,
		type UserGroup,
		type UserGroupMember,
		type User,
		type RoleGrant,
		RoleGrantScopeKind
	} from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Tile, Chip, Stat } from '$lib/components/fleet';
	import ItemTablePicker from '$lib/components/item-table-picker.svelte';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import RoleAssignDialog from '$lib/components/role-assign-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import MaintenanceWindowDialog, {
		entriesFromWindow,
		type MaintenanceWindowEntryInput
	} from '$lib/components/maintenance-window-dialog.svelte';
	import DynamicRuleEditor, {
		type RulePreviewRow,
		ruleSubtext
	} from '$lib/components/dynamic-rule-editor.svelte';
	import FutureScopeDialog from '$lib/components/future-scope-dialog.svelte';
	import type { QueryEditorState } from '$lib/components/query-builder.svelte';
	import MembersTab from './members-tab.svelte';
	import { create } from '@bufbuild/protobuf';
	import {
		MaintenanceWindowSchema,
		MaintenanceWindowEntrySchema
	} from '$contract/cadestro/v1/common_pb';
	import {
		shell,
		enterContext,
		updateContext,
		exitContext,
		leaveContext,
		removeDraft,
		claimDraft,
		draftIdFor,
		type ContextState,
		type PillAction
	} from '$lib/shell/shell.svelte';
	import {
		ArrowLeft,
		RefreshCw,
		Trash2,
		Pencil,
		Play,
		Plus,
		Shield,
		Terminal,
		UsersRound
	} from '@lucide/svelte';

	let group = $state<UserGroup | null>(null);
	let members = $state<UserGroupMember[]>([]);
	let loading = $state(true);
	let evaluating = $state(false);
	let activeTab = $state('members');

	let deleteDialogOpen = $state(false);
	let addMemberDialogOpen = $state(false);
	let assignRoleDialogOpen = $state(false);
	let maintenanceWindowDialogOpen = $state(false);
	let allUsers = $state<User[]>([]);
	let selectedUserIds = $state<string[]>([]);

	let editingIdentity = $state(false);
	let savingIdentity = $state(false);
	let draftName = $state('');
	let draftDescription = $state('');

	const groupId = $derived(page.params.id ?? '');

	const groupContextId = $derived(`user-group:${groupId}`);
	const scim = $derived(group?.isScimManaged ?? false);

	const identityDirty = $derived(
		editingIdentity &&
			group !== null &&
			(draftName !== group.name || draftDescription !== group.description)
	);
	const identityNameValid = $derived(draftName.trim().length > 0);

	let draftQuery = $state('');
	let ruleState = $state<QueryEditorState>({
		text: '',
		valid: false,
		count: null,
		error: m.query_incomplete(),
		validating: false
	});
	let ruleConfirmOpen = $state(false);

	let lastSavedQuery = '';
	const savedQuery = $derived(group?.dynamicQuery ?? '');
	const ruleDirty = $derived(group !== null && draftQuery !== savedQuery);
	const ruleValid = $derived(ruleState.valid === true);

	const roleAssignExcludeIds = $derived(
		(group?.roleGrants ?? [])
			.filter((g) => g.scopeKind === RoleGrantScopeKind.UNSPECIFIED && g.role)
			.map((g) => g.role!.id?.value ?? '')
	);

	const memberUserIds = $derived(new Set(members.map((x) => x.userId?.value ?? '')));
	const availableUsers = $derived(allUsers.filter((u) => !memberUserIds.has(u.id?.value ?? '')));

	const previewRows = $derived<RulePreviewRow[]>(
		members.map((member) => ({
			id: member.userId?.value ?? '',
			primary: member.email,
			attributes: [],
			tone: 'info'
		}))
	);

	onMount(() => {
		if (groupId) loadData();
	});

	onDestroy(() => {
		if (owns) {
			owns = false;
			leaveContext(groupContextId);
		}
	});

	async function loadData() {
		loading = true;
		try {
			const response = await apiClient.getUserGroup(groupId);
			group = response.group ?? null;
			members = response.members;
			if (group) {
				if (!editingIdentity) {
					draftName = group.name;
					draftDescription = group.description;
				}

				if (group.dynamicQuery !== lastSavedQuery) {
					lastSavedQuery = group.dynamicQuery ?? '';
					draftQuery = group.dynamicQuery ?? '';
				}

				const parked = claimDraft(groupContextId) as GroupDraft | undefined;
				if (parked) {
					draftName = parked.name;
					draftDescription = parked.description;
					if (parked.query !== undefined) draftQuery = parked.query;
					editingIdentity = true;
				}
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	const contextRoute = $derived(`/user-groups/${groupId}`);

	interface GroupDraft {
		name: string;
		description: string;
		query: string;
	}

	let owns = false;

	let stashParked = false;

	function startIdentityEdit() {
		draftName = group?.name ?? '';
		draftDescription = group?.description ?? '';
		editingIdentity = true;
	}

	function revertIdentityEdit() {
		editingIdentity = false;
		draftName = group?.name ?? '';
		draftDescription = group?.description ?? '';
	}

	function contextState(): ContextState {

		const entityActions: PillAction[] = [
			{
				id: 'window',
				label: m.user_group_detail_window_label(),
				onRun: () => (maintenanceWindowDialogOpen = true)
			}
		];
		if (!scim) {
			entityActions.push({
				id: 'delete',
				label: m.user_groups_delete(),
				tone: 'danger' as const,
				onRun: () => (deleteDialogOpen = true)
			});
		}
		return {
			id: groupContextId,
			route: contextRoute,
			title: draftName || (group?.name ?? ''),
			dirty: identityDirty || ruleDirty,
			valid: identityNameValid && ruleValid,
			commitLabel:
				ruleDirty && group && group.dynamicQuery === undefined ? m.query_commit_convert() : m.common_save(),

			subtext: !identityNameValid
				? m.validation_name_required()
				: ruleDirty || !ruleValid
					? ruleSubtext(ruleState, 'user').text
					: undefined,
			subtextTone: (!identityNameValid
				? 'warn'
				: ruleDirty || !ruleValid
					? ruleSubtext(ruleState, 'user').tone
					: 'neutral') as 'neutral' | 'warn',
			onCommit: () => {

				owns = false;
				if (ruleDirty) ruleConfirmOpen = true;
				else void saveGroup();
			},
			onCancel: () => {
				owns = false;
				revertIdentityEdit();
				draftQuery = savedQuery;
			},

			onStash: () => {
				stashParked = true;
				owns = false;
			},
			onRestore: () => {
				stashParked = false;
				owns = true;
			},
			stashPayload: (): GroupDraft => ({
				name: draftName,
				description: draftDescription,
				query: draftQuery
			}),
			stashSubtitle: m.user_groups_edit_identity(),
			extraActions: entityActions
		};
	}

	function acquire() {
		owns = true;
		stashParked = false;

		removeDraft(draftIdFor(groupContextId));
		enterContext(contextState());
	}

	function release() {
		owns = false;

		if (shell.pill.context?.id === groupContextId) exitContext();
	}

	$effect(() => {

		const active = group !== null && !savingIdentity;

		void activeTab;

		const holder = shell.pill.context?.id ?? null;

		const patch = contextState();

		untrack(() => {

			const held = holder === groupContextId;
			if (!active) {
				if (held) release();
				return;
			}
			if (held) updateContext(patch);

			else if (holder === null && !stashParked) acquire();
		});
	});

	async function saveGroup() {
		if (!group) return;
		const query = draftQuery;
		const wantsRule = ruleDirty;
		savingIdentity = true;
		try {
			const updated = await apiClient.updateUserGroup(
				(group.id?.value ?? ''),
				draftName.trim(),
				draftDescription.trim()
			);
			if (updated) group = updated;
			if (wantsRule) {
				const ruled = await apiClient.updateUserGroupQuery((group.id?.value ?? ''), query);
				if (ruled) group = ruled;
				toast.success(m.user_group_detail_query_updated());
			}
			editingIdentity = false;
			toast.success(m.user_group_detail_updated());
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			savingIdentity = false;
		}
	}

	async function deleteGroup() {
		if (!group) return;
		try {
			await apiClient.deleteUserGroup((group.id?.value ?? ''));
			toast.success(m.user_groups_deleted());
			goto('/user-groups');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function openAddMemberDialog() {
		selectedUserIds = [];
		try {

			allUsers = await fetchAllPages<User>(async (size, token) => {
				const r = await apiClient.listUsers(size, token);
				return { items: r.users, nextPageToken: r.nextPageToken };
			});
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
			return;
		}
		addMemberDialogOpen = true;
	}

	async function addMembers() {
		if (!group || selectedUserIds.length === 0) return;
		try {
			await apiClient.addUserToGroup((group.id?.value ?? ''), selectedUserIds);
			toast.success(m.user_group_detail_member_added());
			addMemberDialogOpen = false;
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function removeMember(userId: string) {
		if (!group) return;
		try {
			await apiClient.removeUserFromGroup((group.id?.value ?? ''), userId);
			toast.success(m.user_group_detail_member_removed());
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function revokeRoleGrant(grant: RoleGrant) {
		if (!group || !grant.role) return;
		try {
			await apiClient.revokeRoleFromUserGroup((group.id?.value ?? ''), (grant.role.id?.value ?? ''), grant.scopeKind, grant.scopeId?.value);
			toast.success(m.user_group_detail_role_revoked());
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function evaluateGroup() {
		if (!group) return;
		evaluating = true;
		try {
			const result = await apiClient.evaluateDynamicUserGroup((group.id?.value ?? ''));
			if (result.group) group = result.group;
			toast.success(
				m.user_group_detail_evaluated({ added: result.usersAdded, removed: result.usersRemoved })
			);
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			evaluating = false;
		}
	}

	async function updateMaintenanceWindow(entries: MaintenanceWindowEntryInput[]) {
		if (!group) return;
		try {
			const window =
				entries.length === 0
					? undefined
					: create(MaintenanceWindowSchema, {
							schedule: entries.map((e) =>
								create(MaintenanceWindowEntrySchema, { days: e.days, allow: e.allow })
							)
						});
			const updated = await apiClient.setUserGroupMaintenanceWindow((group.id?.value ?? ''), window);
			if (updated) group = updated;
			toast.success(m.user_group_detail_window_updated());
			maintenanceWindowDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}
</script>

<PageShell contentClass="space-y-4">
	{#snippet header()}
		<div class="flex items-center gap-2">
			<Button variant="ghost" size="icon" aria-label={m.common_back()} onclick={() => history.back()}>
				<ArrowLeft class="h-4 w-4" />
			</Button>

			<div class="min-w-0 flex-1">
				<h1 class="truncate text-2xl font-bold">{group?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{groupId}</p>
			</div>
			<div class="ml-auto flex gap-2">
				{#if group?.dynamicQuery !== undefined && !scim}
					<Button variant="outline" size="sm" onclick={evaluateGroup} disabled={evaluating}>
						<span class="mr-2 h-4 w-4" class:animate-spin={evaluating}><Play class="h-4 w-4" /></span>
						{evaluating ? m.user_group_detail_evaluating() : m.user_group_detail_evaluate()}
					</Button>
				{/if}
				<Button variant="outline" size="sm" onclick={loadData} disabled={loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={loading}><RefreshCw class="h-4 w-4" /></span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	{#if loading && !group}
		<div class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate">
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if group}
		<div
			class="rounded-xl border border-hair bg-surface p-4 shadow-plate"
			data-tour="group-header"
			data-testid="group-header"
		>
			<div class="flex flex-wrap items-start gap-3">
				<span class="mt-1 w-4 shrink-0">
					<Tile tone={group.dynamicQuery !== undefined ? 'info' : 'idle'} label={group.name} />
				</span>
				<div class="min-w-0 flex-1">
					{#if editingIdentity}
						<div class="space-y-1.5" data-testid="identity-edit">

							<p class="text-xs text-muted-foreground">{m.user_groups_identity_pill_hint()}</p>
							<Input bind:value={draftName} aria-label={m.common_name()} class="h-8 font-mono text-sm" />
							<Textarea
								bind:value={draftDescription}
								aria-label={m.common_description()}
								rows={2}
								placeholder={m.user_groups_description_placeholder()}
								class="text-sm"
							/>
						</div>
					{:else}
						<div class="flex items-center gap-2">
							<h2 class="truncate font-mono text-lg font-semibold">{group.name}</h2>
							{#if !scim}
								<Button
									variant="ghost"
									size="icon-sm"
									aria-label={m.user_groups_edit_identity()}
									onclick={startIdentityEdit}
								>
									<Pencil class="h-3.5 w-3.5" />
								</Button>
							{/if}
						</div>
						<p class="font-mono text-xs text-faint">{group.id?.value ?? ''}</p>
						<p class="mt-1 text-sm text-muted-foreground">
							{group.description || m.common_no_description()}
						</p>
					{/if}
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<Chip
						tone={group.dynamicQuery !== undefined ? 'info' : 'idle'}
						label={group.dynamicQuery !== undefined ? m.user_groups_dynamic_label() : m.user_group_detail_static()}
					/>
					{#if scim}
						<Chip tone="warn" label={m.user_groups_scim_managed()} />
					{/if}
					<Stat
						tone={group.dynamicQuery !== undefined ? 'info' : 'ok'}
						value={group.memberCount}
						label={m.user_group_detail_members()}
					/>

				</div>
			</div>
		</div>

		<Tabs.Root value={activeTab} onValueChange={(v) => (activeTab = v)}>
			<Tabs.List>
				<Tabs.Trigger value="members">
					{m.user_groups_members_tab({ count: group.memberCount })}
				</Tabs.Trigger>
				{#if !scim}
					<Tabs.Trigger value="rule">{m.query_tab_rule()}</Tabs.Trigger>
				{/if}
				<Tabs.Trigger value="roles">
					{m.user_groups_roles_tab({ count: group.roleGrants.length })}
				</Tabs.Trigger>

			</Tabs.List>

			<Tabs.Content value="members" class="mt-3">
				<MembersTab
					{members}
					isDynamic={group.dynamicQuery !== undefined}
					isScimManaged={scim}
					onadd={openAddMemberDialog}
					onremove={removeMember}
				/>
			</Tabs.Content>

			{#if !scim}
				<Tabs.Content value="rule" class="mt-3">
					<DynamicRuleEditor
						kind="user"
						savedQuery={group.dynamicQuery ?? ''}
						bind:draft={draftQuery}
						isDynamic={group.dynamicQuery !== undefined}
						rows={previewRows}
						total={group.memberCount}
						onstate={(state) => (ruleState = state)}
					/>
				</Tabs.Content>
			{/if}

			<Tabs.Content value="roles" class="mt-3">
				<div class="rounded-xl border border-hair bg-surface shadow-plate" data-testid="roles-tab">
					<div class="flex items-center justify-between border-b px-3 py-2">
						<span class="font-mono text-[0.62rem] tracking-[0.08em] text-faint uppercase">
							{m.user_group_detail_roles()}
						</span>
						<Button size="sm" variant="outline" onclick={() => (assignRoleDialogOpen = true)}>
							<Plus class="mr-1 h-3.5 w-3.5" />
							{m.user_group_detail_assign_role()}
						</Button>
					</div>
					{#if (group.roleGrants?.length ?? 0) === 0}
						<p class="px-3 py-8 text-center text-sm text-muted-foreground">
							{m.user_group_detail_no_roles()}
						</p>
					{:else}
						{#each group.roleGrants ?? [] as grant ((grant.role?.id?.value ?? '') + ':' + grant.scopeKind + ':' + (grant.scopeId?.value ?? ''))}
							{#if grant.role}
								<div class="flex flex-wrap items-center gap-2 border-b border-hair px-3 py-2 last:border-b-0">
									<Shield class="h-4 w-4 shrink-0 text-muted-foreground" />
									<span class="font-mono text-sm">{grant.role.name}</span>
									{#if grant.scopeKind === RoleGrantScopeKind.DEVICE_GROUP}
										<Chip tone="idle">
							<Terminal class="h-3 w-3" />{grant.scopeName || (grant.scopeId?.value ?? '')}
										</Chip>
									{:else if grant.scopeKind === RoleGrantScopeKind.USER_GROUP}
										<Chip tone="idle">
							<UsersRound class="h-3 w-3" />{grant.scopeName || (grant.scopeId?.value ?? '')}
										</Chip>
									{/if}
									{#if grant.role.isSystem}
										<Chip tone="idle" label={m.roles_system_badge()} />
									{/if}
									<Chip tone="info" label={String(grant.role.permissions.length)} />
									<Button
										variant="ghost"
										size="icon-sm"
										class="ml-auto shrink-0 text-muted-foreground hover:text-destructive"
										aria-label={m.user_groups_revoke_role()}
										onclick={() => revokeRoleGrant(grant)}
									>
										<Trash2 class="h-3.5 w-3.5" />
									</Button>
								</div>
							{/if}
						{/each}
					{/if}
				</div>
			</Tabs.Content>

		</Tabs.Root>
	{/if}
</PageShell>

<Dialog.Root bind:open={addMemberDialogOpen}>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{m.user_group_detail_add_member()}</Dialog.Title>
			<Dialog.Description>{m.user_group_detail_add_member_description()}</Dialog.Description>
		</Dialog.Header>

		<ItemTablePicker
			items={availableUsers.map((u) => ({ id: (u.id?.value ?? ''), email: u.email }))}
			bind:selected={selectedUserIds}
			searchPlaceholder={m.picker_search_users()}
			emptyMessage={m.picker_no_users()}
			searchFilter={(item, query) => item.email.toLowerCase().includes(query.toLowerCase())}
		>
			{#snippet headerRow()}
				<Table.Head>{m.users_table_email()}</Table.Head>
			{/snippet}
			{#snippet itemRow(item)}
				<Table.Cell><span class="font-mono text-sm">{item.email}</span></Table.Cell>
			{/snippet}
		</ItemTablePicker>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (addMemberDialogOpen = false)}>{m.common_cancel()}</Button>
			<Button onclick={addMembers} disabled={selectedUserIds.length === 0}>{m.common_add()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

{#if group}
	<RoleAssignDialog
		bind:open={assignRoleDialogOpen}
		title={m.user_group_detail_assign_role()}
		subtitle={group.name}
		excludeRoleIds={roleAssignExcludeIds}
		assign={(roleIds, scopeKind, scopeId) =>
			apiClient.assignRoleToUserGroup(group!.id?.value ?? '', roleIds, scopeKind, scopeId)}
		onAssigned={loadData}
	/>
{/if}

<FutureScopeDialog
	bind:open={ruleConfirmOpen}
	queryText={ruleState.text}
	count={ruleState.count}
	kind="user"
	converting={group?.dynamicQuery === undefined}
	currentMembers={group?.memberCount ?? 0}
	onconfirm={() => {
		ruleConfirmOpen = false;
		void saveGroup();
	}}
	oncancel={() => (ruleConfirmOpen = false)}
/>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.user_groups_delete()}
	description={m.user_groups_delete_confirm()}
	onconfirm={deleteGroup}
/>

<MaintenanceWindowDialog
	bind:open={maintenanceWindowDialogOpen}
	entries={entriesFromWindow(group?.maintenanceWindow)}
	title={m.user_group_detail_window_dialog_title()}
	description={m.user_group_detail_window_dialog_description()}
	onsave={updateMaintenanceWindow}
/>
