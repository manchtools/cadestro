<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Switch } from '$lib/components/ui/switch';
	import FieldError from '$lib/components/ui/field-error/field-error.svelte';
	import * as m from '$lib/paraglide/messages';
	import type { FileFormState } from './types';

	interface Props {
		params: FileFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label for="filePath">{m.actions_params_file_path()}</Label>
		<Input
			id="filePath"
			placeholder="/etc/myapp/config.conf"
			bind:value={params.path}
			required
			aria-invalid={!!errors?.path}
			oninput={() => onclearerror?.('path')}
		/>
		<FieldError error={errors?.path} />
	</div>
	<div class="space-y-2">
		<Label for="fileContent">{m.actions_params_file_content()}</Label>
		<Textarea
			id="fileContent"
			placeholder={m.actions_params_file_content_placeholder()}
			bind:value={params.content}
			rows={6}
			class="font-mono text-sm"
		/>
	</div>
	<div class="grid grid-cols-3 gap-4">
		<div class="space-y-2">
			<Label for="fileOwner">{m.actions_params_file_owner()}</Label>
			<Input id="fileOwner" placeholder="root" bind:value={params.owner} />
		</div>
		<div class="space-y-2">
			<Label for="fileGroup">{m.actions_params_file_group()}</Label>
			<Input id="fileGroup" placeholder="root" bind:value={params.group} />
		</div>
		<div class="space-y-2">
			<Label for="fileMode">{m.actions_params_file_mode()}</Label>
			<Input id="fileMode" placeholder="0644" bind:value={params.mode} />
		</div>
	</div>

	<div class="flex items-center justify-between rounded-lg border p-3">
		<div class="space-y-0.5">
			<Label for="managedBlock" class="cursor-pointer">{m.actions_params_file_managed_block()}</Label>
			<p class="text-xs text-muted-foreground">{m.actions_params_file_managed_block_hint()}</p>
		</div>
		<Switch id="managedBlock" bind:checked={params.managedBlock} />
	</div>
</div>
