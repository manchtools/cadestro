<script lang="ts">
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as m from '$lib/paraglide/messages';
	import type { SshFormState } from './types';
	import UserPicker from './UserPicker.svelte';

	interface Props {
		params: SshFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let {
		params = $bindable(),
		errors = {},
		onclearerror
	}: Props = $props();

	// Config preview
	const configPreview = $derived.by(() => {
		const users = params.users.filter((u) => u.trim());
		const userList = users.length > 0 ? users.join(',') : '<username>';
		const lines = [`Match User ${userList}`];
		lines.push(`    PubkeyAuthentication ${params.allowPubkey ? 'yes' : 'no'}`);
		if (params.allowPubkey) {
			lines.push(`    AuthorizedKeysFile .ssh/authorized_keys`);
		}
		lines.push(`    PasswordAuthentication ${params.allowPassword ? 'yes' : 'no'}`);
		return lines.join('\n');
	});
</script>

<div class="space-y-4">
	<!-- Users -->
	<UserPicker
		bind:usernames={params.users}
		{errors}
		errorField="users"
		{onclearerror}
		label={m.ssh_users()}
		addLabel={m.ssh_users_add()}
		placeholder={m.ssh_users_add_placeholder()}
		description={m.ssh_users_description()}
	/>

	<!-- Authentication toggles -->
	<div class="space-y-3">
		<div class="flex items-center justify-between rounded-lg border p-3">
			<div class="space-y-0.5">
				<Label for="allow-pubkey" class="cursor-pointer">{m.ssh_allow_pubkey()}</Label>
				<p class="text-xs text-muted-foreground">{m.ssh_allow_pubkey_hint()}</p>
			</div>
			<Switch id="allow-pubkey" bind:checked={params.allowPubkey} />
		</div>

		<div class="flex items-center justify-between rounded-lg border p-3">
			<div class="space-y-0.5">
				<Label for="allow-password" class="cursor-pointer">{m.ssh_allow_password()}</Label>
				<p class="text-xs text-muted-foreground">{m.ssh_allow_password_hint()}</p>
			</div>
			<Switch id="allow-password" bind:checked={params.allowPassword} />
		</div>
	</div>

	<!-- Config preview -->
	<div class="rounded-lg bg-muted p-3 text-sm">
		<p class="font-medium mb-1">{m.ssh_preview_title()}</p>
		<pre class="text-xs bg-background rounded p-2 overflow-x-auto"><code>{configPreview}</code></pre>
	</div>
</div>
