<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import ItemTablePicker from '$lib/components/item-table-picker.svelte';
	import RoleScopePicker from '$lib/components/role-scope-picker.svelte';
	import RoleScopableIcon from '$lib/components/role-scopable-icon.svelte';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		fetchAllPages,
		type Role,
		type DeviceGroup,
		type UserGroup,
		type PermissionInfo,
		RoleGrantScopeKind,
		PermissionTargetKind
	} from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import * as m from '$lib/paraglide/messages';

	// One assign-role dialog for every entry point (user list, user detail, user-group
	// detail), so the role table, scopable-role icons and scope picker can't drift
	// apart again. The caller supplies the target label, which roles to hide, and the
	// assign action (assignRoleToUser vs assignRoleToUserGroup); everything else —
	// loading roles/groups/permissions, computing scopability, the picker — lives here.
	let {
		open = $bindable(false),
		title,
		subtitle,
		excludeRoleIds = [],
		assign,
		onAssigned
	}: {
		open: boolean;
		title: string;
		subtitle: string;
		/** Role ids to hide (already granted globally / inherited). */
		excludeRoleIds?: string[];
		assign: (roleIds: string[], scopeKind: RoleGrantScopeKind, scopeId: string) => Promise<void>;
		onAssigned: () => void;
	} = $props();

	let roles = $state<Role[]>([]);
	let deviceGroups = $state<DeviceGroup[]>([]);
	let allUserGroups = $state<UserGroup[]>([]);
	let permTargetKind = $state<Map<string, PermissionTargetKind>>(new Map());
	let selectedRoleIds = $state<string[]>([]);
	let scopeGroupId = $state('');

	// Reload fresh data + reset selection each time the dialog opens.
	$effect(() => {
		if (open) {
			selectedRoleIds = [];
			scopeGroupId = '';
			void loadAll();
		}
	});

	async function loadAll() {
		try {
			const [rolesResp, dg, ug, permsResp] = await Promise.all([
				apiClient.listRoles(),
				fetchAllPages<DeviceGroup>(async (size, token) => {
					const resp = await apiClient.listDeviceGroups(size, token);
					return { items: resp.groups, nextPageToken: resp.nextPageToken };
				}),
				fetchAllPages<UserGroup>(async (size, token) => {
					const resp = await apiClient.listUserGroups(size, token);
					return { items: resp.groups, nextPageToken: resp.nextPageToken };
				}),
				apiClient.listPermissions()
			]);
			roles = rolesResp.roles;
			deviceGroups = dg;
			allUserGroups = ug;
			permTargetKind = new Map(permsResp.permissions.map((p: PermissionInfo) => [p.key, p.targetKind]));
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	// A role is scopable when ALL its permissions share one scopable
	// target kind (DEVICE -> device group, USER -> user group). Driven by each
	// permission's target_kind from ListPermissions.
	function roleScopeKind(role: Role): RoleGrantScopeKind | null {
		if (role.permissions.length === 0) return null;
		let kind: RoleGrantScopeKind | null = null;
		for (const p of role.permissions) {
			const tk = permTargetKind.get(p);
			let want: RoleGrantScopeKind;
			if (tk === PermissionTargetKind.DEVICE) want = RoleGrantScopeKind.DEVICE_GROUP;
			else if (tk === PermissionTargetKind.USER) want = RoleGrantScopeKind.USER_GROUP;
			else return null;
			if (kind === null) kind = want;
			else if (kind !== want) return null;
		}
		return kind;
	}

	const excluded = $derived(new Set(excludeRoleIds));
	const unassignedRoles = $derived(roles.filter((r) => !excluded.has((r.id?.value ?? ''))));

	const selectedScopeKind = $derived.by<RoleGrantScopeKind | null>(() => {
		if (selectedRoleIds.length === 0) return null;
		let kind: RoleGrantScopeKind | null = null;
		for (const id of selectedRoleIds) {
			const role = roles.find((r) => (r.id?.value ?? '') === id);
			const k = role ? roleScopeKind(role) : null;
			if (k === null) return null;
			if (kind === null) kind = k;
			else if (kind !== k) return null;
		}
		return kind;
	});
	const showScopePicker = $derived(selectedScopeKind !== null);
	const scopeGroups = $derived([
		...deviceGroups.map((g) => ({ id: g.id?.value ?? '', name: g.name, kind: 'device' as const })),
		...allUserGroups.map((g) => ({ id: g.id?.value ?? '', name: g.name, kind: 'user' as const }))
	]);
	const scopeActiveKind = $derived(
		selectedScopeKind === RoleGrantScopeKind.USER_GROUP ? ('user' as const) : ('device' as const)
	);

	async function doAssign() {
		if (selectedRoleIds.length === 0) return;
		const useScope = selectedScopeKind !== null && scopeGroupId !== '';
		const scopeKind = useScope ? selectedScopeKind : RoleGrantScopeKind.UNSPECIFIED;
		try {
			await assign(selectedRoleIds, scopeKind, useScope ? scopeGroupId : '');
			toast.success(m.roles_assigned());
			open = false;
			onAssigned();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{title}</Dialog.Title>
			<Dialog.Description>{subtitle}</Dialog.Description>
		</Dialog.Header>

		<ItemTablePicker
			items={unassignedRoles.map((r) => ({
				id: r.id?.value ?? '',
				name: r.name,
				description: r.description,
				scopeKind: roleScopeKind(r)
			}))}
			bind:selected={selectedRoleIds}
			searchPlaceholder={m.picker_search_roles()}
			emptyMessage={m.picker_no_roles()}
			searchFilter={(item, query) =>
				item.name.toLowerCase().includes(query.toLowerCase()) ||
				(item.description?.toLowerCase().includes(query.toLowerCase()) ?? false)}
		>
			{#snippet headerRow()}
				<Table.Head>{m.roles_name()}</Table.Head>
				<Table.Head>{m.roles_description_field()}</Table.Head>
			{/snippet}
			{#snippet itemRow(item)}
				<Table.Cell class="py-2">
					<span class="inline-flex items-center gap-1.5">
						<span class="text-sm font-medium">{item.name}</span>
						<RoleScopableIcon scopeKind={item.scopeKind} />
					</span>
					<span class="block font-mono text-[0.68rem] text-faint">{item.id}</span>
				</Table.Cell>
				<Table.Cell class="py-2">
					<span class="line-clamp-1 text-xs text-muted-foreground">{item.description ?? ''}</span>
				</Table.Cell>
			{/snippet}
		</ItemTablePicker>

		{#if selectedRoleIds.length > 0}
			<RoleScopePicker
				groups={scopeGroups}
				activeKind={scopeActiveKind}
				bind:scopeId={scopeGroupId}
				disabled={!showScopePicker}
				label={m.roles_scope_label()}
				allOptionLabel={showScopePicker
					? selectedScopeKind === RoleGrantScopeKind.USER_GROUP
						? m.roles_scope_all_users()
						: m.roles_scope_all_devices()
					: m.roles_scope_org_wide()}
				hint={showScopePicker
					? selectedScopeKind === RoleGrantScopeKind.USER_GROUP
						? m.roles_scope_user_hint()
						: m.roles_scope_hint()
					: m.roles_scopability_mixed()}
			/>
		{/if}

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button onclick={doAssign} disabled={selectedRoleIds.length === 0}>{m.common_assign()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
