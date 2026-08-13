<script lang="ts">
	import * as m from '$lib/paraglide/messages';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import type { UpdateFormState } from './types';

	interface Props {
		params: UpdateFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="securityOnly">{m.actions_params_update_security_only()}</Label>
			<p class="text-xs text-muted-foreground">{m.actions_params_update_security_only_description()}</p>
		</div>
		<Switch id="securityOnly" bind:checked={params.securityOnly} />
	</div>
	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="autoremove">{m.actions_params_update_autoremove()}</Label>
			<p class="text-xs text-muted-foreground">{m.actions_params_update_autoremove_description()}</p>
		</div>
		<Switch id="autoremove" bind:checked={params.autoremove} />
	</div>
	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="rebootIfRequired">{m.actions_params_update_reboot()}</Label>
			<p class="text-xs text-muted-foreground">{m.actions_params_update_reboot_description()}</p>
		</div>
		<Switch id="rebootIfRequired" bind:checked={params.rebootIfRequired} />
	</div>
	<p class="text-xs text-muted-foreground mt-4">
		{m.actions_params_update_version_pinning_note()}
	</p>
</div>
