<script lang="ts">
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Search } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { DeviceStatus } from '$sdk/powermanage/v1/common_pb';

	type DeviceItem = {
		id: string;
		hostname: string;
		status: DeviceStatus;
	};

	type GroupItem = {
		id: string;
		name: string;
		description?: string;
	};

	type UserItem = {
		id: string;
		email: string;
	};

	let {
		devices = [],
		deviceGroups = [],
		users = [],
		userGroups = [],
		selectedDeviceIds = $bindable<string[]>([]),
		selectedGroupIds = $bindable<string[]>([]),
		selectedUserIds = $bindable<string[]>([]),
		selectedUserGroupIds = $bindable<string[]>([])
	}: {
		devices: DeviceItem[];
		deviceGroups: GroupItem[];
		users: UserItem[];
		userGroups: GroupItem[];
		selectedDeviceIds: string[];
		selectedGroupIds: string[];
		selectedUserIds: string[];
		selectedUserGroupIds: string[];
	} = $props();

	let activeTab = $state('groups');
	let deviceSearch = $state('');
	let groupSearch = $state('');
	let userSearch = $state('');
	let userGroupSearch = $state('');

	const filteredDevices = $derived(
		devices.filter(
			(d) =>
				d.hostname.toLowerCase().includes(deviceSearch.toLowerCase()) ||
				d.id.toLowerCase().includes(deviceSearch.toLowerCase())
		)
	);

	const filteredGroups = $derived(
		deviceGroups.filter(
			(g) =>
				g.name.toLowerCase().includes(groupSearch.toLowerCase()) ||
				(g.description?.toLowerCase().includes(groupSearch.toLowerCase()) ?? false)
		)
	);

	const filteredUsers = $derived(
		users.filter(
			(u) =>
				u.email.toLowerCase().includes(userSearch.toLowerCase()) ||
				u.id.toLowerCase().includes(userSearch.toLowerCase())
		)
	);

	const filteredUserGroups = $derived(
		userGroups.filter(
			(g) =>
				g.name.toLowerCase().includes(userGroupSearch.toLowerCase()) ||
				(g.description?.toLowerCase().includes(userGroupSearch.toLowerCase()) ?? false)
		)
	);

	const allDevicesSelected = $derived(
		filteredDevices.length > 0 && filteredDevices.every((d) => selectedDeviceIds.includes(d.id))
	);
	const someDevicesSelected = $derived(
		filteredDevices.some((d) => selectedDeviceIds.includes(d.id)) && !allDevicesSelected
	);

	const allGroupsSelected = $derived(
		filteredGroups.length > 0 && filteredGroups.every((g) => selectedGroupIds.includes(g.id))
	);
	const someGroupsSelected = $derived(
		filteredGroups.some((g) => selectedGroupIds.includes(g.id)) && !allGroupsSelected
	);

	const allUsersSelected = $derived(
		filteredUsers.length > 0 && filteredUsers.every((u) => selectedUserIds.includes(u.id))
	);
	const someUsersSelected = $derived(
		filteredUsers.some((u) => selectedUserIds.includes(u.id)) && !allUsersSelected
	);

	const allUserGroupsSelected = $derived(
		filteredUserGroups.length > 0 && filteredUserGroups.every((g) => selectedUserGroupIds.includes(g.id))
	);
	const someUserGroupsSelected = $derived(
		filteredUserGroups.some((g) => selectedUserGroupIds.includes(g.id)) && !allUserGroupsSelected
	);

	function toggleDevice(id: string) {
		if (selectedDeviceIds.includes(id)) {
			selectedDeviceIds = selectedDeviceIds.filter((s) => s !== id);
		} else {
			selectedDeviceIds = [...selectedDeviceIds, id];
		}
	}

	function toggleAllDevices() {
		if (allDevicesSelected) {
			const filteredIds = new Set(filteredDevices.map((d) => d.id));
			selectedDeviceIds = selectedDeviceIds.filter((id) => !filteredIds.has(id));
		} else {
			const existing = new Set(selectedDeviceIds);
			for (const d of filteredDevices) {
				existing.add(d.id);
			}
			selectedDeviceIds = [...existing];
		}
	}

	function toggleGroup(id: string) {
		if (selectedGroupIds.includes(id)) {
			selectedGroupIds = selectedGroupIds.filter((s) => s !== id);
		} else {
			selectedGroupIds = [...selectedGroupIds, id];
		}
	}

	function toggleAllGroups() {
		if (allGroupsSelected) {
			const filteredIds = new Set(filteredGroups.map((g) => g.id));
			selectedGroupIds = selectedGroupIds.filter((id) => !filteredIds.has(id));
		} else {
			const existing = new Set(selectedGroupIds);
			for (const g of filteredGroups) {
				existing.add(g.id);
			}
			selectedGroupIds = [...existing];
		}
	}

	function toggleUser(id: string) {
		if (selectedUserIds.includes(id)) {
			selectedUserIds = selectedUserIds.filter((s) => s !== id);
		} else {
			selectedUserIds = [...selectedUserIds, id];
		}
	}

	function toggleAllUsers() {
		if (allUsersSelected) {
			const filteredIds = new Set(filteredUsers.map((u) => u.id));
			selectedUserIds = selectedUserIds.filter((id) => !filteredIds.has(id));
		} else {
			const existing = new Set(selectedUserIds);
			for (const u of filteredUsers) {
				existing.add(u.id);
			}
			selectedUserIds = [...existing];
		}
	}

	function toggleUserGroup(id: string) {
		if (selectedUserGroupIds.includes(id)) {
			selectedUserGroupIds = selectedUserGroupIds.filter((s) => s !== id);
		} else {
			selectedUserGroupIds = [...selectedUserGroupIds, id];
		}
	}

	function toggleAllUserGroups() {
		if (allUserGroupsSelected) {
			const filteredIds = new Set(filteredUserGroups.map((g) => g.id));
			selectedUserGroupIds = selectedUserGroupIds.filter((id) => !filteredIds.has(id));
		} else {
			const existing = new Set(selectedUserGroupIds);
			for (const g of filteredUserGroups) {
				existing.add(g.id);
			}
			selectedUserGroupIds = [...existing];
		}
	}

	function getStatusVariant(status: DeviceStatus): 'default' | 'secondary' | 'outline' {
		switch (status) {
			case DeviceStatus.ONLINE:
				return 'default';
			case DeviceStatus.OFFLINE:
				return 'secondary';
			default:
				return 'outline';
		}
	}

	function getStatusLabel(status: DeviceStatus): string {
		switch (status) {
			case DeviceStatus.ONLINE:
				return m.devices_status_online();
			case DeviceStatus.OFFLINE:
				return m.devices_status_offline();
			default:
				return '';
		}
	}
