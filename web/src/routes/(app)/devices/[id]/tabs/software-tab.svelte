<script lang="ts">
	import { onMount } from 'svelte';
	import { apiClient, type InventoryTableResult, type ManagedAction } from '$lib/sdk';
	import { formatTimestampDateTime } from '$lib/sdk';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import * as Table from '$lib/components/ui/table';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Package, Shield } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import ManagePackageDialog from './manage-package-dialog.svelte';

	interface Props {
		inventory: InventoryTableResult[];
		deviceId: string;
	}

	let { inventory, deviceId }: Props = $props();

	let search = $state('');

	let manageDialogOpen = $state(false);
	let selectedPackage = $state({ name: '', version: '', source: '' });

	let assignedActions = $state<ManagedAction[]>([]);
	let assignedActionIds = $derived(new Set(assignedActions.map((a) => a.id)));

	const managedPackages = $derived.by(() => {
		const map = new Map<string, { actionId: string; actionName: string }>();
		for (const action of assignedActions) {
			if (action.type !== ActionType.PACKAGE) continue;
			if (action.params.case !== 'package') continue;
			const p = action.params.value;
			const entry = { actionId: action.id?.value ?? '', actionName: action.name };
			if (p.name) map.set(p.name.toLowerCase(), entry);
			if (p.aptName) map.set(p.aptName.toLowerCase(), entry);
			if (p.dnfName) map.set(p.dnfName.toLowerCase(), entry);
			if (p.pacmanName) map.set(p.pacmanName.toLowerCase(), entry);
			if (p.zypperName) map.set(p.zypperName.toLowerCase(), entry);
		}
		return map;
	});

	const SUPPORTED_SOURCES = new Set(['deb', 'rpm', 'system']);

	onMount(() => {
		loadDeviceAssignments();
	});

	async function loadDeviceAssignments() {
		try {
			const response = await apiClient.getDeviceAssignments(deviceId);
			assignedActions = response.actions;
		} catch (error) {
			console.error('Failed to load device assignments:', error);
		}
	}

	function openManageDialog(pkg: { name: string; version: string; source: string }) {
		selectedPackage = pkg;
		manageDialogOpen = true;
	}

	function handleAssigned() {
		loadDeviceAssignments();
	}

	const packageTables = $derived(
		inventory.filter((t) =>
			['deb_packages', 'rpm_packages', 'python_packages', 'packages'].includes(t.tableName)
		)
	);

	interface PackageRow {
		name: string;
		version: string;
		arch: string;
		source: string;
	}

	const allPackages = $derived.by(() => {
		const pkgs: PackageRow[] = [];
		for (const table of packageTables) {
			const source = table.tableName.replace('_packages', '').replace('packages', 'system');
			for (const row of table.rows) {
				pkgs.push({
					name: row.data['name'] ?? '',
					version: row.data['version'] ?? '',
					arch: row.data['arch'] ?? '',
					source
				});
			}
		}
		pkgs.sort((a, b) => a.name.localeCompare(b.name));
		return pkgs;
	});

	const filteredPackages = $derived(
		search
			? allPackages.filter(
					(p) =>
						p.name.toLowerCase().includes(search.toLowerCase()) ||
						p.version.toLowerCase().includes(search.toLowerCase())
				)
			: allPackages
	);

	const collectedAt = $derived(packageTables.length > 0 ? packageTables[0].collectedAt : undefined);
</script>

<div class="space-y-4">
	{#if packageTables.length === 0}
		<div
			class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface py-12 text-center shadow-plate"
		>
			<Package class="mb-2 h-8 w-8 text-muted-foreground" />
			<p class="text-muted-foreground">{m.software_no_data()}</p>
		</div>
	{:else}
		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<span class="flex items-center gap-2">
					<Package class="h-4 w-4 text-faint" />
					<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
						{m.software_packages()}
					</span>
				</span>
				<span class="text-sm text-muted-foreground">{m.software_count({ count: String(allPackages.length) })}</span>
			</div>
			{#if collectedAt}
				<p class="mt-1 text-xs text-muted-foreground">{m.inventory_collected_at({ timestamp: formatTimestampDateTime(collectedAt) })}</p>
			{/if}
			<div class="mt-3 space-y-4">
				<Input
					type="text"
					placeholder={m.software_search()}
					bind:value={search}
				/>

				<div class="max-h-[600px] overflow-auto">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>{m.software_name()}</Table.Head>
								<Table.Head>{m.software_version()}</Table.Head>
								<Table.Head>{m.software_arch()}</Table.Head>
								<Table.Head>{m.software_source()}</Table.Head>
								<Table.Head class="w-[100px]"></Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each filteredPackages.slice(0, 200) as pkg}
								{@const managed = managedPackages.get(pkg.name.toLowerCase())}
								{@const supported = SUPPORTED_SOURCES.has(pkg.source)}
								<Table.Row>
									<Table.Cell class="font-medium text-sm">{pkg.name}</Table.Cell>
									<Table.Cell class="text-xs font-mono">{pkg.version}</Table.Cell>
									<Table.Cell class="text-xs">{pkg.arch}</Table.Cell>
									<Table.Cell class="text-xs">{pkg.source}</Table.Cell>
									<Table.Cell class="text-right">
										{#if managed}
											<Badge variant="secondary" class="text-xs">
												<Shield class="mr-1 h-3 w-3" />
												{m.software_managed()}
											</Badge>
										{:else if supported}
											<Button
												variant="ghost"
												size="sm"
												class="h-7 text-xs"
												onclick={() => openManageDialog(pkg)}
											>
												{m.software_manage()}
											</Button>
										{/if}
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
					{#if filteredPackages.length > 200}
						<p class="text-sm text-muted-foreground text-center py-2">
							{m.software_truncated({ shown: 200, total: filteredPackages.length })}
						</p>
					{/if}
				</div>
			</div>
		</section>
	{/if}
</div>

<ManagePackageDialog
	bind:open={manageDialogOpen}
	packageName={selectedPackage.name}
	packageVersion={selectedPackage.version}
	{deviceId}
	assignedActionIds={new Set([...assignedActionIds].map((id) => id?.value ?? ''))}
	onassigned={handleAssigned}
/>
