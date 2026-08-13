<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { AppFormState } from './types';

	interface Props {
		params: AppFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label for="appUrl">{m.actions_params_app_url()}</Label>
		<Input id="appUrl" placeholder="https://example.com/app.AppImage" bind:value={params.url} required aria-invalid={!!errors?.url} oninput={() => onclearerror?.('url')} />
		<FieldError error={errors?.url} />
	</div>
	<div class="space-y-2">
		<Label for="appChecksum">{m.actions_params_app_checksum()}</Label>
		<Input id="appChecksum" placeholder="abc123..." bind:value={params.checksumSha256} />
	</div>
	<div class="space-y-2">
		<Label for="appInstallPath">{m.actions_params_app_install_path()}</Label>
		<Input id="appInstallPath" placeholder="/opt/myapp" bind:value={params.installPath} />
	</div>
</div>
