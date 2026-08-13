<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import { Eye, EyeOff } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { EncryptionFormState } from './types';

	interface Props {
		params: EncryptionFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let {
		params = $bindable(),
		errors = {},
		onclearerror
	}: Props = $props();

	let showPresharedKey = $state(false);

	const DEVICE_BOUND_KEY_OPTIONS = [
		{ value: 'NONE', label: () => m.luks_device_bound_key_none(), description: () => m.luks_device_bound_key_none_description() },
		{ value: 'TPM', label: () => m.luks_device_bound_key_tpm(), description: () => m.luks_device_bound_key_tpm_description() },
		{ value: 'USER_PASSPHRASE', label: () => m.luks_device_bound_key_user_passphrase(), description: () => m.luks_device_bound_key_user_passphrase_description() }
	];

	const COMPLEXITY_OPTIONS = [
		{ value: 'ALPHANUMERIC', label: () => m.lps_complexity_alphanumeric(), description: () => m.lps_complexity_alphanumeric_description() },
		{ value: 'COMPLEX', label: () => m.lps_complexity_complex(), description: () => m.lps_complexity_complex_description() }
	];

	function getDeviceBoundKeyLabel(value: string): string {
		return DEVICE_BOUND_KEY_OPTIONS.find((o) => o.value === value)?.label() ?? value;
	}

	function getComplexityLabel(value: string): string {
		return COMPLEXITY_OPTIONS.find((c) => c.value === value)?.label() ?? value;
	}

	const showUserPassphraseOptions = $derived(params.deviceBoundKeyType === 'USER_PASSPHRASE');
</script>

<div class="space-y-4">
	<!-- Pre-shared Key -->
	<div class="space-y-1.5">
		<Label>{m.luks_preshared_key()}</Label>
		<div class="flex gap-2">
			<Input
				type={showPresharedKey ? 'text' : 'password'}
				placeholder={m.luks_preshared_key_placeholder()}
				bind:value={params.presharedKey}
				oninput={() => onclearerror?.('presharedKey')}
				class="flex-1 font-mono text-sm"
			/>
			<Button
				type="button"
				variant="outline"
				size="icon"
				onclick={() => (showPresharedKey = !showPresharedKey)}
			>
				{#if showPresharedKey}
					<EyeOff class="h-4 w-4" />
				{:else}
					<Eye class="h-4 w-4" />
				{/if}
			</Button>
		</div>
		<FieldError error={errors.presharedKey} />
		{#if params.presharedKeyConfigured}
			<p class="text-xs text-muted-foreground">{m.actions_secret_keep_existing()}</p>
		{/if}
		<p class="text-xs text-muted-foreground">{m.luks_preshared_key_description()}</p>
	</div>

	<!-- Rotation Interval -->
	<div class="space-y-1.5">
		<Label>{m.luks_rotation_interval()}</Label>
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
		<p class="text-xs text-muted-foreground">{m.luks_rotation_interval_description()}</p>
	</div>

	<!-- Minimum Words -->
	<div class="space-y-1.5">
		<Label>{m.luks_min_words()}</Label>
		<Input
			type="number"
			min={3}
			max={10}
			bind:value={params.minWords}
			oninput={() => onclearerror?.('minWords')}
			class="w-24"
		/>
		<FieldError error={errors.minWords} />
		<p class="text-xs text-muted-foreground">{m.luks_min_words_description()}</p>
	</div>

	<!-- Device-Bound Key Type -->
	<div class="space-y-1.5">
		<Label>{m.luks_device_bound_key_type()}</Label>
		<Select.Root
			type="single"
			value={params.deviceBoundKeyType}
			onValueChange={(v) => {
				params.deviceBoundKeyType = v;
				onclearerror?.('deviceBoundKeyType');
			}}
		>
			<Select.Trigger class="w-full">
				{getDeviceBoundKeyLabel(params.deviceBoundKeyType)}
			</Select.Trigger>
			<Select.Content>
				{#each DEVICE_BOUND_KEY_OPTIONS as option}
					<Select.Item value={option.value}>
						<div>
							<div class="font-medium">{option.label()}</div>
							<div class="text-xs text-muted-foreground">{option.description()}</div>
						</div>
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
		<FieldError error={errors.deviceBoundKeyType} />
	</div>

	<!-- User Passphrase Options (conditional) -->
	{#if showUserPassphraseOptions}
		<div class="space-y-4 rounded-lg border p-3">
			<p class="text-sm font-medium">{m.luks_user_passphrase_settings()}</p>

			<!-- Min Length -->
			<div class="space-y-1.5">
				<Label>{m.luks_user_passphrase_min_length()}</Label>
				<Input
					type="number"
					min={16}
					max={128}
					bind:value={params.userPassphraseMinLength}
					oninput={() => onclearerror?.('userPassphraseMinLength')}
					class="w-24"
				/>
				<FieldError error={errors.userPassphraseMinLength} />
			</div>

			<!-- Complexity -->
			<div class="space-y-1.5">
				<Label>{m.lps_complexity()}</Label>
				<Select.Root
					type="single"
					value={params.userPassphraseComplexity}
					onValueChange={(v) => {
						params.userPassphraseComplexity = v;
						onclearerror?.('userPassphraseComplexity');
					}}
				>
					<Select.Trigger class="w-full">
						{getComplexityLabel(params.userPassphraseComplexity)}
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
				<FieldError error={errors.userPassphraseComplexity} />
			</div>
		</div>
	{/if}

	<!-- Info box -->
	<div class="rounded-lg bg-muted p-3 text-sm space-y-1">
		<p class="font-medium">{m.luks_info_title()}</p>
		<p class="text-xs text-muted-foreground">{m.luks_info_description()}</p>
	</div>
</div>
