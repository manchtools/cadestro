<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { ShellFormState } from './types';

	interface Props {
		params: ShellFormState;
		complianceOnly?: boolean;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), complianceOnly = false, errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
	{#if complianceOnly}
		<div class="space-y-2">
			<Label for="detectionScript">{m.actions_params_shell_script()}</Label>
			<Textarea
				id="detectionScript"
				placeholder="#!/bin/bash&#10;# Exit 0 = compliant, non-zero = non-compliant"
				bind:value={params.detectionScript}
				rows={8}
				class="font-mono text-sm"
				aria-invalid={!!errors?.detectionScript}
				oninput={() => onclearerror?.('detectionScript')}
			/>
			<FieldError error={errors?.detectionScript} />
		</div>
	{:else}
			<div class="space-y-2">
				<Label for="detectionScript">{m.actions_params_shell_detection_script()}</Label>
				<p class="text-xs text-muted-foreground">{m.actions_params_shell_detection_script_description()}</p>
				<Textarea
					id="detectionScript"
					placeholder="#!/bin/bash&#10;# Exit 0 = compliant, non-zero = needs remediation"
					bind:value={params.detectionScript}
					rows={6}
					class="font-mono text-sm"
					aria-invalid={!!errors?.detectionScript}
					oninput={() => onclearerror?.('detectionScript')}
				/>
				<FieldError error={errors?.detectionScript} />
			</div>
		<div class="space-y-2">
			<Label for="script">{m.actions_params_shell_script()}</Label>
			{#if params.detectionScript}
				<p class="text-xs text-muted-foreground">{m.actions_params_shell_remediation_description()}</p>
			{/if}
			<Textarea
				id="script"
				placeholder="#!/bin/bash&#10;echo 'Hello World'"
				bind:value={params.script}
				rows={8}
				class="font-mono text-sm"
				aria-invalid={!!errors?.script}
				oninput={() => onclearerror?.('script')}
			/>
			<FieldError error={errors?.script} />
		</div>
	{/if}
	<div class="space-y-2">
		<Label for="interpreter">{m.actions_params_shell_interpreter()}</Label>
		<Input
			id="interpreter"
			placeholder="/bin/bash"
			bind:value={params.interpreter}
			aria-invalid={!!errors?.interpreter}
			oninput={() => onclearerror?.('interpreter')}
		/>
		<FieldError error={errors?.interpreter} />
	</div>
	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="runAsRoot">{m.actions_params_shell_run_as_root()}</Label>
			<p class="text-xs text-muted-foreground">{m.actions_params_shell_run_as_root_description()}</p>
		</div>
		<Switch id="runAsRoot" bind:checked={params.runAsRoot} />
	</div>
</div>