</script>

<Tabs.Root bind:value={activeTab}>
	<Tabs.List>
		<Tabs.Trigger value="groups">
			{m.picker_groups_tab({ count: deviceGroups.length })}
		</Tabs.Trigger>
		<Tabs.Trigger value="devices">
			{m.picker_devices_tab({ count: devices.length })}
		</Tabs.Trigger>
		<Tabs.Trigger value="users">
			{m.picker_users_tab({ count: users.length })}
		</Tabs.Trigger>
		<Tabs.Trigger value="user_groups">
			{m.picker_user_groups_tab({ count: userGroups.length })}
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="groups">
		<div class="space-y-3">
			<div class="relative">
				<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					placeholder={m.picker_search_groups()}
					bind:value={groupSearch}
					class="pl-9"
				/>
			</div>

			{#if filteredGroups.length === 0}
				<p class="py-6 text-center text-sm text-muted-foreground">
					{groupSearch ? m.picker_no_groups_search() : m.picker_no_groups()}
				</p>
			{:else}
				<div class="max-h-64 overflow-y-auto rounded-md border">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head class="w-10">
									<Checkbox
										checked={allGroupsSelected}
										indeterminate={someGroupsSelected}
										onCheckedChange={toggleAllGroups}
									/>
								</Table.Head>
								<Table.Head>{m.common_name()}</Table.Head>
								<Table.Head>{m.common_description()}</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each filteredGroups as group (group.id)}
								{@const isSelected = selectedGroupIds.includes(group.id)}
								<Table.Row
									data-state={isSelected ? 'selected' : undefined}
									class="cursor-pointer"
									onclick={() => toggleGroup(group.id)}
								>
									<Table.Cell>
										<Checkbox
											checked={isSelected}
											onCheckedChange={() => toggleGroup(group.id)}
											onclick={(e: MouseEvent) => e.stopPropagation()}
										/>
									</Table.Cell>
									<Table.Cell>
										<span class="font-medium">{group.name}</span>
									</Table.Cell>
									<Table.Cell>
										<span class="text-muted-foreground">{group.description ?? ''}</span>
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
				<p class="text-xs text-muted-foreground">
					{m.picker_groups_selected({ count: selectedGroupIds.length })}
				</p>
			{/if}
		</div>
	</Tabs.Content>
	<Tabs.Content value="devices">
		<div class="space-y-3">
			<div class="relative">
				<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					placeholder={m.picker_search_devices()}
					bind:value={deviceSearch}
					class="pl-9"
				/>
			</div>

			{#if filteredDevices.length === 0}
				<p class="py-6 text-center text-sm text-muted-foreground">
					{deviceSearch ? m.picker_no_devices_search() : m.picker_no_devices()}
				</p>
			{:else}
				<div class="max-h-64 overflow-y-auto rounded-md border">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head class="w-10">
									<Checkbox
										checked={allDevicesSelected}
										indeterminate={someDevicesSelected}
										onCheckedChange={toggleAllDevices}
									/>
								</Table.Head>
								<Table.Head>{m.devices_table_hostname()}</Table.Head>
								<Table.Head>{m.common_status()}</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each filteredDevices as device (device.id)}
								{@const isSelected = selectedDeviceIds.includes(device.id)}
								<Table.Row
									data-state={isSelected ? 'selected' : undefined}
									class="cursor-pointer"
									onclick={() => toggleDevice(device.id)}
								>
									<Table.Cell>
										<Checkbox
											checked={isSelected}
											onCheckedChange={() => toggleDevice(device.id)}
											onclick={(e: MouseEvent) => e.stopPropagation()}
										/>
									</Table.Cell>
									<Table.Cell>
										<span class="font-medium">{device.hostname}</span>
									</Table.Cell>
									<Table.Cell>
										<Badge variant={getStatusVariant(device.status)}>
											{getStatusLabel(device.status)}
										</Badge>
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
				<p class="text-xs text-muted-foreground">
					{m.picker_devices_selected({ count: selectedDeviceIds.length })}
				</p>
			{/if}
		</div>
	</Tabs.Content>
	<Tabs.Content value="users">
		<div class="space-y-3">
			<div class="relative">
				<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					placeholder={m.picker_search_users()}
					bind:value={userSearch}
					class="pl-9"
				/>
			</div>

			{#if filteredUsers.length === 0}
				<p class="py-6 text-center text-sm text-muted-foreground">
					{userSearch ? m.picker_no_users_search() : m.picker_no_users()}
				</p>
			{:else}
				<div class="max-h-64 overflow-y-auto rounded-md border">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head class="w-10">
									<Checkbox
										checked={allUsersSelected}
										indeterminate={someUsersSelected}
										onCheckedChange={toggleAllUsers}
									/>
								</Table.Head>
								<Table.Head>{m.users_table_email()}</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each filteredUsers as user (user.id)}
								{@const isSelected = selectedUserIds.includes(user.id)}
								<Table.Row
									data-state={isSelected ? 'selected' : undefined}
									class="cursor-pointer"
									onclick={() => toggleUser(user.id)}
								>
									<Table.Cell>
										<Checkbox
											checked={isSelected}
											onCheckedChange={() => toggleUser(user.id)}
											onclick={(e: MouseEvent) => e.stopPropagation()}
										/>
									</Table.Cell>
									<Table.Cell>
										<span class="font-medium">{user.email}</span>
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
				<p class="text-xs text-muted-foreground">
					{m.picker_users_selected({ count: selectedUserIds.length })}
				</p>
			{/if}
		</div>
	</Tabs.Content>
	<Tabs.Content value="user_groups">
		<div class="space-y-3">
			<div class="relative">
				<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					placeholder={m.picker_search_user_groups()}
					bind:value={userGroupSearch}
					class="pl-9"
				/>
			</div>

			{#if filteredUserGroups.length === 0}
				<p class="py-6 text-center text-sm text-muted-foreground">
					{userGroupSearch ? m.picker_no_user_groups_search() : m.picker_no_user_groups()}
				</p>
			{:else}
				<div class="max-h-64 overflow-y-auto rounded-md border">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head class="w-10">
									<Checkbox
										checked={allUserGroupsSelected}
										indeterminate={someUserGroupsSelected}
										onCheckedChange={toggleAllUserGroups}
									/>
								</Table.Head>
								<Table.Head>{m.common_name()}</Table.Head>
								<Table.Head>{m.common_description()}</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each filteredUserGroups as group (group.id)}
								{@const isSelected = selectedUserGroupIds.includes(group.id)}
								<Table.Row
									data-state={isSelected ? 'selected' : undefined}
									class="cursor-pointer"
									onclick={() => toggleUserGroup(group.id)}
								>
									<Table.Cell>
										<Checkbox
											checked={isSelected}
											onCheckedChange={() => toggleUserGroup(group.id)}
											onclick={(e: MouseEvent) => e.stopPropagation()}
										/>
									</Table.Cell>
									<Table.Cell>
										<span class="font-medium">{group.name}</span>
									</Table.Cell>
									<Table.Cell>
										<span class="text-muted-foreground">{group.description ?? ''}</span>
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
				<p class="text-xs text-muted-foreground">
					{m.picker_user_groups_selected({ count: selectedUserGroupIds.length })}
				</p>
			{/if}
		</div>
	</Tabs.Content>
</Tabs.Root>
