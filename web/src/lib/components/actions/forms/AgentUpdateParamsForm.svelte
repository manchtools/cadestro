<script lang="ts">
	import * as m from '$lib/paraglide/messages';
	import { Label } from '$lib/components/ui/label';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import type { AgentUpdateFormState } from './types';

	interface Props {
		params: AgentUpdateFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();

	// A form state built before allow_redirect existed (an older saved draft, or
	// any path that didn't run defaultAgentUpdateForm) lacks the field. Coerce it
	// to the intended default so the Switch never binds undefined (Svelte rejects
	// bind:checked={undefined}) and the proto carries the operator's choice.
	if (params.allowRedirect === undefined) {
		params.allowRedirect = true;
	}
</script>

<div class="space-y-6">
	<p class="text-sm text-muted-foreground">{m.agent_update_https_note()}</p>

	<div class="space-y-4">
		<h4 class="font-medium text-sm">{m.agent_update_amd64_section()}</h4>
		<div class="space-y-2">
			<Label for="amd64BinaryUrl">{m.agent_update_amd64_binary_url()}</Label>
			<Input
				id="amd64BinaryUrl"
				type="url"
				bind:value={params.amd64BinaryUrl}
				placeholder={m.agent_update_amd64_binary_url_placeholder()}
				oninput={() => onclearerror?.('amd64BinaryUrl')}
			/>
			<FieldError error={errors?.amd64BinaryUrl} />
		</div>
		<div class="space-y-2">
			<Label for="amd64ChecksumUrl">{m.agent_update_amd64_checksum_url()}</Label>
			<Input
				id="amd64ChecksumUrl"
				type="url"
				bind:value={params.amd64ChecksumUrl}
				placeholder={m.agent_update_amd64_checksum_url_placeholder()}
				oninput={() => onclearerror?.('amd64ChecksumUrl')}
			/>
			<FieldError error={errors?.amd64ChecksumUrl} />
		</div>
	</div>

	<div class="space-y-4">
		<h4 class="font-medium text-sm">{m.agent_update_arm64_section()}</h4>
		<div class="space-y-2">
			<Label for="arm64BinaryUrl">{m.agent_update_arm64_binary_url()}</Label>
			<Input
				id="arm64BinaryUrl"
				type="url"
				bind:value={params.arm64BinaryUrl}
				placeholder={m.agent_update_arm64_binary_url_placeholder()}
				oninput={() => onclearerror?.('arm64BinaryUrl')}
			/>
			<FieldError error={errors?.arm64BinaryUrl} />
		</div>
		<div class="space-y-2">
			<Label for="arm64ChecksumUrl">{m.agent_update_arm64_checksum_url()}</Label>
			<Input
				id="arm64ChecksumUrl"
				type="url"
				bind:value={params.arm64ChecksumUrl}
				placeholder={m.agent_update_arm64_checksum_url_placeholder()}
				oninput={() => onclearerror?.('arm64ChecksumUrl')}
			/>
			<FieldError error={errors?.arm64ChecksumUrl} />
		</div>
	</div>

	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="allowRedirect">{m.agent_update_allow_redirect()}</Label>
			<p class="text-xs text-muted-foreground">{m.agent_update_allow_redirect_description()}</p>
		</div>
		<Switch id="allowRedirect" bind:checked={params.allowRedirect} />
	</div>
</div>
