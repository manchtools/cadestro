<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { ShellActionParams as ShellFormState } from '$contract/cadestro/v1/actions_pb';

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
 <div class="space-y-2"><Label for="workingDirectory">Working directory</Label><Input id="workingDirectory" bind:value={params.workingDirectory} placeholder="/var/lib/cadestro" /></div>
 <div class="space-y-2"><Label for="environment">Environment</Label><Textarea id="environment" rows={3} value={Object.entries(params.environment).map(([key, value]) => `${key}=${value}`).join('\n')} oninput={(event) => { params.environment = Object.fromEntries(event.currentTarget.value.split('\n').filter(Boolean).map((line) => { const at = line.indexOf('='); return at < 0 ? [line, ''] : [line.slice(0, at), line.slice(at + 1)]; })); }} placeholder="KEY=value" /></div>
 <div class="flex items-center justify-between"><Label for="isCompliance">Compliance check</Label><Switch id="isCompliance" bind:checked={params.isCompliance} /></div>
</div>
