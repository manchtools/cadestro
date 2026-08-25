<script lang="ts">
	// User group detail — entity header card + tabs with counts.
	//
	// A SCIM-managed group's lifecycle lives at the identity provider: its
	// identity, membership and rule are all read-only here, and the note says
	// why. Role grants are NOT part of SCIM, so they stay editable.
	//
	// The pill is this group's action bar, held for the whole visit so Delete and
	// the maintenance window have a home that does not depend on having typed
	// something first. `dirty` still tells the truth about the identity buffer.
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
	// ONE context for the whole group — see the device-group twin: a second
	// context on the Rule tab meant two Saves for one entity.
	const groupContextId = $derived(`user-group:${groupId}`);
	const scim = $derived(group?.isScimManaged ?? false);
	// Gated on `editingIdentity`: the pill is held even when the fields are shut,
	// and a group the operator only looked at must never park a draft on the stage.
	const identityDirty = $derived(
		editingIdentity &&
			group !== null &&
			(draftName !== group.name || draftDescription !== group.description)
	);
	const identityNameValid = $derived(draftName.trim().length > 0);

	// The rule's edit buffer and its live validation, reported up by the editor.
	let draftQuery = $state('');
	let ruleState = $state<QueryEditorState>({
		text: '',
		valid: false,
		count: null,
		error: m.query_incomplete(),
		validating: false
	});
	let ruleConfirmOpen = $state(false);
	/** The stored rule as last seen, so a reload cannot clobber an edit. */
	let lastSavedQuery = '';
	const savedQuery = $derived(group?.dynamicQuery ?? '');
	const ruleDirty = $derived(group !== null && draftQuery !== savedQuery);
	const ruleValid = $derived(ruleState.valid === true);

	// Role ids hidden from the assign picker: held GLOBALLY (unscoped). A role held
	// only at group scopes stays selectable so a second scope can be added (#7).
	const roleAssignExcludeIds = $derived(
		(group?.roleGrants ?? [])
			.filter((g) => g.scopeKind === RoleGrantScopeKind.UNSPECIFIED && g.role)
			.map((g) => g.role!.id?.value ?? '')
	);

	const memberUserIds = $derived(new Set(members.map((x) => x.userId)));
	const availableUsers = $derived(allUsers.filter((u) => !memberUserIds.has(u.id)));

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

	// If the page goes away mid-edit the pill must not keep pointing at a group
	// that is no longer on screen — and must not DISCARD the unsaved identity.
	// Auto-stash-on-navigate parks it on the stage instead; a commit/cancel/stash
	// already cleared `owns`, so this only fires on a genuine leave.
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
				// Rebase the rule buffer on the STORED rule whenever the stored rule
				// itself moved (first read, or a save landing) — never on an unrelated
				// reload, which would eat an edit in progress. Imperative and ordered,
				// so the parked-draft restore below still wins.
				if (group.dynamicQuery !== lastSavedQuery) {
					lastSavedQuery = group.dynamicQuery;
					draftQuery = group.dynamicQuery;
				}
				// …then take back a draft this page parked on the stage. The buffer is
				// component state, so it did NOT survive the unmount: the stash
				// snapshotted it onto the card and this is where it comes back.
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

	// ── the pill is this group's action bar ──────────────────────────────────
	/** This page's home — a stashed draft restores by navigating back to it. */
	const contextRoute = $derived(`/user-groups/${groupId}`);

	/** What Stash has to carry. The identity buffer is component state, so an
	 *  unmount destroys it: the card must hold the buffer itself. */
	interface GroupDraft {
		name: string;
		description: string;
		query: string;
	}

	// Plain `let`, not `$state`: the effect writes it, so a tracked read would
	// make the effect depend on its own write.
	let owns = false;
	/** The operator set this context aside on purpose: do not take the bar
	 *  back when the slot frees, or Stash would undo itself. */
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
		// The group's own actions. A SCIM-managed group's lifecycle lives at the
		// identity provider, so it is offered no Delete at all — an action that is
		// invalid for this entity must not appear and then fail.
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
				ruleDirty && group && !group.isDynamic ? m.query_commit_convert() : m.common_save(),
			// Every context explains itself; a greyed Save with no reason is a dead
			// button. The rule's caption is the shared one.
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
				// The store already exited context; a FAILED save re-acquires it below,
				// so the operator never loses the buffer with the commit.
				owns = false;
				if (ruleDirty) ruleConfirmOpen = true;
				else void saveGroup();
			},
			onCancel: () => {
				owns = false;
				revertIdentityEdit();
				draftQuery = savedQuery;
			},
			// Stash releases the pill deliberately. The effect wakes when the slot
			// frees, so without a remembered intent it would re-acquire instantly
			// and the stash would never take.
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
		// Resuming edits supersedes any card this context parked on the stage: the
		// live buffer is newer than the snapshot on the card.
		removeDraft(draftIdFor(groupContextId));
		enterContext(contextState());
	}

	function release() {
		owns = false;
		// Only tear down our own context — another surface may have taken over.
		if (shell.pill.context?.id === groupContextId) exitContext();
	}

	$effect(() => {
		// Read every reactive input HERE. `savingIdentity` parks the pill for the
		// round trip: an in-flight commit has no second commit.
		const active = group !== null && !savingIdentity;
		// Tracked, but deliberately NOT a gate. The Rule tab publishes its own
		// context and there is one pill, so whoever holds it keeps it — this only
		// wakes the effect on a tab switch, so the bar comes back when the rule
		// editor lets go. Gating on the tab instead is what made the pill reset to
		// nav the moment the operator opened the query, and patching our state onto
		// the rule editor's context is what greyed out its Save.
		void activeTab;
		// Tracked: this is what wakes the effect when whoever else held the bar
		// lets go. The rule editor exits its context the moment the query matches
		// the stored one again, and nothing else here would change — so the pill
		// stayed empty until the next keystroke.
		const holder = shell.pill.context?.id ?? null;
		// The WHOLE state, not a three-field subset. Once the rule joined this
		// context, a patch that carried only the identity fields left the pill
		// blind to it: Save stayed disabled over a valid rule edit, and the rule's
		// caption never reached the bar. Building the full state here also keeps
		// every reactive input tracked in one place.
		const patch = contextState();
		// …and write to the store UNTRACKED. The store helpers read
		// `shell.pill.context` themselves, so a tracked call would make this effect
		// depend on the pill it just wrote — and Stash, which clears the context,
		// would be undone by an instant re-acquire.
		untrack(() => {
			// Held is read from the STORE: another surface on this page (the Rule
			// tab's editor) may have taken the single context slot, and a stale
			// local flag made us patch OUR state onto ITS context — which set
			// dirty:false on a dirty query and greyed out its Save.
			const held = holder === groupContextId;
			if (!active) {
				if (held) release();
				return;
			}
			if (held) updateContext(patch);
			// Never stomp a context somebody else is holding — the Rule tab's editor
			// takes this slot the moment its query is dirty, and patching our state
			// onto it set dirty:false on a dirty query and greyed out its Save.
			else if (holder === null && !stashParked) acquire();
		});
	});

	/** The group's ONE commit: identity and rule in a single save. Two contexts
	 *  meant two Saves for one entity — see the device-group twin. */
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
				const ruled = await apiClient.updateUserGroupQuery((group.id?.value ?? ''), true, query);
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

	// ── mutations ────────────────────────────────────────────────────────────
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
			// F022: page through all users instead of capping at 200.
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

	/** Reached only through the future-scope confirm in the rule editor. */
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
			<!-- The ENTITY, not the section: the page title and the pill must agree. -->
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-2xl font-bold">{group?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{groupId}</p>
			</div>
			<div class="ml-auto flex gap-2">
				{#if group?.isDynamic && !scim}
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
					<Tile tone={group.isDynamic ? 'info' : 'idle'} label={group.name} />
				</span>
				<div class="min-w-0 flex-1">
					{#if editingIdentity}
						<div class="space-y-1.5" data-testid="identity-edit">
							<!-- Standing note first, same as the device-group twin: where the
							     commit lives is no use after the fields are already typed. -->
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
						<p class="font-mono text-xs text-faint">{group.id}</p>
						<p class="mt-1 text-sm text-muted-foreground">
							{group.description || m.common_no_description()}
						</p>
					{/if}
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<Chip
						tone={group.isDynamic ? 'info' : 'idle'}
						label={group.isDynamic ? m.user_groups_dynamic_label() : m.user_group_detail_static()}
					/>
					{#if scim}
						<Chip tone="warn" label={m.user_groups_scim_managed()} />
					{/if}
					<Stat
						tone={group.isDynamic ? 'info' : 'ok'}
						value={group.memberCount}
						label={m.user_group_detail_members()}
					/>
					<!-- No trash glyph here: Delete acts on the whole group, so it is a
					     pill action — and a SCIM-managed group, whose lifecycle lives at
					     the identity provider, is offered none at all. -->
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
				<!-- No Schedules tab: the maintenance window was its only row, and it
				     is a group-wide policy, so it moved to the pill with Delete. An
				     empty tab holding one edit button was not worth a tab. -->
			</Tabs.List>

			<Tabs.Content value="members" class="mt-3">
				<MembersTab
					{members}
					isDynamic={group.isDynamic}
					isScimManaged={scim}
					onadd={openAddMemberDialog}
					onremove={removeMember}
				/>
			</Tabs.Content>

			{#if !scim}
				<Tabs.Content value="rule" class="mt-3">
					<DynamicRuleEditor
						kind="user"
						savedQuery={group.dynamicQuery}
						bind:draft={draftQuery}
						isDynamic={group.isDynamic}
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
						{#each group.roleGrants ?? [] as grant (grant.role?.id + ':' + grant.scopeKind + ':' + grant.scopeId)}
							{#if grant.role}
								<div class="flex flex-wrap items-center gap-2 border-b border-hair px-3 py-2 last:border-b-0">
									<Shield class="h-4 w-4 shrink-0 text-muted-foreground" />
									<span class="font-mono text-sm">{grant.role.name}</span>
									{#if grant.scopeKind === RoleGrantScopeKind.DEVICE_GROUP}
										<Chip tone="idle">
											<Terminal class="h-3 w-3" />{grant.scopeName || grant.scopeId}
										</Chip>
									{:else if grant.scopeKind === RoleGrantScopeKind.USER_GROUP}
										<Chip tone="idle">
											<UsersRound class="h-3 w-3" />{grant.scopeName || grant.scopeId}
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

<!-- Gates the group's ONE commit: a standing rule decides membership from here
     on, and a banner you can scroll past is not an acknowledgement. -->
<FutureScopeDialog
	bind:open={ruleConfirmOpen}
	queryText={ruleState.text}
	count={ruleState.count}
	kind="user"
	converting={!(group?.isDynamic ?? false)}
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
