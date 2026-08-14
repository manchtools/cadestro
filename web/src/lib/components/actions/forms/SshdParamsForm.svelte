<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import * as m from '$lib/paraglide/messages';
	import { Plus, Trash2 } from '@lucide/svelte';
	import type { SshdFormState, SshdDirectiveFormState } from './types';

	interface Props {
		params: SshdFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let {
		params = $bindable(),
		errors = {},
		onclearerror
	}: Props = $props();

	// Known sshd_config directives organized by category
	const KNOWN_DIRECTIVES: Record<string, Array<{ key: string; defaultValue: string }>> = {
		security: [
			{ key: 'PermitRootLogin', defaultValue: 'no' },
			{ key: 'PasswordAuthentication', defaultValue: 'no' },
			{ key: 'PubkeyAuthentication', defaultValue: 'yes' },
			{ key: 'PermitEmptyPasswords', defaultValue: 'no' },
			{ key: 'ChallengeResponseAuthentication', defaultValue: 'no' },
			{ key: 'KbdInteractiveAuthentication', defaultValue: 'no' },
			{ key: 'UsePAM', defaultValue: 'yes' },
			{ key: 'StrictModes', defaultValue: 'yes' }
		],
		network: [
			{ key: 'Port', defaultValue: '22' },
			{ key: 'ListenAddress', defaultValue: '0.0.0.0' },
			{ key: 'AddressFamily', defaultValue: 'any' },
			{ key: 'TCPKeepAlive', defaultValue: 'yes' }
		],
		limits: [
			{ key: 'MaxAuthTries', defaultValue: '3' },
			{ key: 'MaxSessions', defaultValue: '10' },
			{ key: 'LoginGraceTime', defaultValue: '60' },
			{ key: 'ClientAliveInterval', defaultValue: '300' },
			{ key: 'ClientAliveCountMax', defaultValue: '3' },
			{ key: 'MaxStartups', defaultValue: '10:30:60' }
		],
		features: [
			{ key: 'X11Forwarding', defaultValue: 'no' },
			{ key: 'AllowTcpForwarding', defaultValue: 'no' },
			{ key: 'AllowAgentForwarding', defaultValue: 'no' },
			{ key: 'GatewayPorts', defaultValue: 'no' },
			{ key: 'PermitTunnel', defaultValue: 'no' },
			{ key: 'PrintMotd', defaultValue: 'yes' },
			{ key: 'PrintLastLog', defaultValue: 'yes' }
		],
		access: [
			{ key: 'AllowUsers', defaultValue: '' },
			{ key: 'AllowGroups', defaultValue: '' },
			{ key: 'DenyUsers', defaultValue: '' },
			{ key: 'DenyGroups', defaultValue: '' }
		],
		logging: [
			{ key: 'LogLevel', defaultValue: 'INFO' },
			{ key: 'SyslogFacility', defaultValue: 'AUTH' }
		]
	};

	const CATEGORY_LABELS: Record<string, () => string> = {
		security: m.sshd_category_security,
		network: m.sshd_category_network,
		limits: m.sshd_category_limits,
		features: m.sshd_category_features,
		access: m.sshd_category_access,
		logging: m.sshd_category_logging
	};

	let selectedCategory = $state<string>('');
	let selectedDirective = $state<string>('');
	let customKey = $state('');
	let customValue = $state('');

	// Available directives for selected category (exclude already added ones)
	const availableDirectives = $derived.by(() => {
		if (!selectedCategory || !KNOWN_DIRECTIVES[selectedCategory]) return [];
		const existingKeys = new Set(params.directives.map((d) => d.key));
		return KNOWN_DIRECTIVES[selectedCategory].filter((d) => !existingKeys.has(d.key));
	});

	function addKnownDirective() {
		if (!selectedCategory || !selectedDirective) return;
		const directive = KNOWN_DIRECTIVES[selectedCategory]?.find(
			(d) => d.key === selectedDirective
		);
		if (directive) {
			params.directives = [
				...params.directives,
				{ key: directive.key, value: directive.defaultValue }
			];
			selectedDirective = '';
			onclearerror?.('directives');
		}
	}

	function addCustomDirective() {
		if (!customKey.trim()) return;
		params.directives = [
			...params.directives,
			{ key: customKey.trim(), value: customValue.trim() }
		];
		customKey = '';
		customValue = '';
		onclearerror?.('directives');
	}

	function removeDirective(index: number) {
		params.directives = params.directives.filter((_, i) => i !== index);
	}

	function updateDirectiveValue(index: number, value: string) {
		params.directives = params.directives.map((d, i) =>
			i === index ? { ...d, value } : d
		);
	}

	// Config preview
	const configPreview = $derived.by(() => {
		if (params.directives.length === 0) return '# No directives configured';
		const lines = ['# Managed by Cadestro - do not edit manually'];
		for (const d of params.directives) {
			lines.push(`${d.key || '<key>'} ${d.value || '<value>'}`);
		}
		return lines.join('\n');
	});
</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label>{m.sshd_directives()}</Label>

		<!-- Add known directive -->
		<div class="flex gap-2 items-end">
			<div class="flex-1">
				<Select.Root
					type="single"
					value={selectedCategory}
					onValueChange={(v) => {
						selectedCategory = v;
						selectedDirective = '';
					}}
				>
					<Select.Trigger class="w-full">
						{selectedCategory
							? CATEGORY_LABELS[selectedCategory]?.()
							: m.sshd_category_security()}
					</Select.Trigger>
					<Select.Content>
						{#each Object.entries(CATEGORY_LABELS) as [key, labelFn]}
							<Select.Item value={key}>{labelFn()}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<div class="flex-1">
				<Select.Root
					type="single"
					value={selectedDirective}
					onValueChange={(v) => (selectedDirective = v)}
				>
					<Select.Trigger class="w-full" disabled={availableDirectives.length === 0}>
						{selectedDirective || m.sshd_directive_key()}
					</Select.Trigger>
					<Select.Content>
						{#each availableDirectives as directive}
							<Select.Item value={directive.key}>
								{directive.key}
							</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<Button
				type="button"
				variant="outline"
				size="sm"
				onclick={addKnownDirective}
				disabled={!selectedDirective}
			>
				<Plus class="h-4 w-4 mr-1" />
				{m.sshd_add_directive()}
			</Button>
		</div>

		<!-- Add custom directive -->
		<div class="flex gap-2 items-end">
			<Input
				placeholder={m.sshd_directive_key()}
				bind:value={customKey}
				class="flex-1"
			/>
			<Input
				placeholder={m.sshd_directive_value()}
				bind:value={customValue}
				class="flex-1"
			/>
			<Button
				type="button"
				variant="outline"
				size="sm"
				onclick={addCustomDirective}
				disabled={!customKey.trim()}
			>
				<Plus class="h-4 w-4 mr-1" />
				{m.sshd_add_custom()}
			</Button>
		</div>

		<FieldError error={errors.directives} />
	</div>

	<!-- Directive list -->
	{#if params.directives.length > 0}
		<div class="space-y-2">
			{#each params.directives as directive, index}
				<div class="flex gap-2 items-center rounded-lg border p-2">
					<code class="text-sm font-medium min-w-[200px]">{directive.key}</code>
					<Input
						value={directive.value}
						oninput={(e) => updateDirectiveValue(index, e.currentTarget.value)}
						class="flex-1"
						placeholder={m.sshd_directive_value()}
					/>
					<Button
						type="button"
						variant="ghost"
						size="icon"
						onclick={() => removeDirective(index)}
						class="shrink-0 text-destructive hover:text-destructive"
					>
						<Trash2 class="h-4 w-4" />
					</Button>
				</div>
			{/each}
		</div>
	{:else}
		<p class="text-sm text-muted-foreground text-center py-4 border rounded-lg">
			{m.sshd_no_directives()}
		</p>
	{/if}

	<!-- Config preview -->
	<div class="rounded-lg bg-muted p-3 text-sm">
		<p class="font-medium mb-1">{m.sshd_preview_title()}</p>
		<pre class="text-xs bg-background rounded p-2 overflow-x-auto"><code>{configPreview}</code></pre>
	</div>
</div>
