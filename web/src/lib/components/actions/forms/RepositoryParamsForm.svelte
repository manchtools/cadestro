<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Switch } from '$lib/components/ui/switch';
	import * as Tabs from '$lib/components/ui/tabs';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { RepositoryFormState } from './types';

	interface Props {
		params: RepositoryFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();
</script>

<div class="space-y-4">
	<div class="space-y-2">
		<Label for="repoName">{m.actions_params_repo_name()}</Label>
		<Input
			id="repoName"
			placeholder="e.g., docker-ce, vscode"
			bind:value={params.name}
			required
			aria-invalid={!!errors?.name}
			oninput={() => onclearerror?.('name')}
		/>
		<p class="text-xs text-muted-foreground">
			{m.actions_params_repo_name_description()}
		</p>
		<FieldError error={errors?.name} />
	</div>

	<Tabs.Root value="apt" class="w-full">
		<Tabs.List class="grid w-full grid-cols-4">
			<Tabs.Trigger value="apt" class={params.apt.disabled ? 'opacity-50' : ''}>
				APT {!params.apt.disabled ? '✓' : ''}
			</Tabs.Trigger>
			<Tabs.Trigger value="dnf" class={params.dnf.disabled ? 'opacity-50' : ''}>
				DNF {!params.dnf.disabled ? '✓' : ''}
			</Tabs.Trigger>
			<Tabs.Trigger value="pacman" class={params.pacman.disabled ? 'opacity-50' : ''}>
				Pacman {!params.pacman.disabled ? '✓' : ''}
			</Tabs.Trigger>
			<Tabs.Trigger value="zypper" class={params.zypper.disabled ? 'opacity-50' : ''}>
				Zypper {!params.zypper.disabled ? '✓' : ''}
			</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="apt" class="space-y-4 pt-4">
			<div class="flex items-center justify-between">
				<div class="space-y-0.5">
					<Label for="aptEnabled">{m.actions_params_repo_apt_enable()}</Label>
					<p class="text-xs text-muted-foreground">{m.actions_params_repo_apt_description()}</p>
				</div>
				<Switch id="aptEnabled" checked={!params.apt.disabled} onCheckedChange={(v) => params.apt.disabled = !v} />
			</div>

			{#if !params.apt.disabled}
				<div class="space-y-4 rounded-md border p-3">
					<div class="space-y-2">
						<Label for="aptUrl">{m.actions_params_repo_apt_url()}</Label>
						<Input
							id="aptUrl"
							placeholder="https://download.docker.com/linux/ubuntu"
							bind:value={params.apt.url}
						/>
					</div>
					<div class="grid grid-cols-2 gap-3">
						<div class="space-y-2">
							<Label for="aptDistribution">{m.actions_params_repo_apt_distribution()}</Label>
							<Input
								id="aptDistribution"
								placeholder="e.g., jammy, noble"
								bind:value={params.apt.distribution}
							/>
						</div>
						<div class="space-y-2">
							<Label for="aptComponents">{m.actions_params_repo_apt_components()}</Label>
							<Input
								id="aptComponents"
								placeholder="stable (space-separated)"
								bind:value={params.apt.components}
							/>
						</div>
					</div>
					<div class="space-y-2">
						<Label for="aptArch">{m.actions_params_repo_apt_arch()}</Label>
						<Input
							id="aptArch"
							placeholder="e.g., amd64"
							bind:value={params.apt.arch}
						/>
					</div>
					<div class="space-y-2">
						<Label for="aptGpgKeyUrl">{m.actions_params_repo_apt_gpg_key_url()}</Label>
						<Input
							id="aptGpgKeyUrl"
							placeholder="https://download.docker.com/linux/ubuntu/gpg"
							bind:value={params.apt.gpgKeyUrl}
						/>
					</div>
					<div class="space-y-2">
						<Label for="aptGpgKey">{m.actions_params_repo_apt_gpg_key_inline()}</Label>
						<Textarea
							id="aptGpgKey"
							placeholder="-----BEGIN PGP PUBLIC KEY BLOCK-----"
							bind:value={params.apt.gpgKey}
							rows={3}
							class="font-mono text-xs"
						/>
					</div>
					<div class="flex items-center justify-between">
						<div class="space-y-0.5">
							<Label for="aptTrusted">{m.actions_params_repo_apt_trusted()}</Label>
							<p class="text-xs text-muted-foreground">{m.actions_params_repo_apt_trusted_warning()}</p>
						</div>
						<Switch id="aptTrusted" bind:checked={params.apt.trusted} />
					</div>
				</div>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="dnf" class="space-y-4 pt-4">
			<div class="flex items-center justify-between">
				<div class="space-y-0.5">
					<Label for="dnfEnabled">{m.actions_params_repo_dnf_enable()}</Label>
					<p class="text-xs text-muted-foreground">{m.actions_params_repo_dnf_description()}</p>
				</div>
				<Switch id="dnfEnabled" checked={!params.dnf.disabled} onCheckedChange={(v) => params.dnf.disabled = !v} />
			</div>

			{#if !params.dnf.disabled}
				<div class="space-y-4 rounded-md border p-3">
					<div class="space-y-2">
						<Label for="dnfBaseurl">{m.actions_params_repo_dnf_baseurl()}</Label>
						<Input
							id="dnfBaseurl"
							placeholder="https://download.docker.com/linux/fedora/$releasever/$basearch/stable"
							bind:value={params.dnf.baseurl}
						/>
					</div>
					<div class="space-y-2">
						<Label for="dnfDescription">{m.actions_params_repo_dnf_repo_description()}</Label>
						<Input
							id="dnfDescription"
							placeholder="Docker CE Stable"
							bind:value={params.dnf.description}
						/>
					</div>
					<div class="space-y-2">
						<Label for="dnfGpgkey">{m.actions_params_repo_dnf_gpg_key()}</Label>
						<Input
							id="dnfGpgkey"
							placeholder="https://download.docker.com/linux/fedora/gpg"
							bind:value={params.dnf.gpgkey}
						/>
					</div>
					<div class="grid grid-cols-2 gap-3">
						<div class="flex items-center justify-between">
							<Label for="dnfEnabled">{m.actions_params_repo_dnf_enabled()}</Label>
							<Switch id="dnfEnabled" bind:checked={params.dnf.enabled} />
						</div>
						<div class="flex items-center justify-between">
							<Label for="dnfGpgcheck">{m.actions_params_repo_dnf_gpgcheck()}</Label>
							<Switch id="dnfGpgcheck" bind:checked={params.dnf.gpgcheck} />
						</div>
					</div>
					<div class="flex items-center justify-between">
						<div class="space-y-0.5">
							<Label for="dnfModuleHotfixes">{m.actions_params_repo_dnf_module_hotfixes()}</Label>
							<p class="text-xs text-muted-foreground">{m.actions_params_repo_dnf_module_hotfixes_description()}</p>
						</div>
						<Switch id="dnfModuleHotfixes" bind:checked={params.dnf.moduleHotfixes} />
					</div>
				</div>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="pacman" class="space-y-4 pt-4">
			<div class="flex items-center justify-between">
				<div class="space-y-0.5">
					<Label for="pacmanEnabled">{m.actions_params_repo_pacman_enable()}</Label>
					<p class="text-xs text-muted-foreground">{m.actions_params_repo_pacman_description()}</p>
				</div>
				<Switch id="pacmanEnabled" checked={!params.pacman.disabled} onCheckedChange={(v) => params.pacman.disabled = !v} />
			</div>

			{#if !params.pacman.disabled}
				<div class="space-y-4 rounded-md border p-3">
					<div class="space-y-2">
						<Label for="pacmanServer">{m.actions_params_repo_pacman_server()}</Label>
						<Input
							id="pacmanServer"
							placeholder="https://mirror.example.com/$repo/os/$arch"
							bind:value={params.pacman.server}
						/>
						<p class="text-xs text-muted-foreground">
							{m.actions_params_repo_pacman_server_description()}
						</p>
					</div>
					<div class="space-y-2">
						<Label for="pacmanSigLevel">{m.actions_params_repo_pacman_sig_level()}</Label>
						<Input
							id="pacmanSigLevel"
							placeholder="Optional TrustAll"
							bind:value={params.pacman.sigLevel}
						/>
					</div>
				</div>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="zypper" class="space-y-4 pt-4">
			<div class="flex items-center justify-between">
				<div class="space-y-0.5">
					<Label for="zypperEnabled">{m.actions_params_repo_zypper_enable()}</Label>
					<p class="text-xs text-muted-foreground">{m.actions_params_repo_zypper_description()}</p>
				</div>
				<Switch id="zypperEnabled" checked={!params.zypper.disabled} onCheckedChange={(v) => params.zypper.disabled = !v} />
			</div>

			{#if !params.zypper.disabled}
				<div class="space-y-4 rounded-md border p-3">
					<div class="space-y-2">
						<Label for="zypperUrl">{m.actions_params_repo_zypper_url()}</Label>
						<Input
							id="zypperUrl"
							placeholder="https://download.docker.com/linux/sles/$releasever/$basearch/stable"
							bind:value={params.zypper.url}
						/>
					</div>
					<div class="space-y-2">
						<Label for="zypperDescription">{m.actions_params_repo_zypper_repo_description()}</Label>
						<Input
							id="zypperDescription"
							placeholder="Docker CE Stable"
							bind:value={params.zypper.description}
						/>
					</div>
					<div class="space-y-2">
						<Label for="zypperType">{m.actions_params_repo_zypper_type()}</Label>
						<Input
							id="zypperType"
							placeholder="rpm-md (default)"
							bind:value={params.zypper.type}
						/>
					</div>
					<div class="space-y-2">
						<Label for="zypperGpgkey">{m.actions_params_repo_zypper_gpg_key()}</Label>
						<Input
							id="zypperGpgkey"
							placeholder="https://download.docker.com/linux/sles/gpg"
							bind:value={params.zypper.gpgkey}
						/>
					</div>
					<div class="grid grid-cols-2 gap-3">
						<div class="flex items-center justify-between">
							<Label for="zypperEnabled">{m.actions_params_repo_zypper_enabled()}</Label>
							<Switch id="zypperEnabled" bind:checked={params.zypper.enabled} />
						</div>
						<div class="flex items-center justify-between">
							<Label for="zypperGpgcheck">{m.actions_params_repo_zypper_gpgcheck()}</Label>
							<Switch id="zypperGpgcheck" bind:checked={params.zypper.gpgcheck} />
						</div>
					</div>
					<div class="flex items-center justify-between">
						<div class="space-y-0.5">
							<Label for="zypperAutorefresh">{m.actions_params_repo_zypper_autorefresh()}</Label>
							<p class="text-xs text-muted-foreground">{m.actions_params_repo_zypper_autorefresh_description()}</p>
						</div>
						<Switch id="zypperAutorefresh" bind:checked={params.zypper.autorefresh} />
					</div>
				</div>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>
