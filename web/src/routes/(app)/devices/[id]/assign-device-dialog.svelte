<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';
	import { apiClient, type User, type UserGroup } from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Tabs from '$lib/components/ui/tabs';
	import ItemTablePicker from '$lib/components/item-table-picker.svelte';
	import * as Table from '$lib/components/ui/table';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;
		hostname: string;
		existingUserIds?: string[];
		existingGroupIds?: string[];
		onassign: (userIds: string[], groupIds: string[]) => void;
	}

	let { open = $bindable(), hostname, existingUserIds = [], existingGroupIds = [], onassign }: Props = $props();

	let activeTab = $state('users');
	let selectedUserIds = $state<string[]>([]);
	let selectedGroupIds = $state<string[]>([]);
	let users = $state<{ id: string; email: string }[]>([]);
	let groups = $state<{ id: string; name: string; description: string }[]>([]);
	let assigning = $state(false);

	$effect(() => {
		if (open) {
			selectedUserIds = [];
			selectedGroupIds = [];
			activeTab = 'users';
			loadData();
		}
	});

	async function loadData() {
		try {
			const [usersResp, groupsResp] = await Promise.all([
				apiClient.listUsers(),
				apiClient.listUserGroups()
			]);
			// Filter out disabled users and already-assigned items
			users = usersResp.users
				.filter((u: User) => !u.disabled && !existingUserIds.includes((u.id?.value ?? '')))
				.map((u: User) => ({ id: u.id?.value ?? '', email: u.email }));
			groups = groupsResp.groups
				.filter((g: UserGroup) => !existingGroupIds.includes((g.id?.value ?? '')))
				.map((g: UserGroup) => ({ id: g.id?.value ?? '', name: g.name, description: g.description }));
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function handleAssign() {
		if (selectedUserIds.length === 0 && selectedGroupIds.length === 0) return;
		assigning = true;
		onassign(selectedUserIds, selectedGroupIds);
		assigning = false;
	}

	const hasSelection = $derived(selectedUserIds.length > 0 || selectedGroupIds.length > 0);
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{m.devices_assign_dialog_title()}</Dialog.Title>
			<Dialog.Description>
				{m.devices_assign_dialog_description({ hostname })}
			</Dialog.Description>
		</Dialog.Header>

		<Tabs.Root bind:value={activeTab}>
			<Tabs.List>
				<Tabs.Trigger value="users">
					{m.picker_users_tab({ count: users.length })}
				</Tabs.Trigger>
				<Tabs.Trigger value="user_groups">
					{m.picker_user_groups_tab({ count: groups.length })}
				</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="users">
				<ItemTablePicker
					items={users}
					bind:selected={selectedUserIds}
					searchPlaceholder={m.picker_search_users()}
					emptyMessage={m.picker_no_users()}
					searchFilter={(item, query) =>
						item.email.toLowerCase().includes(query.toLowerCase())
					}
				>
					{#snippet headerRow()}
						<Table.Head>{m.users_table_email()}</Table.Head>
					{/snippet}
					{#snippet itemRow(item)}
						<Table.Cell>
							<span class="font-medium">{item.email}</span>
						</Table.Cell>
					{/snippet}
				</ItemTablePicker>
			</Tabs.Content>

			<Tabs.Content value="user_groups">
				<ItemTablePicker
					items={groups}
					bind:selected={selectedGroupIds}
					searchPlaceholder={m.picker_search_user_groups()}
					emptyMessage={m.picker_no_user_groups()}
					searchFilter={(item, query) =>
						item.name.toLowerCase().includes(query.toLowerCase()) ||
						(item.description?.toLowerCase().includes(query.toLowerCase()) ?? false)
					}
				>
					{#snippet headerRow()}
						<Table.Head>{m.common_name()}</Table.Head>
						<Table.Head>{m.common_description()}</Table.Head>
					{/snippet}
					{#snippet itemRow(item)}
						<Table.Cell>
							<span class="font-medium">{item.name}</span>
						</Table.Cell>
						<Table.Cell>
							<span class="text-muted-foreground">{item.description ?? ''}</span>
						</Table.Cell>
					{/snippet}
				</ItemTablePicker>
			</Tabs.Content>
		</Tabs.Root>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button onclick={handleAssign} disabled={!hasSelection || assigning}>
				{assigning ? m.devices_assigning() : m.common_assign()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
