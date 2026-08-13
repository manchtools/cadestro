<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import type { ManagedAction } from '$sdk/powermanage/v1/control_pb';
	import { Plus, X, UserCog } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { GroupFormState } from './types';
	import { getUserLoaders } from './user-loader-context.svelte';
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';

	interface Props {
		params: GroupFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
		/**
		 * Optional loader for the "pick from existing USER actions"
		 * dropdown. Falls back to the ancestor-provided loader from
		 * user-loader-context. Consumers without access to the
		 * control-plane API (e.g. the marketplace) can omit this and
		 * the dropdown shows an empty state. The web app sets the
		 * context to `apiClient.listActions(100, '', ActionType.USER)`.
		 */
		loadUserActions?: () => Promise<ManagedAction[]>;
	}

	let {
		params = $bindable(),
		errors = {},
		onclearerror,
		loadUserActions
	}: Props = $props();

	const ctx = getUserLoaders();
	const resolvedLoadUserActions = $derived(loadUserActions ?? ctx.loadUserActions);

	let userActions = $state<ManagedAction[]>([]);
	let loadingUsers = $state(false);

	$effect(() => {
		fetchUserActions();
	});

	async function fetchUserActions() {
		const loader = resolvedLoadUserActions;
		if (!loader) return;
		loadingUsers = true;
		try {
			userActions = await loader();
		} catch (e) {
			console.error('Failed to load user actions:', e);
			toast.error(getLocalizedError(e));
		} finally {
			loadingUsers = false;
		}
	}

	// Available user actions (exclude already added members)
	const availableUserActions = $derived.by(() => {
		const existing = new Set(params.members.map((u) => u.trim().toLowerCase()));
		return userActions.filter((a) => {
			if (a.params.case !== 'user') return false;
			return !existing.has(a.params.value.username.toLowerCase());
		});
	});

	function handleUserActionSelect(actionId: string) {
		const action = userActions.find((a) => a.id === actionId);
		if (action?.params.case === 'user') {
			const username = action.params.value.username;
			if (!params.members.some((u) => u.trim().toLowerCase() === username.toLowerCase())) {
				params.members = [...params.members, username];
				onclearerror?.('members');
			}
		}
	}

	let newMember = $state('');

	function addMember() {
		const trimmed = newMember.trim();
		if (!trimmed) return;
		if (!params.members.some((u) => u.trim().toLowerCase() === trimmed.toLowerCase())) {
			params.members = [...params.members, trimmed];
			onclearerror?.('members');
		}
		newMember = '';
	}

	function addMemberOnEnter(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			addMember();
		}
	}

	function removeMember(index: number) {
		params.members = params.members.filter((_, i) => i !== index);
	}

	function updateMember(index: number, value: string) {
		params.members = params.members.map((u, i) => (i === index ? value : u));
		onclearerror?.('members');
	}

	const memberCount = $derived(params.members.filter((u) => u.trim()).length);
</script>

<div class="space-y-4">
	<!-- Group Name -->
	<div class="space-y-1.5">
		<Label for="groupName">{m.group_name()}</Label>
		<Input
			id="groupName"
			placeholder={m.group_name_placeholder()}
			bind:value={params.name}
			required
			aria-invalid={!!errors?.name}
			oninput={() => onclearerror?.('name')}
		/>
		<FieldError error={errors.name} />
		<p class="text-xs text-muted-foreground">{m.group_name_description()}</p>
	</div>

	<!-- Members -->
	<div class="space-y-1.5">
		<Label>{m.group_members()} ({memberCount})</Label>

		<!-- Add member input + picker -->
		<div class="flex gap-2">
			<Input
				placeholder={m.group_members_add_placeholder()}
				bind:value={newMember}
				onkeydown={addMemberOnEnter}
				class="flex-1 font-mono text-sm"
			/>
			<Button
				type="button"
				variant="outline"
				size="sm"
				onclick={addMember}
				disabled={!newMember.trim()}
			>
				<Plus class="h-4 w-4 mr-1" />
				{m.group_members_add()}
			</Button>
			{#if availableUserActions.length > 0}
				<Select.Root
					type="single"
					value=""
					onValueChange={handleUserActionSelect}
				>
					<Select.Trigger class="w-[180px]">
						<UserCog class="h-4 w-4 mr-2" />
						{m.group_members_select()}
					</Select.Trigger>
					<Select.Content>
						{#each availableUserActions as userAction}
							{@const userParams = userAction.params.case === 'user' ? userAction.params.value : null}
							{#if userParams}
								<Select.Item value={userAction.id}>
									{userAction.name} ({userParams.username})
								</Select.Item>
							{/if}
						{/each}
					</Select.Content>
				</Select.Root>
			{:else if userActions.length === 0 && !loadingUsers}
				<Tooltip.Root>
					<Tooltip.Trigger>
						{#snippet child({ props })}
							<button
								type="button"
								disabled
								class="inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm border border-input bg-background px-3 h-9 w-[180px] opacity-50 cursor-not-allowed"
								{...props}
							>
								<UserCog class="h-4 w-4" />
								{m.group_members_select()}
							</button>
						{/snippet}
					</Tooltip.Trigger>
					<Tooltip.Content>
						{m.group_members_no_user_actions()}
					</Tooltip.Content>
				</Tooltip.Root>
			{/if}
		</div>

		<!-- Member list -->
		{#if params.members.length > 0}
			<div class="space-y-1.5">
				{#each params.members as member, index}
					<div class="flex gap-2 items-center">
						<Input
							value={member}
							oninput={(e) => updateMember(index, e.currentTarget.value)}
							class="flex-1 font-mono text-sm"
							placeholder={m.group_members_add_placeholder()}
						/>
						<Button
							type="button"
							variant="ghost"
							size="icon"
							onclick={() => removeMember(index)}
							class="shrink-0 text-destructive hover:text-destructive"
						>
							<X class="h-4 w-4" />
						</Button>
					</div>
				{/each}
			</div>
		{/if}

		<FieldError error={errors.members} />
		<p class="text-xs text-muted-foreground">{m.group_members_description()}</p>
	</div>

	<!-- GID -->
	<div class="space-y-1.5">
		<Label for="groupGid">{m.group_gid()}</Label>
		<Input
			id="groupGid"
			type="number"
			placeholder="1001"
			bind:value={params.gid}
			min="0"
			max="65534"
		/>
		<p class="text-xs text-muted-foreground">{m.group_gid_description()}</p>
	</div>

	<!-- System Group -->
	<div class="flex items-center space-x-2">
		<Checkbox id="systemGroup" bind:checked={params.systemGroup} />
		<Label for="systemGroup" class="text-sm font-normal cursor-pointer">
			{m.group_system_group()}
		</Label>
	</div>
	<p class="text-xs text-muted-foreground ml-6">{m.group_system_group_description()}</p>
</div>
