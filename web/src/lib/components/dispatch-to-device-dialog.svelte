<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';
	import { apiClient, fetchAllPages, type Device } from '$lib/sdk';
	import { DeviceStatus } from '$sdk/powermanage/v1/common_pb';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import { Search } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	// Dispatches a single existing action OR a single action set to ONE
	// device, on demand (no schedule). The existing run-script-dialog
	// handles the opposite direction (pick an action from device-detail);
	// this dialog is the missing inverse — open it from action-detail or
	// action-set-detail to pick a device and "run now".
	//
	// Exactly one of `actionId` / `actionSetId` should be set. The dialog
	// picks the matching dispatch RPC at submit time.
	interface Props {
		open: boolean;
		title: string;
		actionId?: string;
		actionSetId?: string;
		ondispatched?: () => void;
	}

	let {
		open = $bindable(),
		title,
		actionId,
		actionSetId,
		ondispatched
	}: Props = $props();

	let devices = $state<Device[]>([]);
	let loading = $state(false);
	let selectedDeviceId = $state<string | null>(null);
	let dispatching = $state(false);
	let searchQuery = $state('');

	const filtered = $derived(
		searchQuery
			? devices.filter(
					(d) =>
						d.hostname.toLowerCase().includes(searchQuery.toLowerCase()) ||
						d.id.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: devices
	);

	$effect(() => {
		if (open) {
			selectedDeviceId = null;
			searchQuery = '';
			loadDevices();
		}
	});

	async function loadDevices() {
		loading = true;
		try {
			devices = await fetchAllPages<Device>(async (size, token) => {
				const r = await apiClient.listDevices(size, token);
				return { items: r.devices, nextPageToken: r.nextPageToken };
			});
		} catch (error) {
			console.warn('Failed to load devices', error);
			devices = [];
		} finally {
			loading = false;
		}
	}

	async function dispatchSelected() {
		if (!selectedDeviceId) return;
		dispatching = true;
		try {
			if (actionSetId) {
				await apiClient.dispatchActionSet(selectedDeviceId, actionSetId);
			} else if (actionId) {
				await apiClient.dispatchAction(selectedDeviceId, actionId);
			} else {
				toast.error(m.dispatch_to_device_no_target());
				return;
			}
			toast.success(m.dispatch_to_device_dispatched());
			open = false;
			ondispatched?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			dispatching = false;
		}
	}

	function statusLabel(s: DeviceStatus): string {
		switch (s) {
			case DeviceStatus.ONLINE:
				return m.dispatch_to_device_status_online();
			case DeviceStatus.OFFLINE:
				return m.dispatch_to_device_status_offline();
			default:
				return m.dispatch_to_device_status_unknown();
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{title}</Dialog.Title>
			<Dialog.Description>{m.dispatch_to_device_description()}</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-3">
			<div class="relative">
				<Search class="text-muted-foreground absolute top-2.5 left-2 h-4 w-4" />
				<Input
					placeholder={m.dispatch_to_device_search_placeholder()}
					bind:value={searchQuery}
					class="pl-8"
				/>
			</div>

			{#if loading}
				<div class="text-muted-foreground py-8 text-center text-sm">
					{m.common_loading()}
				</div>
			{:else if filtered.length === 0}
				<div class="text-muted-foreground py-8 text-center text-sm">
					{m.dispatch_to_device_no_devices()}
				</div>
			{:else}
				<div class="max-h-96 overflow-auto rounded-md border">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>{m.dispatch_to_device_column_hostname()}</Table.Head>
								<Table.Head class="w-32">{m.dispatch_to_device_column_status()}</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each filtered as device (device.id)}
								<Table.Row
									data-selected={selectedDeviceId === device.id ? '' : undefined}
									class="cursor-pointer"
									onclick={() => (selectedDeviceId = device.id)}
								>
									<Table.Cell>
										<div>
											<span class="font-medium">{device.hostname}</span>
											<p class="text-muted-foreground line-clamp-1 text-xs">{device.id}</p>
										</div>
									</Table.Cell>
									<Table.Cell>
										<Badge variant={device.status === DeviceStatus.ONLINE ? 'default' : 'outline'}>
											{statusLabel(device.status)}
										</Badge>
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			{/if}
		</div>

		<Dialog.Footer class="mt-4">
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button onclick={dispatchSelected} disabled={!selectedDeviceId || dispatching}>
				{dispatching ? m.dispatch_to_device_dispatching() : m.dispatch_to_device_run_now()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
