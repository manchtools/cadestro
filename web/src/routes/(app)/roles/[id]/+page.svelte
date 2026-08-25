<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type Role, type PermissionInfo, PermissionTargetKind } from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Chip } from '$lib/components/fleet';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import PermissionMatrix from './permission-matrix.svelte';
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
	let allPermissions = $state<PermissionInfo[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let deleteDialogOpen = $state(false);

	let editName = $state('');
	let editDescription = $state('');
	let selectedPermissions = $state<Set<string>>(new Set());

	const roleId = $derived(page.params.id ?? '');

	const roleScopability = $derived.by<'device' | 'user' | 'mixed' | 'none'>(() => {
		if (selectedPermissions.size === 0) return 'none';
		let kind: PermissionTargetKind | null = null;
		for (const key of selectedPermissions) {
			const tk =
				allPermissions.find((p) => p.key === key)?.targetKind ?? PermissionTargetKind.UNSPECIFIED;
			if (tk === PermissionTargetKind.UNSPECIFIED) return 'mixed';
			if (kind === null) kind = tk;
			else if (kind !== tk) return 'mixed';
		}
		return kind === PermissionTargetKind.DEVICE ? 'device' : 'user';
	});

	function setsEqual(a: Set<string>, b: Set<string>): boolean {
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
		if (roleId) loadData();
	});

	async function loadData() {
		loading = true;
		try {
			const [roleResp, permsResp] = await Promise.all([
				apiClient.getRole(roleId),
				apiClient.listPermissions()
			]);
			role = roleResp.role ?? null;
			allPermissions = permsResp.permissions;

			if (role) {
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
		if (!role || !hasChanges) return;
		saving = true;
		try {
			const updated = await apiClient.updateRole(
				(role.id?.value ?? ''),
				editName.trim(),
				editDescription.trim(),
				Array.from(selectedPermissions)
			);
			if (updated) {
				role = updated;
				editName = updated.name;
				editDescription = updated.description;
				selectedPermissions = new Set(updated.permissions);
			}
			toast.success(m.roles_updated());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
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
			await apiClient.deleteRole((role.id?.value ?? ''));
			toast.success(m.roles_deleted());
			goto('/roles');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	function togglePermission(key: string) {
		const next = new Set(selectedPermissions);
		if (next.has(key)) next.delete(key);
		else next.add(key);
		selectedPermissions = next;
	}

	function toggleGroup(keys: string[], allSelected: boolean) {
		const next = new Set(selectedPermissions);
		for (const key of keys) {
			if (allSelected) next.delete(key);
			else next.add(key);
		}
		selectedPermissions = next;
	}

	function selectAll() {
		selectedPermissions = new Set(allPermissions.map((p) => p.key));
	}

	function deselectAll() {
		selectedPermissions = new Set();
	}

	const contextId = $derived(`role:${roleId}`);

	const contextRoute = $derived(`/roles/${roleId}`);

	interface RoleDraft {
		name: string;
		description: string;
		permissions: string[];
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
			valid: nameValid,
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

			extraActions: role && !role.isSystem
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
			valid: nameValid,

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
					{#if role?.isSystem}
						<Chip tone="info" label={m.roles_system_badge()} />
					{/if}
					{#if roleScopability === 'device'}
						<span data-testid="role-scopability" data-scope="device">
							<Chip tone="idle">
								<Server class="size-3" />{m.roles_scopable_device()}
							</Chip>
						</span>
					{:else if roleScopability === 'user'}
						<span data-testid="role-scopability" data-scope="user">
							<Chip tone="idle">
								<Users class="size-3" />{m.roles_scopable_user()}
							</Chip>
						</span>
					{/if}
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

				{#if role.isSystem}
					<p class="text-xs text-muted-foreground">{m.roles_editor_system_locked()}</p>
				{/if}
				{#if roleScopability === 'mixed'}
					<p class="max-w-[80ch] text-xs text-muted-foreground">{m.roles_scopability_mixed()}</p>
				{/if}
				<div class="grid gap-3 sm:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="editName">{m.roles_name()}</Label>
						<Input
							id="editName"
							bind:value={editName}
							disabled={role.isSystem}
							aria-invalid={!nameValid}
							placeholder={m.roles_name_placeholder()}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="editDesc">{m.roles_description_field()}</Label>
						<Input
							id="editDesc"
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
					<Button variant="outline" size="sm" onclick={selectAll} disabled={!allPermissions.length}>
						{m.roles_select_all()}
					</Button>
					<Button
						variant="outline"
						size="sm"
						onclick={deselectAll}
						disabled={selectedPermissions.size === 0}
					>
						{m.roles_deselect_all()}
					</Button>
				</span>
			</div>

			<PermissionMatrix
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
