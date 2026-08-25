<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import * as Tabs from '$lib/components/ui/tabs';
	import type { ManagedAction } from '$contract/cadestro/v1/control_pb';
	import { Plus, X, UserCog, UserRound } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getUserLoaders, type UserLite } from './user-loader-context.svelte';
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';

	export type { UserLite };

	interface Props {
		usernames: string[];
		errors?: Partial<Record<string, string>>;
		errorField?: string;
		onclearerror?: (field: string) => void;
		label: string;
		addLabel: string;
		placeholder: string;
		description: string;

		loadUserActions?: () => Promise<ManagedAction[]>;

		loadPlatformUsers?: () => Promise<UserLite[]>;
	}

	let {
		usernames = $bindable(),
		errors = {},
		errorField = 'users',
		onclearerror,
		label,
		addLabel,
		placeholder,
		description,
		loadUserActions,
		loadPlatformUsers
	}: Props = $props();

	const ctx = getUserLoaders();
	const resolvedLoadUserActions = $derived(loadUserActions ?? ctx.loadUserActions);
	const resolvedLoadPlatformUsers = $derived(loadPlatformUsers ?? ctx.loadPlatformUsers);

	let activeTab = $state('user_actions');

	let userActions = $state<ManagedAction[]>([]);
	let loadingUserActions = $state(false);

	let platformUsers = $state<UserLite[]>([]);
	let loadingPlatformUsers = $state(false);

	$effect(() => {
		fetchUserActions();
		fetchPlatformUsers();
	});

	async function fetchUserActions() {
		const loader = resolvedLoadUserActions;
		if (!loader) return;
		loadingUserActions = true;
		try {
			userActions = await loader();
		} catch (e) {
			console.error('Failed to load user actions:', e);

			toast.error(getLocalizedError(e));
		} finally {
			loadingUserActions = false;
		}
	}

	async function fetchPlatformUsers() {
		const loader = resolvedLoadPlatformUsers;
		if (!loader) return;
		loadingPlatformUsers = true;
		try {
			const list = await loader();
			platformUsers = list.filter((u) => u.linuxUsername && !u.disabled);
		} catch (e) {
			console.error('Failed to load platform users:', e);
			toast.error(getLocalizedError(e));
		} finally {
			loadingPlatformUsers = false;
		}
	}

	const availableUserActions = $derived.by(() => {
		const existing = new Set(usernames.map((u) => u.trim().toLowerCase()));
		return userActions.filter((a) => {
			if (a.params.case !== 'user') return false;
			return !existing.has(a.params.value.username.toLowerCase());
		});
	});

	const availablePlatformUsers = $derived.by(() => {
		const existing = new Set(usernames.map((u) => u.trim().toLowerCase()));
		return platformUsers.filter(
			(u) => !existing.has(u.linuxUsername.toLowerCase())
		);
	});

	function handleUserActionSelect(actionId: string) {
		const action = userActions.find((a) => (a.id?.value ?? '') === actionId);
		if (action?.params.case === 'user') {
			const username = action.params.value.username;
			if (!usernames.some((u) => u.trim().toLowerCase() === username.toLowerCase())) {
				usernames = [...usernames, username];
				onclearerror?.(errorField);
			}
		}
	}

	function handlePlatformUserSelect(userId: string) {
		const user = platformUsers.find((u) => u.id === userId);
		if (user?.linuxUsername) {
			const username = user.linuxUsername;
			if (!usernames.some((u) => u.trim().toLowerCase() === username.toLowerCase())) {
				usernames = [...usernames, username];
				onclearerror?.(errorField);
			}
		}
	}

	let newUsername = $state('');

	function addUser() {
		const trimmed = newUsername.trim();
		if (!trimmed) return;
		if (!usernames.some((u) => u.trim().toLowerCase() === trimmed.toLowerCase())) {
			usernames = [...usernames, trimmed];
			onclearerror?.(errorField);
		}
		newUsername = '';
	}

	function addUserOnEnter(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			addUser();
		}
	}

	function removeUser(index: number) {
		usernames = usernames.filter((_, i) => i !== index);
	}

	function updateUser(index: number, value: string) {
		usernames = usernames.map((u, i) => (i === index ? value : u));
		onclearerror?.(errorField);
	}

	const userCount = $derived(usernames.filter((u) => u.trim()).length);
</script>

<div class="space-y-1.5">
	<Label>{label} ({userCount})</Label>

	<div class="flex gap-2">
		<Input
			{placeholder}
			bind:value={newUsername}
			onkeydown={addUserOnEnter}
			class="flex-1 font-mono text-sm"
		/>
		<Button
			type="button"
			variant="outline"
			size="sm"
			onclick={addUser}
			disabled={!newUsername.trim()}
		>
			<Plus class="h-4 w-4 mr-1" />
			{addLabel}
		</Button>
	</div>

	<Tabs.Root bind:value={activeTab}>
		<Tabs.List class="w-full grid grid-cols-2">
			<Tabs.Trigger value="user_actions">{m.user_picker_tab_user_actions()}</Tabs.Trigger>
			<Tabs.Trigger value="users">{m.user_picker_tab_users()}</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="user_actions" class="pt-2">
			{#if availableUserActions.length > 0}
				<Select.Root
					type="single"
					value=""
					onValueChange={handleUserActionSelect}
				>
					<Select.Trigger class="w-full">
						<UserCog class="h-4 w-4 mr-2" />
						{m.user_picker_select_user_action()}
					</Select.Trigger>
					<Select.Content>
						{#each availableUserActions as userAction}
							{@const userParams = userAction.params.case === 'user' ? userAction.params.value : null}
							{#if userParams}
								<Select.Item value={(userAction.id?.value ?? '')}>
									{userAction.name} ({userParams.username})
								</Select.Item>
							{/if}
						{/each}
					</Select.Content>
				</Select.Root>
			{:else if userActions.length === 0 && !loadingUserActions}
				<p class="text-sm text-muted-foreground py-2">{m.user_picker_no_user_actions()}</p>
			{:else}
				<p class="text-sm text-muted-foreground py-2">{m.user_picker_no_user_actions()}</p>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="users" class="pt-2">
			{#if availablePlatformUsers.length > 0}
				<Select.Root
					type="single"
					value=""
					onValueChange={handlePlatformUserSelect}
				>
					<Select.Trigger class="w-full">
						<UserRound class="h-4 w-4 mr-2" />
						{m.user_picker_select_user()}
					</Select.Trigger>
					<Select.Content>
						{#each availablePlatformUsers as user}
							<Select.Item value={user.id}>
								<span class="font-mono">{user.linuxUsername}</span>
								<span class="text-muted-foreground ml-1">({user.email})</span>
							</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			{:else if !loadingPlatformUsers}
				<p class="text-sm text-muted-foreground py-2">{m.user_picker_no_users()}</p>
			{/if}
		</Tabs.Content>
	</Tabs.Root>

	{#if usernames.length > 0}
		<div class="space-y-1.5">
			{#each usernames as user, index}
				<div class="flex gap-2 items-center">
					<Input
						value={user}
						oninput={(e) => updateUser(index, e.currentTarget.value)}
						class="flex-1 font-mono text-sm"
						{placeholder}
					/>
					<Button
						type="button"
						variant="ghost"
						size="icon"
						onclick={() => removeUser(index)}
						class="shrink-0 text-destructive hover:text-destructive"
					>
						<X class="h-4 w-4" />
					</Button>
				</div>
			{/each}
		</div>
	{/if}

	<FieldError error={errors[errorField]} />
	<p class="text-xs text-muted-foreground">{description}</p>
</div>
