<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { ServiceFormState } from './types';

	interface Props {
		params: ServiceFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();

	const systemdStates = $derived([
		{ value: 'RUNNING', label: m.actions_params_systemd_state_running() },
		{ value: 'STOPPED', label: m.actions_params_systemd_state_stopped() },
		{ value: 'RESTARTED', label: m.actions_params_systemd_state_restarted() }
	]);

</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label for="unitName">{m.actions_params_systemd_unit_name()}</Label>
		<Input
			id="unitName"
			placeholder="e.g., nginx.service"
			bind:value={params.unitName}
			required
			aria-invalid={!!errors?.unitName}
			oninput={() => onclearerror?.('unitName')}
		/>
		<FieldError error={errors?.unitName} />
	</div>
	<div class="space-y-2">
		<Label>{m.actions_params_systemd_service_state()}</Label>
		<Select.Root type="single" bind:value={params.desiredState}>
			<Select.Trigger>
				{systemdStates.find((s) => s.value === params.desiredState)?.label ?? m.actions_params_systemd_select_state()}
			</Select.Trigger>
			<Select.Content>
				{#each systemdStates as state}
					<Select.Item value={state.value}>{state.label}</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
		<FieldError error={errors?.desiredState} />
	</div>
	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="enableUnit">{m.actions_params_systemd_enable_boot()}</Label>
			<p class="text-xs text-muted-foreground">{m.actions_params_systemd_enable_boot_description()}</p>
		</div>
		<Switch id="enableUnit" bind:checked={params.enable} />
	</div>
</div>
