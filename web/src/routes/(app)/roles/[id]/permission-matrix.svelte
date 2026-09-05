<script lang="ts">

	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Permission } from '$contract/cadestro/v1/control_pb';
	import { Globe, Search } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { groupPermissions } from './permission-groups';

	let {
		permissions,
		selected,
		columnLabel,
		onToggle,
		onToggleGroup,
		disabled = false
	}: {
		disabled?: boolean;
		permissions: Permission[];
		selected: Set<Permission>;

		columnLabel: string;
		onToggle: (key: Permission) => void;
		onToggleGroup: (keys: Permission[], allSelected: boolean) => void;
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
						Permission[p].toLowerCase().includes(q) ||

						g.name.toLowerCase().includes(q)
				)
			}))
			.filter((g) => g.permissions.length > 0);
	});

	function groupState(perms: Permission[]): { all: boolean; some: boolean } {
		const on = perms.filter((p) => selected.has(p)).length;
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
					<Checkbox {disabled}
						checked={gs.all}
						indeterminate={gs.some}
						aria-label={m.roles_matrix_group_toggle({ group: group.name })}
						onCheckedChange={() =>
							onToggleGroup(
								group.permissions.map((p) => p),
								gs.all
							)}
					/>
					<span class="font-mono text-[0.68rem] font-semibold tracking-[0.08em] uppercase">
						{group.name}
					</span>
					<span class="ml-auto font-mono text-[0.65rem] tabular-nums text-faint">
						{group.permissions.filter((p) => selected.has(p)).length}/{group.permissions.length}
					</span>
				</div>

				<div class="grid gap-x-4 gap-y-px p-2 md:grid-cols-2 2xl:grid-cols-3">
					{#each group.permissions as perm (perm)}
						<label
							data-testid="matrix-row"
							data-permission={perm}
							class="flex cursor-pointer items-start gap-2.5 rounded-md px-2 py-1.5 hover:bg-accent/40"
						>

							<span class="flex h-[1.15rem] shrink-0 items-center">
								<Checkbox {disabled}
									checked={selected.has(perm)}
									aria-label={Permission[perm]}
									onCheckedChange={() => onToggle(perm)}
								/>
							</span>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5">
									<span class="truncate font-mono text-[0.78rem]">{Permission[perm].replaceAll('_', ' ')}</span>

								</span>

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
