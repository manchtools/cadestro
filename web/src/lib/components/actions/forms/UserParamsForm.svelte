<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import FieldError from '$lib/components/ui/field-error/field-error.svelte';
	import { Plus, X } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { UserFormState } from './types';

	interface Props {
		params: UserFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();

	function addKey() {
		params.sshAuthorizedKeys = [...params.sshAuthorizedKeys, ''];
	}

	function removeKey(index: number) {
		params.sshAuthorizedKeys = params.sshAuthorizedKeys.filter((_, i) => i !== index);
	}
</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label for="username">{m.actions_params_user_username()}</Label>
		<Input
			id="username"
			placeholder="johndoe"
			bind:value={params.username}
			required
			aria-invalid={!!errors?.username}
			oninput={() => onclearerror?.('username')}
		/>
		<FieldError error={errors?.username} />
		<p class="text-xs text-muted-foreground">{m.actions_params_user_username_hint()}</p>
	</div>

	<div class="grid grid-cols-2 gap-4">
		<div class="space-y-2">
			<Label for="uid">{m.actions_params_user_uid()}</Label>
			<Input
				id="uid"
				type="number"
				placeholder="1001"
				bind:value={params.uid}
				min="0"
				max="65534"
			/>
			<p class="text-xs text-muted-foreground">{m.actions_params_user_uid_hint()}</p>
		</div>
		<div class="space-y-2">
			<Label for="gid">{m.actions_params_user_gid()}</Label>
			<Input
				id="gid"
				type="number"
				placeholder="1001"
				bind:value={params.gid}
				min="0"
				max="65534"
			/>
			<p class="text-xs text-muted-foreground">{m.actions_params_user_gid_hint()}</p>
		</div>
	</div>

	<div class="space-y-2">
		<Label for="primaryGroup">{m.actions_params_user_primary_group()}</Label>
		<Input
			id="primaryGroup"
			placeholder="developers"
			bind:value={params.primaryGroup}
		/>
		<p class="text-xs text-muted-foreground">{m.actions_params_user_primary_group_hint()}</p>
	</div>

	<div class="space-y-2">
		<Label for="homeDir">{m.actions_params_user_home_dir()}</Label>
		<Input
			id="homeDir"
			placeholder="/home/johndoe"
			bind:value={params.homeDir}
		/>
		<p class="text-xs text-muted-foreground">{m.actions_params_user_home_dir_hint()}</p>
	</div>

	<div class="space-y-2">
		<Label for="shell">{m.actions_params_user_shell()}</Label>
		<Input
			id="shell"
			placeholder="/bin/bash"
			bind:value={params.shell}
		/>
		<p class="text-xs text-muted-foreground">{m.actions_params_user_shell_hint()}</p>
	</div>

	<div class="space-y-2">
		<Label for="comment">{m.actions_params_user_comment()}</Label>
		<Input
			id="comment"
			placeholder={m.actions_params_user_comment_placeholder()}
			bind:value={params.comment}
		/>
		<p class="text-xs text-muted-foreground">{m.actions_params_user_comment_hint()}</p>
	</div>

	<div class="space-y-2">
		<Label>{m.actions_params_user_ssh_keys()}</Label>
		{#each params.sshAuthorizedKeys as _, i}
			<div class="flex gap-2">
				<Input
					placeholder="ssh-ed25519 AAAAC3..."
					bind:value={params.sshAuthorizedKeys[i]}
					class="font-mono text-xs"
				/>
				<Button variant="ghost" size="icon" class="shrink-0" onclick={() => removeKey(i)}>
					<X class="h-4 w-4" />
				</Button>
			</div>
		{/each}
		<Button variant="outline" size="sm" onclick={addKey}>
			<Plus class="h-4 w-4 mr-1" />
			{m.actions_params_user_ssh_keys_add()}
		</Button>
		<p class="text-xs text-muted-foreground">{m.actions_params_user_ssh_keys_hint()}</p>
	</div>

	<div class="space-y-3 rounded-lg border p-4">
		<h4 class="text-sm font-medium">{m.actions_params_user_options()}</h4>
		<div class="flex items-center space-x-2">
			<Checkbox id="systemUser" bind:checked={params.systemUser} />
			<Label for="systemUser" class="text-sm font-normal cursor-pointer">
				{m.actions_params_user_system_user()}
			</Label>
		</div>
		<p class="text-xs text-muted-foreground ml-6">{m.actions_params_user_system_user_hint()}</p>

		<div class="flex items-center space-x-2">
			<Checkbox id="createHome" bind:checked={params.createHome} />
			<Label for="createHome" class="text-sm font-normal cursor-pointer">
				{m.actions_params_user_create_home()}
			</Label>
		</div>
		<p class="text-xs text-muted-foreground ml-6">{m.actions_params_user_create_home_hint()}</p>

		<div class="flex items-center space-x-2">
			<Checkbox id="disabled" bind:checked={params.disabled} />
			<Label for="disabled" class="text-sm font-normal cursor-pointer">
				{m.actions_params_user_disabled()}
			</Label>
		</div>
		<p class="text-xs text-muted-foreground ml-6">{m.actions_params_user_disabled_hint()}</p>

		<div class="flex items-center space-x-2">
			<Checkbox id="hidden" bind:checked={params.hidden} />
			<Label for="hidden" class="text-sm font-normal cursor-pointer">
				{m.actions_params_user_hidden()}
			</Label>
		</div>
		<p class="text-xs text-muted-foreground ml-6">{m.actions_params_user_hidden_hint()}</p>
	</div>
</div>
