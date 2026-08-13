<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { DirectoryFormState } from './types';

	interface Props {
		params: DirectoryFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label for="dirPath">{m.actions_params_dir_path()}</Label>
		<Input
			id="dirPath"
			placeholder="/etc/myapp/data"
			bind:value={params.path}
			required
			aria-invalid={!!errors?.path}
			oninput={() => onclearerror?.('path')}
		/>
		<p class="text-xs text-muted-foreground">
			{m.actions_params_dir_path_description()}
		</p>
		<FieldError error={errors?.path} />
	</div>
	<div class="grid grid-cols-3 gap-4">
		<div class="space-y-2">
			<Label for="dirOwner">{m.actions_params_dir_owner()}</Label>
			<Input id="dirOwner" placeholder="root" bind:value={params.owner} />
		</div>
		<div class="space-y-2">
			<Label for="dirGroup">{m.actions_params_dir_group()}</Label>
			<Input id="dirGroup" placeholder="root" bind:value={params.group} />
		</div>
		<div class="space-y-2">
			<Label for="dirMode">{m.actions_params_dir_mode()}</Label>
			<Input id="dirMode" placeholder="0755" bind:value={params.mode} />
		</div>
	</div>
	<div class="flex items-center space-x-2">
		<Checkbox id="dirRecursive" bind:checked={params.recursive} />
		<Label for="dirRecursive" class="cursor-pointer">{m.actions_params_dir_recursive()}</Label>
	</div>
</div>
