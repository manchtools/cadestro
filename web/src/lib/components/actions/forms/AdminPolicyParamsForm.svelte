<script lang="ts">
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import * as m from '$lib/paraglide/messages';
	import type { AdminPolicyFormState } from './types';
	import UserPicker from './UserPicker.svelte';

	interface Props {
		params: AdminPolicyFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let {
		params = $bindable(),
		errors = {},
		onclearerror
	}: Props = $props();

	const ACCESS_LEVELS = [
		{ value: 'FULL', label: () => m.sudo_access_level_full(), description: () => m.sudo_access_level_full_description() },
		{ value: 'LIMITED', label: () => m.sudo_access_level_limited(), description: () => m.sudo_access_level_limited_description() },
		{ value: 'CUSTOM', label: () => m.sudo_access_level_custom(), description: () => m.sudo_access_level_custom_description() }
	];

	const BACKENDS = [
		{ value: 'SUDO', label: 'sudo', description: '/etc/sudoers.d/ — mainstream Linux default' },
		{ value: 'DOAS', label: 'doas', description: '/etc/doas.d/ — OpenBSD-style lightweight alternative' }
	];

	function getAccessLevelLabel(value: string): string {
		return ACCESS_LEVELS.find((l) => l.value === value)?.label() ?? value;
	}

	function getBackendLabel(value: string): string {
		return BACKENDS.find((b) => b.value === value)?.label ?? value;
	}

	const configPreview = $derived.by(() => {
		const sudoGroup = '%cadestro-sudo-{id}';
		const doasGroup = ':cadestro-sudo-{id}';
		const isDoas = params.backend === 'DOAS';
		const lines = ['# Managed by Cadestro - do not edit manually'];

		if (isDoas) {
			switch (params.accessLevel) {
				case 'FULL':
					lines.push('# Full admin access via doas (password required)');
					lines.push(`permit persist ${doasGroup} as root`);
					break;
				case 'LIMITED':

					lines.push('# Limited admin access via doas (password required)');
					lines.push(`permit persist ${doasGroup} as root cmd /usr/bin/apt`);
					lines.push(`permit persist ${doasGroup} as root cmd /usr/bin/systemctl`);
					lines.push(`permit persist ${doasGroup} as root cmd /usr/bin/journalctl`);
					lines.push(`permit persist ${doasGroup} as root cmd /usr/bin/ip`);
					lines.push(`permit persist ${doasGroup} as root cmd /usr/bin/nmcli`);
					lines.push(`permit persist ${doasGroup} as root cmd /usr/sbin/reboot`);
					lines.push(`permit persist ${doasGroup} as root cmd /usr/sbin/shutdown`);
					break;
				case 'CUSTOM':
					if (params.customConfig.trim()) {
						lines.push('# Custom doas rules');

						lines.push(
							params.customConfig
								.replace(/%\{group\}/g, doasGroup)
								.replace(/\{group\}/g, doasGroup.slice(1))
						);
					} else {
						lines.push('# <custom configuration>');
					}
					break;
			}
			return lines.join('\n');
		}

		switch (params.accessLevel) {
			case 'FULL':
				lines.push('# Full sudo access (password required)');
				lines.push(`${sudoGroup} ALL=(ALL:ALL) ALL`);
				break;
			case 'LIMITED':
				lines.push('# Limited sudo access - system management only (password required)');
				lines.push(`${sudoGroup} ALL=(ALL) /usr/bin/apt, /usr/bin/apt-get, ...`);
				lines.push(`${sudoGroup} ALL=(ALL) /usr/bin/systemctl, /usr/bin/journalctl`);
				lines.push(`${sudoGroup} ALL=(ALL) /usr/bin/ip, /usr/bin/nmcli, /usr/bin/docker, ...`);
				lines.push(`${sudoGroup} ALL=(ALL) /usr/sbin/reboot, /usr/sbin/shutdown`);
				break;
			case 'CUSTOM':
				if (params.customConfig.trim()) {
					lines.push('# Custom sudo access');
					lines.push(params.customConfig.replace(/\{group\}/g, sudoGroup.slice(1)));
				} else {
					lines.push('# <custom configuration>');
				}
				break;
		}

		return lines.join('\n');
	});
</script>

<div class="space-y-4">

	<div class="space-y-1.5">
		<Label>{m.sudo_privilege_backend()}</Label>
		<Select.Root
			type="single"
			value={params.backend}
			onValueChange={(v) => {
				params.backend = v;
				onclearerror?.('backend');
			}}
		>
			<Select.Trigger class="w-full">
				{getBackendLabel(params.backend)}
			</Select.Trigger>
			<Select.Content>
				{#each BACKENDS as backend}
					<Select.Item value={backend.value}>
						<div>
							<div class="font-medium">{backend.label}</div>
							<div class="text-xs text-muted-foreground">{backend.description}</div>
						</div>
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
		<FieldError error={errors.backend} />
	</div>

	<div class="space-y-1.5">
		<Label>{m.sudo_access_level()}</Label>
		<Select.Root
			type="single"
			value={params.accessLevel}
			onValueChange={(v) => {
				params.accessLevel = v;
				onclearerror?.('accessLevel');
			}}
		>
			<Select.Trigger class="w-full">
				{getAccessLevelLabel(params.accessLevel)}
			</Select.Trigger>
			<Select.Content>
				{#each ACCESS_LEVELS as level}
					<Select.Item value={level.value}>
						<div>
							<div class="font-medium">{level.label()}</div>
							<div class="text-xs text-muted-foreground">{level.description()}</div>
						</div>
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
		<FieldError error={errors.accessLevel} />
	</div>

	<UserPicker
		bind:usernames={params.users}
		{errors}
		errorField="users"
		{onclearerror}
		label={m.sudo_users()}
		addLabel={m.sudo_users_add()}
		placeholder={m.sudo_users_add_placeholder()}
		description={m.sudo_users_description()}
	/>

	{#if params.accessLevel === 'CUSTOM'}
		<div class="space-y-1.5">
			<Label>{m.sudo_custom_config()}</Label>
			<Textarea
				placeholder={m.sudo_custom_config_placeholder({ group: '{group}' })}
				bind:value={params.customConfig}
				oninput={() => onclearerror?.('customConfig')}
				rows={8}
				class="font-mono text-sm"
			/>
			<FieldError error={errors.customConfig} />
			<p class="text-xs text-muted-foreground">{m.sudo_custom_config_description({ group: '{group}' })}</p>
		</div>
	{/if}

	<div class="rounded-lg bg-muted p-3 text-sm">
		<p class="font-medium mb-1">
			{params.backend === 'DOAS' ? '/etc/doas.d/<id>.conf preview' : m.sudo_preview_title()}
		</p>
		<pre class="text-xs bg-background rounded p-2 overflow-x-auto"><code>{configPreview}</code></pre>
	</div>
</div>
