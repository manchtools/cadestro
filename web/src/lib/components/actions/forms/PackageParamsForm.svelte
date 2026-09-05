<script lang="ts">
	import { untrack } from 'svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { PackageActionParams as PackageFormState } from '$contract/cadestro/v1/actions_pb';

	interface Props {
		params: PackageFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
		<div class="space-y-1.5">
			<Label for="packageName">{m.actions_params_package_name()}</Label>
			<Input
				id="packageName"
				placeholder="e.g., firefox"
				bind:value={params.name}
				required
				aria-invalid={!!errors?.packageName}
				oninput={() => onclearerror?.('name')}
			/>
			<FieldError error={errors?.packageName} />
		</div>

	<div class="space-y-1.5 sm:max-w-64">
		<Label for="packageVersion">{m.actions_params_package_version()}</Label>
		<Input id="packageVersion" placeholder="e.g., 120.0" bind:value={params.version} />
	</div>
</div>
