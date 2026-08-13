<script lang="ts">
	import type { InventoryTableResult } from '$lib/sdk';
	import { formatTimestampDateTime } from '$lib/sdk';
	import * as Table from '$lib/components/ui/table';
	import { HardDrive, Network, Usb, CircuitBoard } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		inventory: InventoryTableResult[];
	}

	let { inventory }: Props = $props();

	const blockDevices = $derived(inventory.find((t) => t.tableName === 'block_devices'));
	const interfaces = $derived(inventory.find((t) => t.tableName === 'interface_details'));
	const interfaceAddresses = $derived(inventory.find((t) => t.tableName === 'interface_addresses'));
	const usbDevices = $derived(inventory.find((t) => t.tableName === 'usb_devices'));
	const pciDevices = $derived(inventory.find((t) => t.tableName === 'pci_devices'));

	const PREFERRED_COLUMNS: Record<string, string[]> = {
		'interface_details': ['interface', 'mac', 'type', 'mtu', 'link_speed', 'enabled'],
		'interface_addresses': ['interface', 'address', 'mask', 'broadcast', 'type'],
	};

	function getColumns(table: InventoryTableResult | undefined): string[] {
		if (!table || table.rows.length === 0) return [];
		const preferred = PREFERRED_COLUMNS[table.tableName];
		if (preferred) {
			const available = new Set(Object.keys(table.rows[0].data));
			return preferred.filter((col) => available.has(col));
		}
		return Object.keys(table.rows[0].data).sort();
	}

	const hasNetwork = $derived(
		(interfaces && interfaces.rows.length > 0) ||
			(interfaceAddresses && interfaceAddresses.rows.length > 0)
	);
</script>

<div class="space-y-6">
	{#if !blockDevices && !hasNetwork && !usbDevices && !pciDevices}
		<div
			class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface py-12 text-center shadow-plate"
		>
			<CircuitBoard class="mb-2 h-8 w-8 text-muted-foreground" />
			<p class="text-muted-foreground">{m.hardware_no_data()}</p>
		</div>
	{:else}
		{#if blockDevices && blockDevices.rows.length > 0}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<span class="flex items-center gap-2">
						<HardDrive class="h-4 w-4 text-faint" />
						<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{m.hardware_storage()}</span>
					</span>
					{#if blockDevices.collectedAt}
						<p class="text-xs text-muted-foreground">
							{m.inventory_collected_at({ timestamp: formatTimestampDateTime(blockDevices.collectedAt) })}
						</p>
					{/if}
				</div>
				<div class="mt-3">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								{#each getColumns(blockDevices) as col}
									<Table.Head class="whitespace-nowrap">{col}</Table.Head>
								{/each}
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each blockDevices.rows as row}
								<Table.Row>
									{#each getColumns(blockDevices) as col}
										<Table.Cell class="text-xs whitespace-nowrap" title={row.data[col] ?? ''}>{row.data[col] ?? ''}</Table.Cell>
									{/each}
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			</section>
		{/if}

		{#if hasNetwork}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<span class="flex items-center gap-2">
						<Network class="h-4 w-4 text-faint" />
						<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{m.hardware_network()}</span>
					</span>
					{#if interfaces?.collectedAt}
						<p class="text-xs text-muted-foreground">
							{m.inventory_collected_at({ timestamp: formatTimestampDateTime(interfaces?.collectedAt) })}
						</p>
					{/if}
				</div>
				<div class="mt-3 space-y-4">
					{#if interfaces && interfaces.rows.length > 0}
						<div>
							<h4 class="text-sm font-medium mb-2">{m.hardware_network_interfaces()}</h4>
							<Table.Root>
								<Table.Header>
									<Table.Row>
										{#each getColumns(interfaces) as col}
											<Table.Head class="whitespace-nowrap">{col}</Table.Head>
										{/each}
									</Table.Row>
								</Table.Header>
								<Table.Body>
									{#each interfaces.rows as row}
										<Table.Row>
											{#each getColumns(interfaces) as col}
												<Table.Cell class="text-xs whitespace-nowrap">{row.data[col] ?? ''}</Table.Cell>
											{/each}
										</Table.Row>
									{/each}
								</Table.Body>
							</Table.Root>
						</div>
					{/if}
					{#if interfaceAddresses && interfaceAddresses.rows.length > 0}
						<div>
							<h4 class="text-sm font-medium mb-2">{m.hardware_network_addresses()}</h4>
							<Table.Root>
								<Table.Header>
									<Table.Row>
										{#each getColumns(interfaceAddresses) as col}
											<Table.Head class="whitespace-nowrap">{col}</Table.Head>
										{/each}
									</Table.Row>
								</Table.Header>
								<Table.Body>
									{#each interfaceAddresses.rows as row}
										<Table.Row>
											{#each getColumns(interfaceAddresses) as col}
												<Table.Cell class="text-xs whitespace-nowrap">{row.data[col] ?? ''}</Table.Cell>
											{/each}
										</Table.Row>
									{/each}
								</Table.Body>
							</Table.Root>
						</div>
					{/if}
				</div>
			</section>
		{/if}

		{#if usbDevices && usbDevices.rows.length > 0}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<span class="flex items-center gap-2">
						<Usb class="h-4 w-4 text-faint" />
						<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{m.hardware_usb()}</span>
					</span>
					{#if usbDevices.collectedAt}
						<p class="text-xs text-muted-foreground">
							{m.inventory_collected_at({ timestamp: formatTimestampDateTime(usbDevices.collectedAt) })}
						</p>
					{/if}
				</div>
				<div class="mt-3">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								{#each getColumns(usbDevices) as col}
									<Table.Head class="whitespace-nowrap">{col}</Table.Head>
								{/each}
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each usbDevices.rows as row}
								<Table.Row>
									{#each getColumns(usbDevices) as col}
										<Table.Cell class="text-xs whitespace-nowrap" title={row.data[col] ?? ''}>{row.data[col] ?? ''}</Table.Cell>
									{/each}
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			</section>
		{/if}

		{#if pciDevices && pciDevices.rows.length > 0}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<span class="flex items-center gap-2">
						<CircuitBoard class="h-4 w-4 text-faint" />
						<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{m.hardware_pci()}</span>
					</span>
					{#if pciDevices.collectedAt}
						<p class="text-xs text-muted-foreground">
							{m.inventory_collected_at({ timestamp: formatTimestampDateTime(pciDevices.collectedAt) })}
						</p>
					{/if}
				</div>
				<div class="mt-3">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								{#each getColumns(pciDevices) as col}
									<Table.Head class="whitespace-nowrap">{col}</Table.Head>
								{/each}
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each pciDevices.rows as row}
								<Table.Row>
									{#each getColumns(pciDevices) as col}
										<Table.Cell class="text-xs whitespace-nowrap" title={row.data[col] ?? ''}>{row.data[col] ?? ''}</Table.Cell>
									{/each}
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			</section>
		{/if}
	{/if}
</div>
