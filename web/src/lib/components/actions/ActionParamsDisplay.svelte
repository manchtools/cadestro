<script lang="ts">
	import type { ManagedAction } from '$sdk/powermanage/v1/control_pb';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		params: ManagedAction['params'];
		class?: string;
	}

	let { params, class: className }: Props = $props();

	function formatSummary(params: ManagedAction['params']): string {
		if (!params || params.case === undefined) return m.actions_display_no_params();

		switch (params.case) {
			case 'package': {
				const hasManagerNames = params.value.aptName || params.value.dnfName ||
					params.value.pacmanName || params.value.zypperName;
				const displayName = params.value.name || (hasManagerNames ? m.actions_display_per_manager() : 'none');
				return `${m.actions_display_package()}: ${displayName}${params.value.version ? ` (${params.value.version})` : ''}`;
			}
			case 'shell':
				if (params.value.isCompliance) {
					return `${m.actions_params_shell_detection_script()} (${params.value.interpreter || '/bin/bash'})`;
				}
				return `${m.actions_display_script()} (${params.value.interpreter || '/bin/bash'})${params.value.runAsRoot ? ' [root]' : ''}`;
			case 'service':
				return `${m.actions_display_unit_name()}: ${params.value.unitName}`;
			case 'file':
				return `${m.actions_display_path()}: ${params.value.path}`;
			case 'app':
				return `${m.actions_display_url()}: ${params.value.url}`;
			case 'update':
				return params.value.securityOnly ? m.actions_display_system_update_security() : m.actions_display_system_update();
			case 'repository': {
				const configured = [];
				if (params.value.apt && !params.value.apt.disabled) configured.push('APT');
				if (params.value.dnf && !params.value.dnf.disabled) configured.push('DNF');
				if (params.value.pacman && !params.value.pacman.disabled) configured.push('Pacman');
				if (params.value.zypper && !params.value.zypper.disabled) configured.push('Zypper');
				return `Repository: ${params.value.name} (${configured.join(', ')})`;
			}
			case 'flatpak':
				return `Flatpak: ${params.value.appId}${params.value.remote ? ` (${params.value.remote})` : ''}`;
			case 'directory':
				return `${m.actions_type_directory()}: ${params.value.path}`;
			case 'user':
				return `${m.actions_type_user()}: ${params.value.username}${params.value.disabled ? ' [disabled]' : ''}`;
			case 'ssh': {
				const sshUsers = params.value.users;
				return `SSH: ${sshUsers.length} user${sshUsers.length !== 1 ? 's' : ''} (${sshUsers.join(', ')})`;
			}
			case 'sshd':
				return `${m.actions_type_sshd()}: ${params.value.directives.length} directive${params.value.directives.length !== 1 ? 's' : ''} (priority ${params.value.priority})`;
			case 'adminPolicy': {
				const levelLabel = params.value.accessLevel === 1 ? 'Full' : params.value.accessLevel === 2 ? 'Limited' : 'Custom';
				return `${m.actions_type_sudo()}: ${levelLabel} (${params.value.users.length} user${params.value.users.length !== 1 ? 's' : ''})`;
			}
			case 'lps':
				return `${m.actions_type_lps()}: ${params.value.usernames.join(', ')} (${params.value.rotationIntervalDays}d)`;
			case 'encryption': {
				const keyType = params.value.deviceBoundKeyType === 1 ? 'TPM' : params.value.deviceBoundKeyType === 2 ? 'User' : 'None';
				return `${m.actions_type_luks()}: ${params.value.minWords} words, ${params.value.rotationIntervalDays}d, slot 7: ${keyType}`;
			}
			case 'group':
				return `${m.actions_type_group()}: ${params.value.name} (${params.value.members.length} member${params.value.members.length !== 1 ? 's' : ''})`;
			case 'wifi': {
				const authLabel = params.value.authType === 1 ? 'PSK' : params.value.authType === 2 ? 'EAP-TLS' : '?';
				return `${m.actions_type_wifi()}: ${params.value.ssid} (${authLabel})`;
			}
			case 'agentUpdate': {
				const archs = [params.value.amd64 ? 'amd64' : '', params.value.arm64 ? 'arm64' : ''].filter(Boolean).join(', ');
				return `${m.actions_type_agent_update()}: ${archs}`;
			}
			default:
				return m.actions_display_unknown_params();
		}
	}

	function yesNo(value: boolean): string {
		return value ? m.actions_display_yes() : m.actions_display_no();
	}
</script>

