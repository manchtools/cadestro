<script lang="ts">

	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import RoleScopableIcon from '$lib/components/role-scopable-icon.svelte';
	import { PermissionTargetKind, RoleGrantScopeKind, type PermissionInfo } from '$lib/sdk';
	import { Globe, Search } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { groupPermissions } from './permission-groups';

	let {
		permissions,
		selected,
		columnLabel,
		onToggle,
		onToggleGroup
	}: {
		permissions: PermissionInfo[];
		selected: Set<string>;

		columnLabel: string;
		onToggle: (key: string) => void;
		onToggleGroup: (keys: string[], allSelected: boolean) => void;
	} = $props();

	let filter = $state('');

	const groups = $derived(groupPermissions(permissions));

	const visibleGroups = $derived.by(() => {
		const q = filter.trim().toLowerCase();
		if (!q) return groups;
		return groups
			.map((g) => ({
				name: g.name,
				permissions: g.permissions.filter(
					(p) =>
						p.key.toLowerCase().includes(q) ||
						p.description.toLowerCase().includes(q) ||
						g.name.toLowerCase().includes(q)
				)
			}))
			.filter((g) => g.permissions.length > 0);
	});

	function scopeKindOf(p: PermissionInfo): RoleGrantScopeKind | null {
		if (p.targetKind === PermissionTargetKind.DEVICE) return RoleGrantScopeKind.DEVICE_GROUP;
		if (p.targetKind === PermissionTargetKind.USER) return RoleGrantScopeKind.USER_GROUP;
		return null;
	}

	function groupState(perms: PermissionInfo[]): { all: boolean; some: boolean } {
		const on = perms.filter((p) => selected.has(p.key)).length;
		return { all: on > 0 && on === perms.length, some: on > 0 && on < perms.length };
	}
</script>

<section
	data-tour="roles-matrix"
	data-testid="roles-matrix"
	class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate"
>
	<header class="flex flex-wrap items-center gap-3 border-b border-hair px-4 py-3">
		<div class="min-w-0 flex-1">
			<h2 class="text-sm font-semibold">{m.roles_matrix_title()}</h2>
			<p class="mt-0.5 max-w-[70ch] text-xs text-muted-foreground">{m.roles_matrix_hint()}</p>
		</div>
		<div class="relative w-56 shrink-0">
			<Search class="absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
			<Input
				value={filter}
				oninput={(e) => (filter = e.currentTarget.value)}
				placeholder={m.roles_matrix_filter()}
				aria-label={m.roles_matrix_filter()}
				class="h-8 pl-8 text-xs"
			/>
		</div>
	</header>

	{#if permissions.length === 0}

		<p data-testid="roles-matrix-unavailable" class="bg-crit-soft px-4 py-6 text-sm text-crit">
			{m.roles_matrix_unavailable()}
		</p>
	{:else}
		<div class="grid grid-cols-[1fr_auto] items-center gap-x-4 border-b border-hair bg-sunken px-4 py-2">
			<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
				{m.roles_matrix_permission()}
			</span>
			<span
				class="max-w-[14rem] truncate text-right font-mono text-[0.62rem] tracking-[0.1em] text-accent-ink uppercase"
				title={columnLabel}
			>
				{columnLabel}
			</span>
		</div>

		{#each visibleGroups as group (group.name)}
			{@const gs = groupState(group.permissions)}
			<div class="border-b border-hair last:border-b-0">
				<div class="flex items-center gap-2.5 bg-sunken/60 px-4 py-1.5">
					<Checkbox
						checked={gs.all}
						indeterminate={gs.some}
						aria-label={m.roles_matrix_group_toggle({ group: group.name })}
						onCheckedChange={() =>
							onToggleGroup(
								group.permissions.map((p) => p.key),
								gs.all
							)}
					/>
					<span class="font-mono text-[0.68rem] font-semibold tracking-[0.08em] uppercase">
						{group.name}
					</span>
					<span class="ml-auto font-mono text-[0.65rem] tabular-nums text-faint">
						{group.permissions.filter((p) => selected.has(p.key)).length}/{group.permissions.length}
					</span>
				</div>

				<div class="grid gap-x-4 gap-y-px p-2 md:grid-cols-2 2xl:grid-cols-3">
					{#each group.permissions as perm (perm.key)}
						{@const scopeKind = scopeKindOf(perm)}
						<label
							data-testid="matrix-row"
							data-permission={perm.key}
							class="flex cursor-pointer items-start gap-2.5 rounded-md px-2 py-1.5 hover:bg-accent/40"
						>

							<span class="flex h-[1.15rem] shrink-0 items-center">
								<Checkbox
									checked={selected.has(perm.key)}
									aria-label={perm.key}
									onCheckedChange={() => onToggle(perm.key)}
								/>
							</span>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5">
									<span class="truncate font-mono text-[0.78rem]">{perm.key}</span>
									{#if scopeKind === null}
										<span
											class="inline-flex"
											title={m.roles_matrix_global_only()}
											aria-label={m.roles_matrix_global_only()}
										>
											<Globe class="size-3.5 text-faint" aria-hidden="true" />
										</span>
									{:else}
										<RoleScopableIcon {scopeKind} />
									{/if}
								</span>
								<span class="block truncate text-xs text-muted-foreground">{perm.description}</span>
							</span>
						</label>
					{/each}
				</div>
			</div>
		{/each}

		{#if visibleGroups.length === 0}
			<p class="px-4 py-6 text-sm text-muted-foreground">{m.common_no_results_search()}</p>
		{/if}
	{/if}
</section>

{#if permissions.length > 0}
	<p class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
		<span class="inline-flex items-center gap-1.5">
			<Globe class="size-3.5 text-faint" aria-hidden="true" />{m.roles_matrix_global_only()}
		</span>
		<span class="inline-flex items-center gap-1.5">
			<RoleScopableIcon scopeKind={RoleGrantScopeKind.DEVICE_GROUP} />{m.roles_scopable_device()}
		</span>
		<span class="inline-flex items-center gap-1.5">
			<RoleScopableIcon scopeKind={RoleGrantScopeKind.USER_GROUP} />{m.roles_scopable_user()}
		</span>
	</p>
{/if}
