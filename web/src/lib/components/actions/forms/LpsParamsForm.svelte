<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import * as m from '$lib/paraglide/messages';
	import type { LpsFormState } from './types';
	import UserPicker from './UserPicker.svelte';

	interface Props {
		params: LpsFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let {
		params = $bindable(),
		errors = {},
		onclearerror
	}: Props = $props();

	const COMPLEXITY_OPTIONS = [
		{ value: 'ALPHANUMERIC', label: () => m.lps_complexity_alphanumeric(), description: () => m.lps_complexity_alphanumeric_description() },
		{ value: 'COMPLEX', label: () => m.lps_complexity_complex(), description: () => m.lps_complexity_complex_description() }
	];

	function getComplexityLabel(value: string): string {
		return COMPLEXITY_OPTIONS.find((c) => c.value === value)?.label() ?? value;
	}
</script>

<div class="space-y-4">
	<!-- Usernames -->
	<UserPicker
		bind:usernames={params.usernames}
		{errors}
		errorField="usernames"
		{onclearerror}
		label={m.lps_usernames()}
		addLabel={m.lps_usernames_add()}
		placeholder={m.lps_usernames_placeholder()}
		description={m.lps_usernames_description()}
	/>

	<!-- Password Length -->
	<div class="space-y-1.5">
		<Label>{m.lps_password_length()}</Label>
		<Input
			type="number"
			min={8}
			max={128}
			bind:value={params.passwordLength}
			oninput={() => onclearerror?.('passwordLength')}
		/>
		<FieldError error={errors.passwordLength} />
		<p class="text-xs text-muted-foreground">{m.lps_password_length_description()}</p>
	</div>

	<!-- Complexity -->
	<div class="space-y-1.5">
		<Label>{m.lps_complexity()}</Label>
		<Select.Root
			type="single"
			value={params.complexity}
			onValueChange={(v) => {
				params.complexity = v;
				onclearerror?.('complexity');
			}}
		>
			<Select.Trigger class="w-full">
				{getComplexityLabel(params.complexity)}
			</Select.Trigger>
			<Select.Content>
				{#each COMPLEXITY_OPTIONS as option}
					<Select.Item value={option.value}>
						<div>
							<div class="font-medium">{option.label()}</div>
							<div class="text-xs text-muted-foreground">{option.description()}</div>
						</div>
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
		<FieldError error={errors.complexity} />
	</div>

	<!-- Rotation Interval -->
	<div class="space-y-1.5">
		<Label>{m.lps_rotation_interval()}</Label>
		<div class="flex items-center gap-2">
			<Input
				type="number"
				min={1}
				max={365}
				bind:value={params.rotationIntervalDays}
				oninput={() => onclearerror?.('rotationIntervalDays')}
				class="w-24"
			/>
			<span class="text-sm text-muted-foreground">{m.lps_days()}</span>
		</div>
		<FieldError error={errors.rotationIntervalDays} />
		<p class="text-xs text-muted-foreground">{m.lps_rotation_interval_description()}</p>
	</div>

	<!-- Grace Period -->
	<div class="space-y-1.5">
		<Label>{m.lps_grace_period()}</Label>
		<div class="flex items-center gap-2">
			<Input
				type="number"
				min={0}
				max={8760}
				bind:value={params.gracePeriodHours}
				oninput={() => onclearerror?.('gracePeriodHours')}
				class="w-24"
			/>
			<span class="text-sm text-muted-foreground">{m.lps_hours()}</span>
		</div>
		<FieldError error={errors.gracePeriodHours} />
		<p class="text-xs text-muted-foreground">{m.lps_grace_period_description()}</p>
	</div>

	<!-- Info box -->
	<div class="rounded-lg bg-muted p-3 text-sm space-y-1">
		<p class="font-medium">{m.lps_info_title()}</p>
		<p class="text-xs text-muted-foreground">{m.lps_info_description()}</p>
	</div>
</div>
