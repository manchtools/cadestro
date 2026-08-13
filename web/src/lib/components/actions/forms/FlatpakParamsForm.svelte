<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { FlatpakFormState } from './types';

	interface Props {
		params: FlatpakFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label for="flatpakAppId">{m.actions_params_flatpak_app_id()}</Label>
		<Input
			id="flatpakAppId"
			placeholder="org.mozilla.firefox"
			bind:value={params.appId}
			required
			aria-invalid={!!errors?.appId}
			oninput={() => onclearerror?.('appId')}
		/>
		<p class="text-xs text-muted-foreground">
			{m.actions_params_flatpak_app_id_description()}
		</p>
		<FieldError error={errors?.appId} />
	</div>
	<div class="space-y-2">
		<Label for="flatpakRemote">{m.actions_params_flatpak_remote()}</Label>
		<Input id="flatpakRemote" placeholder="flathub" bind:value={params.remote} />
		<p class="text-xs text-muted-foreground">
			{m.actions_params_flatpak_remote_description()}
		</p>
	</div>
	<div class="flex items-center space-x-2">
		<Checkbox id="flatpakSystemWide" bind:checked={params.systemWide} />
		<Label for="flatpakSystemWide" class="cursor-pointer">{m.actions_params_flatpak_system_wide()}</Label>
	</div>
	<div class="flex items-center space-x-2">
		<Checkbox id="flatpakPin" bind:checked={params.pin} />
		<Label for="flatpakPin" class="cursor-pointer">{m.actions_params_flatpak_pin()}</Label>
	</div>
</div>