{#if params && params.case !== undefined}
	<div class="space-y-3 {className}">
		<p class="text-sm font-medium">{formatSummary(params)}</p>

		{#if params.case === 'package'}
			{@const hasManagerNames = params.value.aptName || params.value.dnfName || params.value.pacmanName || params.value.zypperName}
			<div class="text-sm space-y-1">
				{#if params.value.name}
					<p><span class="text-muted-foreground">{m.actions_display_name()}:</span> {params.value.name}</p>
				{/if}
				{#if hasManagerNames}
					<div class="space-y-1 rounded-md border p-2 mt-2">
						<p class="text-xs font-medium text-muted-foreground">{m.actions_display_per_manager_names()}:</p>
						{#if params.value.aptName}
							<p><span class="text-muted-foreground">APT:</span> {params.value.aptName}</p>
						{/if}
						{#if params.value.dnfName}
							<p><span class="text-muted-foreground">DNF:</span> {params.value.dnfName}</p>
						{/if}
						{#if params.value.pacmanName}
							<p><span class="text-muted-foreground">Pacman:</span> {params.value.pacmanName}</p>
						{/if}
						{#if params.value.zypperName}
							<p><span class="text-muted-foreground">Zypper:</span> {params.value.zypperName}</p>
						{/if}
					</div>
				{/if}
				{#if params.value.version}
					<p><span class="text-muted-foreground">{m.actions_display_version()}:</span> {params.value.version}</p>
				{/if}
				<p><span class="text-muted-foreground">{m.actions_display_allow_downgrade()}:</span> {yesNo(params.value.allowDowngrade)}</p>
			</div>
	{:else if params.case === 'shell'}
		<div class="text-sm space-y-1">
			<p><span class="text-muted-foreground">{m.actions_display_interpreter()}:</span> {params.value.interpreter || '/bin/bash'}</p>
			{#if !params.value.isCompliance}
				<p><span class="text-muted-foreground">{m.actions_display_run_as_root()}:</span> {yesNo(params.value.runAsRoot)}</p>
			{/if}
			{#if params.value.isCompliance}
				<div>
					<p class="text-muted-foreground mb-1">{m.actions_params_shell_detection_script()}:</p>
					<pre class="bg-muted p-2 rounded text-xs overflow-x-auto max-h-48">{params.value.detectionScript}</pre>
				</div>
			{:else}
				{#if params.value.detectionScript}
					<div>
						<p class="text-muted-foreground mb-1">{m.actions_params_shell_detection_script()}:</p>
						<pre class="bg-muted p-2 rounded text-xs overflow-x-auto max-h-48">{params.value.detectionScript}</pre>
					</div>
				{/if}
				<div>
					<p class="text-muted-foreground mb-1">{params.value.detectionScript ? m.actions_params_shell_remediation_description() : m.actions_display_script()}:</p>
					<pre class="bg-muted p-2 rounded text-xs overflow-x-auto max-h-48">{params.value.script}</pre>
				</div>
			{/if}
		</div>
		{:else if params.case === 'service'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.actions_display_unit_name()}:</span> {params.value.unitName}</p>
				<p><span class="text-muted-foreground">{m.actions_display_state()}:</span> {params.value.desiredState === 2 ? m.actions_display_state_stopped() : params.value.desiredState === 3 ? m.actions_display_state_restarted() : m.actions_display_state_running()}</p>
				<p><span class="text-muted-foreground">{m.actions_display_enable_on_boot()}:</span> {yesNo(params.value.enable)}</p>
			</div>
		{:else if params.case === 'file'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.actions_display_path()}:</span> {params.value.path}</p>
				{#if params.value.owner}
					<p><span class="text-muted-foreground">{m.actions_display_owner()}:</span> {params.value.owner}</p>
				{/if}
				{#if params.value.group}
					<p><span class="text-muted-foreground">{m.actions_display_group()}:</span> {params.value.group}</p>
				{/if}
				{#if params.value.mode}
					<p><span class="text-muted-foreground">{m.actions_display_mode()}:</span> {params.value.mode}</p>
				{/if}
				{#if params.value.content}
					<div>
						<p class="text-muted-foreground mb-1">{m.actions_display_content()}:</p>
						<pre class="bg-muted p-2 rounded text-xs overflow-x-auto max-h-48">{params.value.content}</pre>
					</div>
				{/if}
			</div>
		{:else if params.case === 'app'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.actions_display_url()}:</span> {params.value.url}</p>
				{#if params.value.checksumSha256}
					<p><span class="text-muted-foreground">{m.actions_display_checksum()}:</span> <span class="font-mono text-xs">{params.value.checksumSha256}</span></p>
				{/if}
				{#if params.value.installPath}
					<p><span class="text-muted-foreground">{m.actions_display_install_path()}:</span> {params.value.installPath}</p>
				{/if}
			</div>
		{:else if params.case === 'update'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.actions_display_security_only()}:</span> {yesNo(params.value.securityOnly)}</p>
				<p><span class="text-muted-foreground">{m.actions_params_update_autoremove()}:</span> {yesNo(params.value.autoremove)}</p>
				<p><span class="text-muted-foreground">{m.actions_display_reboot_if_required()}:</span> {yesNo(params.value.rebootIfRequired)}</p>
			</div>
		{:else if params.case === 'repository'}
			<div class="text-sm space-y-3">
				<p><span class="text-muted-foreground">{m.actions_display_name()}:</span> {params.value.name}</p>

				{#if params.value.apt && !params.value.apt.disabled}
					<div class="rounded-md border p-2 space-y-1">
						<p class="text-xs font-medium text-muted-foreground">{m.actions_params_package_apt()}</p>
						<p><span class="text-muted-foreground">{m.actions_display_url()}:</span> {params.value.apt.url}</p>
						{#if params.value.apt.distribution}
							<p><span class="text-muted-foreground">{m.actions_display_distribution()}:</span> {params.value.apt.distribution}</p>
						{/if}
						{#if params.value.apt.components?.length}
							<p><span class="text-muted-foreground">{m.actions_display_components()}:</span> {params.value.apt.components.join(' ')}</p>
						{/if}
						{#if params.value.apt.gpgKeyUrl}
							<p><span class="text-muted-foreground">{m.actions_display_gpg_key_url()}:</span> {params.value.apt.gpgKeyUrl}</p>
						{/if}
						{#if params.value.apt.trusted}
							<p class="text-warn">{m.actions_display_trusted_no_gpg()}</p>
						{/if}
					</div>
				{/if}

				{#if params.value.dnf && !params.value.dnf.disabled}
					<div class="rounded-md border p-2 space-y-1">
						<p class="text-xs font-medium text-muted-foreground">{m.actions_params_package_dnf()}</p>
						<p><span class="text-muted-foreground">{m.actions_display_base_url()}:</span> {params.value.dnf.baseurl}</p>
						{#if params.value.dnf.description}
							<p><span class="text-muted-foreground">{m.actions_display_description()}:</span> {params.value.dnf.description}</p>
						{/if}
						<p><span class="text-muted-foreground">{m.actions_display_enabled()}:</span> {yesNo(params.value.dnf.enabled)}</p>
						<p><span class="text-muted-foreground">{m.actions_display_gpg_check()}:</span> {yesNo(params.value.dnf.gpgcheck)}</p>
						{#if params.value.dnf.gpgkey}
							<p><span class="text-muted-foreground">{m.actions_display_gpg_key()}:</span> {params.value.dnf.gpgkey}</p>
						{/if}
					</div>
				{/if}

				{#if params.value.pacman && !params.value.pacman.disabled}
					<div class="rounded-md border p-2 space-y-1">
						<p class="text-xs font-medium text-muted-foreground">{m.actions_params_package_pacman()}</p>
						<p><span class="text-muted-foreground">{m.actions_display_server()}:</span> {params.value.pacman.server}</p>
						{#if params.value.pacman.sigLevel}
							<p><span class="text-muted-foreground">{m.actions_display_signature_level()}:</span> {params.value.pacman.sigLevel}</p>
						{/if}
					</div>
				{/if}

				{#if params.value.zypper && !params.value.zypper.disabled}
					<div class="rounded-md border p-2 space-y-1">
						<p class="text-xs font-medium text-muted-foreground">{m.actions_params_package_zypper()}</p>
						<p><span class="text-muted-foreground">{m.actions_display_url()}:</span> {params.value.zypper.url}</p>
						{#if params.value.zypper.description}
							<p><span class="text-muted-foreground">{m.actions_display_description()}:</span> {params.value.zypper.description}</p>
						{/if}
						<p><span class="text-muted-foreground">{m.actions_display_enabled()}:</span> {yesNo(params.value.zypper.enabled)}</p>
						<p><span class="text-muted-foreground">{m.actions_display_autorefresh()}:</span> {yesNo(params.value.zypper.autorefresh)}</p>
						<p><span class="text-muted-foreground">{m.actions_display_gpg_check()}:</span> {yesNo(params.value.zypper.gpgcheck)}</p>
					</div>
				{/if}
			</div>
		{:else if params.case === 'flatpak'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.actions_display_app_id()}:</span> {params.value.appId}</p>
				{#if params.value.remote}
					<p><span class="text-muted-foreground">{m.actions_display_remote()}:</span> {params.value.remote}</p>
				{/if}
				<p><span class="text-muted-foreground">{m.actions_display_system_wide()}:</span> {yesNo(params.value.systemWide)}</p>
				<p><span class="text-muted-foreground">{m.actions_display_pin_prevent_updates()}:</span> {yesNo(params.value.pin)}</p>
			</div>
		{:else if params.case === 'directory'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.actions_display_path()}:</span> {params.value.path}</p>
				{#if params.value.owner}
					<p><span class="text-muted-foreground">{m.actions_display_owner()}:</span> {params.value.owner}</p>
				{/if}
				{#if params.value.group}
					<p><span class="text-muted-foreground">{m.actions_display_group()}:</span> {params.value.group}</p>
				{/if}
				{#if params.value.mode}
					<p><span class="text-muted-foreground">{m.actions_display_mode()}:</span> {params.value.mode}</p>
				{/if}
				<p><span class="text-muted-foreground">{m.actions_display_recursive()}:</span> {yesNo(params.value.recursive)}</p>
			</div>
		{:else if params.case === 'user'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.actions_params_user_username()}:</span> {params.value.username}</p>
				{#if params.value.uid}
					<p><span class="text-muted-foreground">{m.actions_params_user_uid()}:</span> {params.value.uid}</p>
				{/if}
				{#if params.value.gid}
					<p><span class="text-muted-foreground">{m.actions_params_user_gid()}:</span> {params.value.gid}</p>
				{/if}
				{#if params.value.primaryGroup}
					<p><span class="text-muted-foreground">{m.actions_params_user_primary_group()}:</span> {params.value.primaryGroup}</p>
				{/if}
				{#if params.value.homeDir}
					<p><span class="text-muted-foreground">{m.actions_params_user_home_dir()}:</span> {params.value.homeDir}</p>
				{/if}
				{#if params.value.shell}
					<p><span class="text-muted-foreground">{m.actions_params_user_shell()}:</span> {params.value.shell}</p>
				{/if}
				{#if params.value.comment}
					<p><span class="text-muted-foreground">{m.actions_params_user_comment()}:</span> {params.value.comment}</p>
				{/if}
				<p><span class="text-muted-foreground">{m.actions_params_user_system_user()}:</span> {yesNo(params.value.systemUser)}</p>
				<p><span class="text-muted-foreground">{m.actions_params_user_create_home()}:</span> {yesNo(params.value.createHome)}</p>
				<p><span class="text-muted-foreground">{m.actions_params_user_disabled()}:</span> {yesNo(params.value.disabled)}</p>
				<p><span class="text-muted-foreground">{m.actions_params_user_hidden()}:</span> {yesNo(params.value.hidden)}</p>
			</div>
		{:else if params.case === 'ssh'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.ssh_users()}:</span> {params.value.users.join(', ') || '—'}</p>
				<p><span class="text-muted-foreground">{m.ssh_allow_pubkey()}:</span> {yesNo(params.value.allowPubkey)}</p>
				<p><span class="text-muted-foreground">{m.ssh_allow_password()}:</span> {yesNo(params.value.allowPassword)}</p>
			</div>
		{:else if params.case === 'sshd'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.sshd_priority()}:</span> {params.value.priority}</p>
				{#if params.value.directives.length > 0}
					<div class="space-y-1 rounded-md border p-2 mt-2">
						<p class="text-xs font-medium text-muted-foreground">{m.sshd_directives()}:</p>
						{#each params.value.directives as directive}
							<p><code class="text-xs">{directive.key}</code> <span class="text-muted-foreground">{directive.value}</span></p>
						{/each}
					</div>
				{/if}
			</div>
		{:else if params.case === 'adminPolicy'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.sudo_access_level()}:</span> {params.value.accessLevel === 1 ? m.sudo_access_level_full() : params.value.accessLevel === 2 ? m.sudo_access_level_limited() : m.sudo_access_level_custom()}</p>
				<p><span class="text-muted-foreground">{m.sudo_users()}:</span> {params.value.users.join(', ')}</p>
				{#if params.value.accessLevel === 3 && params.value.customConfig}
					<div>
						<p class="text-muted-foreground mb-1">{m.sudo_custom_config()}:</p>
						<pre class="bg-muted p-2 rounded text-xs overflow-x-auto max-h-48">{params.value.customConfig}</pre>
					</div>
				{/if}
			</div>
		{:else if params.case === 'lps'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.lps_usernames()}:</span> {params.value.usernames.join(', ')}</p>
				<p><span class="text-muted-foreground">{m.lps_password_length()}:</span> {params.value.passwordLength}</p>
				<p><span class="text-muted-foreground">{m.lps_complexity()}:</span> {params.value.complexity === 2 ? m.lps_complexity_complex() : m.lps_complexity_alphanumeric()}</p>
				<p><span class="text-muted-foreground">{m.lps_rotation_interval()}:</span> {params.value.rotationIntervalDays} {m.lps_days()}</p>
				<p><span class="text-muted-foreground">{m.lps_grace_period()}:</span> {params.value.gracePeriodHours} {m.lps_hours()}</p>
			</div>
		{:else if params.case === 'encryption'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.luks_rotation_interval()}:</span> {params.value.rotationIntervalDays} {m.luks_days()}</p>
				<p><span class="text-muted-foreground">{m.luks_min_words()}:</span> {params.value.minWords}</p>
				<p><span class="text-muted-foreground">{m.luks_device_bound_key_type()}:</span> {params.value.deviceBoundKeyType === 1 ? m.luks_device_bound_key_tpm() : params.value.deviceBoundKeyType === 2 ? m.luks_device_bound_key_user_passphrase() : m.luks_device_bound_key_none()}</p>
				{#if params.value.deviceBoundKeyType === 2}
					<p><span class="text-muted-foreground">{m.luks_user_passphrase_min_length()}:</span> {params.value.userPassphraseMinLength}</p>
					<p><span class="text-muted-foreground">{m.lps_complexity()}:</span> {params.value.userPassphraseComplexity === 2 ? m.lps_complexity_complex() : m.lps_complexity_alphanumeric()}</p>
				{/if}
			</div>
		{:else if params.case === 'group'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.group_name()}:</span> {params.value.name}</p>
				<p><span class="text-muted-foreground">{m.group_members()}:</span> {params.value.members.join(', ')}</p>
				{#if params.value.gid}
					<p><span class="text-muted-foreground">{m.group_gid()}:</span> {params.value.gid}</p>
				{/if}
				{#if params.value.systemGroup}
					<p><span class="text-muted-foreground">{m.group_system_group()}:</span> {m.actions_display_yes()}</p>
				{/if}
			</div>
		{:else if params.case === 'wifi'}
			<div class="text-sm space-y-1">
				<p><span class="text-muted-foreground">{m.wifi_ssid()}:</span> {params.value.ssid}</p>
				<p><span class="text-muted-foreground">{m.wifi_auth_type()}:</span> {params.value.authType === 1 ? m.wifi_auth_type_psk() : params.value.authType === 2 ? m.wifi_auth_type_eap_tls() : '?'}</p>
				{#if params.value.authType === 2 && params.value.identity}
					<p><span class="text-muted-foreground">{m.wifi_identity()}:</span> {params.value.identity}</p>
				{/if}
				<p><span class="text-muted-foreground">{m.wifi_auto_connect()}:</span> {yesNo(params.value.autoConnect)}</p>
				<p><span class="text-muted-foreground">{m.wifi_hidden()}:</span> {yesNo(params.value.hidden)}</p>
				{#if params.value.priority}
					<p><span class="text-muted-foreground">{m.wifi_priority()}:</span> {params.value.priority}</p>
				{/if}
			</div>
		{:else if params.case === 'agentUpdate'}
			<div class="text-sm space-y-1">
				{#if params.value.amd64}
					<p class="font-medium">{m.agent_update_amd64_section()}</p>
					<p><span class="text-muted-foreground">{m.agent_update_amd64_binary_url()}:</span> {params.value.amd64.binaryUrl}</p>
					<p><span class="text-muted-foreground">{m.agent_update_amd64_checksum_url()}:</span> {params.value.amd64.checksumUrl}</p>
				{/if}
				{#if params.value.arm64}
					<p class="font-medium">{m.agent_update_arm64_section()}</p>
					<p><span class="text-muted-foreground">{m.agent_update_arm64_binary_url()}:</span> {params.value.arm64.binaryUrl}</p>
					<p><span class="text-muted-foreground">{m.agent_update_arm64_checksum_url()}:</span> {params.value.arm64.checksumUrl}</p>
				{/if}
				{#if params.value.allowRedirect}
					<p><span class="text-muted-foreground">{m.agent_update_allow_redirect()}:</span> {m.common_yes()}</p>
				{/if}
			</div>
		{/if}
	</div>
{:else}
	<p class="text-muted-foreground">{m.actions_display_no_params()}</p>
{/if}
