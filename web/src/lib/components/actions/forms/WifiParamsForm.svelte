<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import { Switch } from '$lib/components/ui/switch';
	import * as m from '$lib/paraglide/messages';
	import type { WifiFormState } from './types';

	interface Props {
		params: WifiFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let {
		params = $bindable(),
		errors = {},
		onclearerror
	}: Props = $props();

	const AUTH_TYPE_OPTIONS = [
		{ value: 'PSK', label: () => m.wifi_auth_type_psk(), description: () => m.wifi_auth_type_psk_description() },
		{ value: 'EAP_TLS', label: () => m.wifi_auth_type_eap_tls(), description: () => m.wifi_auth_type_eap_tls_description() }
	];

	function getAuthTypeLabel(value: string): string {
		return AUTH_TYPE_OPTIONS.find((o) => o.value === value)?.label() ?? value;
	}

	const showPskFields = $derived(params.authType === 'PSK');
	const showEapTlsFields = $derived(params.authType === 'EAP_TLS');
</script>

<div class="space-y-4">

	<div class="space-y-1.5">
		<Label>{m.wifi_ssid()}</Label>
		<Input
			placeholder={m.wifi_ssid_placeholder()}
			bind:value={params.ssid}
			oninput={() => onclearerror?.('ssid')}
		/>
		<FieldError error={errors.ssid} />
	</div>

	<div class="space-y-1.5">
		<Label>{m.wifi_auth_type()}</Label>
		<Select.Root
			type="single"
			value={params.authType}
			onValueChange={(v) => {
				params.authType = v;
				onclearerror?.('authType');
			}}
		>
			<Select.Trigger class="w-full">
				{params.authType ? getAuthTypeLabel(params.authType) : m.wifi_auth_type_select()}
			</Select.Trigger>
			<Select.Content>
				{#each AUTH_TYPE_OPTIONS as option}
					<Select.Item value={option.value}>
						<div>
							<div class="font-medium">{option.label()}</div>
							<div class="text-xs text-muted-foreground">{option.description()}</div>
						</div>
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
		<FieldError error={errors.authType} />
	</div>

	{#if showPskFields}
		<div class="space-y-1.5">
			<Label>{m.wifi_psk()}</Label>
			<Input
				type="password"
				placeholder={m.wifi_psk_placeholder()}
				bind:value={params.psk}
				oninput={() => onclearerror?.('psk')}
			/>
			<FieldError error={errors.psk} />
			{#if params.pskConfigured}
				<p class="text-xs text-muted-foreground">{m.actions_secret_keep_existing()}</p>
			{/if}
		</div>
	{/if}

	{#if showEapTlsFields}
		<div class="space-y-4 rounded-lg border p-3">
			<p class="text-sm font-medium">{m.wifi_eap_tls_settings()}</p>

			<div class="space-y-1.5">
				<Label>{m.wifi_identity()}</Label>
				<Input
					placeholder={m.wifi_identity_placeholder()}
					bind:value={params.identity}
					oninput={() => onclearerror?.('identity')}
				/>
				<FieldError error={errors.identity} />
			</div>

			<div class="space-y-1.5">
				<Label>{m.wifi_ca_cert()}</Label>
				<Textarea
					placeholder={m.wifi_ca_cert_placeholder()}
					bind:value={params.caCert}
					rows={4}
					class="font-mono text-xs"
					oninput={() => onclearerror?.('caCert')}
				/>
				<FieldError error={errors.caCert} />
			</div>

			<div class="space-y-1.5">
				<Label>{m.wifi_client_cert()}</Label>
				<Textarea
					placeholder={m.wifi_client_cert_placeholder()}
					bind:value={params.clientCert}
					rows={4}
					class="font-mono text-xs"
					oninput={() => onclearerror?.('clientCert')}
				/>
				<FieldError error={errors.clientCert} />
			</div>

			<div class="space-y-1.5">
				<Label>{m.wifi_client_key()}</Label>
				<Textarea
					placeholder={m.wifi_client_key_placeholder()}
					bind:value={params.clientKey}
					rows={4}
					class="font-mono text-xs"
					oninput={() => onclearerror?.('clientKey')}
				/>
				<FieldError error={errors.clientKey} />
				{#if params.clientKeyConfigured}
					<p class="text-xs text-muted-foreground">{m.actions_secret_keep_existing()}</p>
				{/if}
			</div>
		</div>
	{/if}

	<div class="grid gap-3 sm:grid-cols-2">
		<div class="flex items-center justify-between gap-2 rounded-lg border p-3">
			<div>
				<Label>{m.wifi_auto_connect()}</Label>
				<p class="text-xs text-muted-foreground">{m.wifi_auto_connect_description()}</p>
			</div>
			<Switch bind:checked={params.autoConnect} />
		</div>

		<div class="flex items-center justify-between gap-2 rounded-lg border p-3">
			<div>
				<Label>{m.wifi_hidden()}</Label>
				<p class="text-xs text-muted-foreground">{m.wifi_hidden_description()}</p>
			</div>
			<Switch bind:checked={params.hidden} />
		</div>
	</div>

	<div class="space-y-1.5">
		<Label>{m.wifi_priority()}</Label>
		<Input
			type="number"
			min={-1}
			max={999}
			bind:value={params.priority}
			oninput={() => onclearerror?.('priority')}
			class="w-24"
		/>
		<FieldError error={errors.priority} />
		<p class="text-xs text-muted-foreground">{m.wifi_priority_description()}</p>
	</div>
</div>
