<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
 import { readRole, saveRoleEdits } from '$lib/role-editor';
	import { Permission, type Role } from '$contract/cadestro/v1/control_pb';
	import { consoleContext } from '$lib/console-context.svelte';
	const { can } = consoleContext();
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Chip } from '$lib/components/fleet';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import PermissionMatrix from './permission-matrix.svelte';
 import { availablePermissions } from './permission-groups';
	import { ArrowLeft, RefreshCw, Trash2, Server, Users } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import {
		shell,
		enterContext,
		updateContext,
		exitContext,
		leaveContext,
		removeDraft,
		claimDraft,
		draftIdFor,
		type ContextState
	} from '$lib/shell/shell.svelte';

	let role = $state<Role | null>(null);
	let allPermissions = $state<Permission[]>([]);
	let loading = $state(true);
	let saving = $state(false);
 let editBaselineReady = $state(true);
	let deleteDialogOpen = $state(false);

	let editName = $state('');
	let editDescription = $state('');
	let selectedPermissions = $state<Set<Permission>>(new Set());

	const roleId = $derived(page.params.id ?? '');

	function setsEqual(a: Set<Permission>, b: Set<Permission>): boolean {
		if (a.size !== b.size) return false;
		for (const item of a) {
			if (!b.has(item)) return false;
		}
		return true;
	}

	const hasChanges = $derived(
		role !== null &&
			(editName !== role.name ||
				editDescription !== role.description ||
				!setsEqual(selectedPermissions, new Set(role.permissions)))
	);

	const nameValid = $derived(editName.trim().length > 0);

	onMount(() => {
		if (roleId && (can(Permission.GET_ROLE) || can(Permission.LIST_ROLES))) loadData(); else loading = false;
	});

	async function loadData() {
        const preserveEdits = hasChanges;
		loading = true;
		try {
			const [roleResp, permsResp] = await Promise.all([
				readRole(roleId, can).then(role => ({ role })),
				can(Permission.LIST_PERMISSIONS) ? api.listPermissions({}) : Promise.resolve({ permissions: availablePermissions })
			]);
			role = roleResp.role ?? null;
 editBaselineReady = true;
			allPermissions = permsResp.permissions;

			if (role && !preserveEdits) {
				editName = role.name;
				editDescription = role.description;
				selectedPermissions = new Set(role.permissions);

				const parked = claimDraft(contextId) as RoleDraft | undefined;
				if (parked) {
					editName = parked.name;
					editDescription = parked.description;
					selectedPermissions = new Set(parked.permissions);
				}
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

 async function saveRole() {
  if (!role || !hasChanges || !editBaselineReady || !can(Permission.UPDATE_ROLE)) return;
  saving = true;
  const result = await saveRoleEdits(role, editName.trim(), editDescription.trim(), [...selectedPermissions], can);
  if (result.role) role = result.role;
  editBaselineReady = result.role !== null;
  if (result.error) toast.error(`${getLocalizedError(result.error)}${editBaselineReady ? '' : ' Refresh the latest role before saving again.'}`);
  else { revertEdits(); toast.success(m.roles_updated()); }
  saving = false;
 }

	function revertEdits() {
		if (!role) return;
		editName = role.name;
		editDescription = role.description;
		selectedPermissions = new Set(role.permissions);
	}

	async function deleteRole() {
		if (!role) return;
		try {
			await api.deleteRole({ id: role.id });
			toast.success(m.roles_deleted());
			goto('/roles');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	function togglePermission(key: Permission) {
		const next = new Set(selectedPermissions);
		if (next.has(key)) next.delete(key);
		else next.add(key);
		selectedPermissions = next;
	}

	function toggleGroup(keys: Permission[], allSelected: boolean) {
		const next = new Set(selectedPermissions);
		for (const key of keys) {
			if (allSelected) next.delete(key);
			else next.add(key);
		}
		selectedPermissions = next;
	}

	function selectAll() {
		selectedPermissions = new Set(allPermissions);
	}

	function deselectAll() {
		selectedPermissions = new Set();
	}

	const contextId = $derived(`role:${roleId}`);

	const contextRoute = $derived(`/roles/${roleId}`);

	interface RoleDraft {
		name: string;
		description: string;
		permissions: Permission[];
	}

	const pillSubtext = $derived(
		nameValid
			? m.roles_permissions_description({
					count: selectedPermissions.size,
					total: allPermissions.length
				})
			: m.roles_editor_name_required()
	);

	let owns = false;

	function contextState(): ContextState {
		return {
			id: contextId,
			route: contextRoute,
			title: editName || (role?.name ?? ''),

			dirty: hasChanges,
			valid: editBaselineReady && nameValid && can(Permission.UPDATE_ROLE),
			commitLabel: m.common_save(),
			subtext: pillSubtext,
			subtextTone: nameValid ? 'neutral' : 'warn',
			onCommit: () => {

				owns = false;
				void saveRole();
			},
			onCancel: () => {
				owns = false;
				revertEdits();
			},

			onStash: () => {
				owns = false;
			},
			onRestore: () => {
				owns = true;
			},

			stashPayload: (): RoleDraft => ({
				name: editName,
				description: editDescription,
				permissions: [...selectedPermissions]
			}),
			stashSubtitle: m.roles_matrix_title(),

			extraActions: role && can(Permission.DELETE_ROLE)
				? [
						{
							id: 'delete',
							label: m.roles_delete(),
							tone: 'danger' as const,
							onRun: () => (deleteDialogOpen = true)
						}
				  ]
				: []
		};
	}

	function acquire() {
		owns = true;

		removeDraft(draftIdFor(contextId));
		enterContext(contextState());
	}

	function release() {
		owns = false;

		if (shell.pill.context?.id === contextId) exitContext();
	}

	$effect(() => {

		const active = role !== null && !saving;
		const patch = {
			title: editName || (role?.name ?? ''),
			valid: editBaselineReady && nameValid && can(Permission.UPDATE_ROLE),

			dirty: hasChanges,
			subtext: pillSubtext,
			subtextTone: (nameValid ? 'neutral' : 'warn') as 'neutral' | 'warn'
		};

		untrack(() => {
			if (!active) {
				if (owns) release();
				return;
			}
			if (owns) updateContext(patch);
			else acquire();
		});
	});

	onDestroy(() => {
		if (owns) {
			owns = false;
			leaveContext(contextId);
		}
	});
</script>

<PageShell contentClass="space-y-4">
	{#snippet header()}
		<div class="flex items-start gap-3">
			<Button
				variant="ghost"
				size="icon"
				aria-label={m.common_back()}
				onclick={() => history.back()}
			>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-2">
					<h1 class="truncate text-2xl font-bold">{role?.name ?? m.common_loading()}</h1>


				</div>
				<p class="mt-0.5 font-mono text-xs text-faint">{roleId}</p>
			</div>
			<Button onclick={loadData} variant="outline" size="sm" disabled={loading || saving}>
				<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
					<RefreshCw class="h-4 w-4" />
				</span>
				{m.common_refresh()}
			</Button>
		</div>
	{/snippet}

	<div class="space-y-4">
		{#if loading && !role}
			<div class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate">
				<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if role}

			<section class="max-w-3xl space-y-3 rounded-xl border border-hair bg-surface p-4 shadow-plate">



				<div class="grid gap-3 sm:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="editName">{m.roles_name()}</Label>
						<Input
							id="editName"
							bind:value={editName}
							disabled={!can(Permission.UPDATE_ROLE)}
							aria-invalid={!nameValid}
							placeholder={m.roles_name_placeholder()}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="editDesc">{m.roles_description_field()}</Label>
						<Input
							disabled={!can(Permission.UPDATE_ROLE)} id="editDesc"
							bind:value={editDescription}
							placeholder={m.roles_description_placeholder()}
						/>
					</div>
				</div>
			</section>

			<div class="flex flex-wrap items-center gap-2">
				<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.roles_permissions()}
				</span>
				<span class="text-xs text-muted-foreground">
					{m.roles_permissions_description({
						count: selectedPermissions.size,
						total: allPermissions.length
					})}
				</span>
				<span class="ml-auto flex gap-2">
					<Button variant="outline" size="sm" onclick={selectAll} disabled={!allPermissions.length || !can(Permission.UPDATE_ROLE)}>
						{m.roles_select_all()}
					</Button>
					<Button
						variant="outline"
						size="sm"
						onclick={deselectAll}
						disabled={selectedPermissions.size === 0 || !can(Permission.UPDATE_ROLE)}
					>
						{m.roles_deselect_all()}
					</Button>
				</span>
			</div>

			<PermissionMatrix disabled={!can(Permission.UPDATE_ROLE)}
				permissions={allPermissions}
				selected={selectedPermissions}
				columnLabel={role.name}
				onToggle={togglePermission}
				onToggleGroup={toggleGroup}
			/>

		{/if}
	</div>
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.roles_delete()}
	description={m.roles_delete_confirm()}
	onconfirm={deleteRole}
/>
