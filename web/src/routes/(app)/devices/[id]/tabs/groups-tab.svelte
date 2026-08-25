<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$lib/navigation';
	import { apiClient, type DeviceGroup } from '$lib/sdk';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { FolderOpen, ExternalLink } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		deviceId: string;
	}

	let { deviceId }: Props = $props();

	let groups = $state<DeviceGroup[]>([]);
	let loading = $state(true);

	let selectedGroup = $state<DeviceGroup | null>(null);
	let sheetOpen = $state(false);

	onMount(() => {
		loadGroups();
	});

	async function loadGroups() {
		loading = true;
		try {
			const response = await apiClient.listDeviceGroupsForDevice(deviceId);
			groups = response.groups;
		} catch (error) {
			console.error('Failed to load device groups:', error);
		} finally {
			loading = false;
		}
	}

	function openGroupSheet(group: DeviceGroup) {
		selectedGroup = group;
		sheetOpen = true;
	}
</script>

<div class="space-y-4">
	{#if loading}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<div class="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
		</div>
	{:else if groups.length === 0}
		<div
			class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface py-12 text-center shadow-plate"
		>
			<FolderOpen class="mb-2 h-8 w-8 text-muted-foreground" />
			<p class="text-muted-foreground">{m.device_groups_tab_empty()}</p>
		</div>
	{:else}
		<section class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate">
			<div class="flex items-center gap-2 border-b border-hair px-4 py-3">
				<FolderOpen class="h-4 w-4 text-faint" />
				<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.device_groups_tab_title()}
				</span>
			</div>
			<div>
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>{m.common_name()}</Table.Head>
							<Table.Head class="w-[120px]">{m.common_type()}</Table.Head>
							<Table.Head class="w-[100px]">{m.device_groups_tab_members()}</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each groups as group}
							<Table.Row class="cursor-pointer hover:bg-muted/50" onclick={() => openGroupSheet(group)}>
								<Table.Cell>
									<div class="space-y-0.5">
										<div class="font-medium">{group.name}</div>
										{#if group.description}
											<div class="text-xs text-muted-foreground">{group.description}</div>
										{/if}
									</div>
								</Table.Cell>
								<Table.Cell>
									<Badge variant="outline" class="text-xs">
										{group.isDynamic ? m.device_groups_tab_type_dynamic() : m.device_groups_tab_type_static()}
									</Badge>
								</Table.Cell>
								<Table.Cell class="text-sm text-muted-foreground">
									{group.memberCount}
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			</div>
		</section>
	{/if}
</div>

<Sheet.Root bind:open={sheetOpen}>
	<Sheet.Content side="right">
		<Sheet.Header>
			<Sheet.Title>{m.device_groups_tab_detail()}</Sheet.Title>
		</Sheet.Header>
		{#if selectedGroup}
			<div class="space-y-6 px-4 py-4">

				<div class="space-y-2">
					<div class="flex items-center gap-2">
						<FolderOpen class="h-4 w-4 text-muted-foreground" />
						<span class="text-sm text-muted-foreground">{m.nav_device_groups()}</span>
					</div>
					<h3 class="font-semibold text-lg">{selectedGroup.name}</h3>
					{#if selectedGroup.description}
						<p class="text-sm text-muted-foreground">
							{selectedGroup.description}
						</p>
					{/if}
					<div class="flex items-center gap-1.5">
						<Badge variant="outline" class="text-xs">
							{selectedGroup.isDynamic ? m.device_groups_tab_type_dynamic() : m.device_groups_tab_type_static()}
						</Badge>
						<Badge variant="secondary" class="text-xs">
							{m.device_groups_tab_members()}: {selectedGroup.memberCount}
						</Badge>
					</div>
				</div>

				{#if selectedGroup.isDynamic && selectedGroup.dynamicQuery}
					<div class="space-y-2">
						<h4 class="text-sm font-medium text-muted-foreground">
							{m.device_groups_tab_dynamic_query()}
						</h4>
						<div class="rounded-md bg-muted p-3 font-mono text-xs">
							<pre class="whitespace-pre-wrap">{selectedGroup.dynamicQuery}</pre>
						</div>
					</div>
				{/if}
			</div>
			<Sheet.Footer>
				<Button
					variant="outline"
					class="w-full"
					onclick={() => {
						sheetOpen = false;
						goto(`/device-groups/${selectedGroup!.id}`);
					}}
				>
					<ExternalLink class="h-4 w-4 mr-2" />
					{m.device_groups_tab_open_group()}
				</Button>
			</Sheet.Footer>
		{/if}
	</Sheet.Content>
</Sheet.Root>
