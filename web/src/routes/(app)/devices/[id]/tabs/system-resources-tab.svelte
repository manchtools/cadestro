<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient, type LpsPassword, type LuksKey, formatTimestampDateTime } from '$lib/sdk';
	import { LuksRevocationStatus, RotationReason } from '$contract/cadestro/v1/common_pb';
	import { getLocalizedError } from '$lib/errors';
	import { Button } from '$lib/components/ui/button';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Chip } from '$lib/components/fleet';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import SecretCell from './secret-cell.svelte';
	import { Lock, HardDrive, ExternalLink, ShieldOff, Copy } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		deviceId: string;
	}

	let { deviceId }: Props = $props();

	let lpsCurrentPasswords = $state<LpsPassword[]>([]);
	let lpsHistoryPasswords = $state<LpsPassword[]>([]);
	let lpsHistoryOpen = $state(false);
	let luksCurrentKeys = $state<LuksKey[]>([]);
	let luksHistoryKeys = $state<LuksKey[]>([]);
	let luksHistoryOpen = $state(false);
	// The token IS the authorization for writing a LUKS keyslot: single-use,
	// device-bound, 24h. The advertised command deliberately carries neither the
	// token nor sudo — argv is world-readable through /proc/<pid>/cmdline — so
	// the operator has to be handed the token separately, and this dialog is the
	// only place it exists in the client.
	//
	// One nullable holder drives BOTH the dialog and its contents, so the
	// credential cannot outlive the surface showing it: closing the dialog is
	// the assignment that drops it. It is never persisted or logged.
	let luksToken = $state<{ uri: string; cliCommand: string; token: string } | null>(null);
	let luksTokenLoading = $state(false);
	let luksRevokeDialogOpen = $state(false);
	let luksRevokeActionId = $state('');
	let luksRevokeDispatching = $state(false);
	let loaded = $state(false);

	// The list RPCs return metadata only. Plaintext comes one entry at a time
	// from the reveal RPCs below, each of which is an audited sensitive read.
	const lpsSecret = $derived({
		reveal: async (id: string) => (await apiClient.revealLpsPassword(id)).password,
		revealLabel: m.lps_passwords_reveal(),
		hideLabel: m.lps_passwords_hide(),
		copyLabel: m.lps_passwords_copy(),
		copiedMessage: m.lps_passwords_copied()
	});

	const luksSecret = $derived({
		reveal: async (id: string) => (await apiClient.revealLuksKey(id)).passphrase,
		revealLabel: m.luks_keys_reveal(),
		hideLabel: m.luks_keys_hide(),
		copyLabel: m.luks_keys_copy(),
		copiedMessage: m.luks_keys_copied()
	});

	$effect(() => {
		if (!loaded) {
			loaded = true;
			loadLpsPasswords();
			loadLuksKeys();
		}
	});

	async function loadLpsPasswords() {
		try {
			const response = await apiClient.listLpsPasswords(deviceId);
			lpsCurrentPasswords = response.current;
			lpsHistoryPasswords = response.history;
		} catch (error) {
			console.error('Failed to load LPS password metadata:', error);
		}
	}

	async function loadLuksKeys() {
		try {
			const response = await apiClient.listLuksKeys(deviceId);
			luksCurrentKeys = response.current;
			luksHistoryKeys = response.history;
		} catch (error) {
			console.error('Failed to load LUKS key metadata:', error);
		}
	}

	function getRotationReasonLabel(reason: RotationReason): string {
		switch (reason) {
			case RotationReason.INITIAL: return m.rotation_reason_initial();
			case RotationReason.SCHEDULED: return m.rotation_reason_scheduled();
			case RotationReason.AUTH_GRACE: return m.rotation_reason_auth_grace();
			default: return m.rotation_reason_unspecified();
		}
	}

	async function createLuksTokenFlow(actionId: string) {
		luksTokenLoading = true;
		try {
			const response = await apiClient.createLuksToken(deviceId, actionId);
			luksToken = {
				uri: response.uri,
				cliCommand: response.cliCommand,
				token: response.token
			};
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			luksTokenLoading = false;
		}
	}

	async function copyToClipboard(value: string, copiedMessage: string) {
		try {
			await navigator.clipboard.writeText(value);
			toast.success(copiedMessage);
		} catch {
			toast.error(m.common_copy_failed());
		}
	}

	async function revokeLuksDeviceKeyFlow() {
		if (!luksRevokeActionId) return;
		luksRevokeDispatching = true;
		try {
			await apiClient.revokeLuksDeviceKey(deviceId, luksRevokeActionId);
			toast.success(m.luks_revoke_dispatched());
			luksRevokeDialogOpen = false;
			luksRevokeActionId = '';
			await loadLuksKeys();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			luksRevokeDispatching = false;
		}
	}
</script>

{#snippet sectionLabel(text: string)}
	<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{text}</span>
{/snippet}

<div class="space-y-6">
	{#if luksCurrentKeys.length > 0}
	<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
		<div class="flex items-center gap-2">
			<HardDrive class="h-4 w-4 text-faint" />
			{@render sectionLabel(m.luks_keys_title())}
		</div>
		<div class="mt-3">
			<div class="space-y-4">
				<div>
					<h4 class="text-sm font-medium mb-2">{m.luks_keys_current()}</h4>
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>{m.luks_keys_device_path()}</Table.Head>
								<Table.Head>{m.luks_keys_action()}</Table.Head>
								<Table.Head>{m.luks_keys_passphrase()}</Table.Head>
								<Table.Head>{m.luks_keys_rotated_at()}</Table.Head>
								<Table.Head>{m.luks_keys_reason()}</Table.Head>
								<Table.Head>{m.luks_revoke_status()}</Table.Head>
								<Table.Head></Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each luksCurrentKeys as lk}
								<Table.Row>
									<Table.Cell class="font-mono text-xs">{lk.devicePath}</Table.Cell>
							<Table.Cell>{lk.actionName || (lk.actionId?.value ?? '').slice(0, 8)}</Table.Cell>
									<Table.Cell>
										<SecretCell id={lk.id} {...luksSecret} />
									</Table.Cell>
									<Table.Cell class="text-sm">{formatTimestampDateTime(lk.rotatedAt)}</Table.Cell>
									<Table.Cell><Badge variant="outline">{getRotationReasonLabel(lk.rotationReason)}</Badge></Table.Cell>
									<Table.Cell>
										{#if lk.revocationStatus === LuksRevocationStatus.DISPATCHED}
											<Chip tone="info" label={m.luks_revoke_status_dispatched()} />
										{:else if lk.revocationStatus === LuksRevocationStatus.SUCCESS}
											<Chip tone="ok" label={m.luks_revoke_status_success()} />
										{:else if lk.revocationStatus === LuksRevocationStatus.FAILED}
											<Chip tone="crit" label={m.luks_revoke_status_failed()} />
										{/if}
										{#if lk.revocationAt}
											<div class="text-xs text-muted-foreground">{m.luks_keys_revocation_at({ time: formatTimestampDateTime(lk.revocationAt) })}</div>
										{/if}
										{#if lk.revocationStatus === LuksRevocationStatus.FAILED && lk.revocationError}
											<div class="text-xs text-destructive" title={lk.revocationError}>{lk.revocationError}</div>
										{/if}
									</Table.Cell>
									<Table.Cell>
										<div class="flex items-center gap-1">
										<Button variant="ghost" size="sm" onclick={() => createLuksTokenFlow(lk.actionId?.value ?? '')} disabled={luksTokenLoading}>
												<ExternalLink class="h-3 w-3 mr-1" />
												{m.luks_set_passphrase()}
											</Button>
											{#if lk.revocationStatus !== LuksRevocationStatus.DISPATCHED}
													<Button variant="ghost" size="sm" class="text-destructive" onclick={() => { luksRevokeActionId = lk.actionId?.value ?? ''; luksRevokeDialogOpen = true; }}>
													<ShieldOff class="h-3 w-3 mr-1" />
													{m.luks_revoke_device_key()}
												</Button>
											{/if}
										</div>
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
				{#if luksHistoryKeys.length > 0}
					<div>
						<button class="text-sm font-medium text-muted-foreground hover:text-foreground flex items-center gap-1" onclick={() => luksHistoryOpen = !luksHistoryOpen}>
							{m.luks_keys_history()} ({luksHistoryKeys.length})
							<span class="text-xs">{luksHistoryOpen ? '▼' : '▶'}</span>
						</button>
						{#if luksHistoryOpen}
							<Table.Root class="mt-2">
								<Table.Header>
									<Table.Row>
										<Table.Head>{m.luks_keys_device_path()}</Table.Head>
										<Table.Head>{m.luks_keys_action()}</Table.Head>
										<Table.Head>{m.luks_keys_passphrase()}</Table.Head>
										<Table.Head>{m.luks_keys_rotated_at()}</Table.Head>
										<Table.Head>{m.luks_keys_reason()}</Table.Head>
									</Table.Row>
								</Table.Header>
								<Table.Body>
									{#each luksHistoryKeys as lk}
										<Table.Row class="opacity-60">
											<Table.Cell class="font-mono text-xs">{lk.devicePath}</Table.Cell>
							<Table.Cell>{lk.actionName || (lk.actionId?.value ?? '').slice(0, 8)}</Table.Cell>
											<Table.Cell>
												<SecretCell id={lk.id} {...luksSecret} />
											</Table.Cell>
											<Table.Cell class="text-sm">{formatTimestampDateTime(lk.rotatedAt)}</Table.Cell>
											<Table.Cell><Badge variant="outline">{getRotationReasonLabel(lk.rotationReason)}</Badge></Table.Cell>
										</Table.Row>
									{/each}
								</Table.Body>
							</Table.Root>
						{/if}
					</div>
				{/if}
			</div>
		</div>
	</section>
	{/if}

	{#if lpsCurrentPasswords.length > 0}
	<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
		<div class="flex items-center gap-2">
			<Lock class="h-4 w-4 text-faint" />
			{@render sectionLabel(m.lps_passwords_title())}
		</div>
		<div class="mt-3">
			<div class="space-y-4">
				<div>
					<h4 class="text-sm font-medium mb-2">{m.lps_passwords_current()}</h4>
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>{m.lps_passwords_username()}</Table.Head>
								<Table.Head>{m.lps_passwords_action()}</Table.Head>
								<Table.Head>{m.lps_passwords_password()}</Table.Head>
								<Table.Head>{m.lps_passwords_rotated_at()}</Table.Head>
								<Table.Head>{m.lps_passwords_reason()}</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each lpsCurrentPasswords as pw}
								<Table.Row>
									<Table.Cell class="font-medium">{pw.username}</Table.Cell>
							<Table.Cell>{pw.actionName || (pw.actionId?.value ?? '').slice(0, 8)}</Table.Cell>
									<Table.Cell>
										<SecretCell id={pw.id} {...lpsSecret} />
									</Table.Cell>
									<Table.Cell class="text-sm">{formatTimestampDateTime(pw.rotatedAt)}</Table.Cell>
									<Table.Cell><Badge variant="outline">{getRotationReasonLabel(pw.rotationReason)}</Badge></Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
				{#if lpsHistoryPasswords.length > 0}
					<div>
						<button class="text-sm font-medium text-muted-foreground hover:text-foreground flex items-center gap-1" onclick={() => lpsHistoryOpen = !lpsHistoryOpen}>
							{m.lps_passwords_history()} ({lpsHistoryPasswords.length})
							<span class="text-xs">{lpsHistoryOpen ? '▼' : '▶'}</span>
						</button>
						{#if lpsHistoryOpen}
							<Table.Root class="mt-2">
								<Table.Header>
									<Table.Row>
										<Table.Head>{m.lps_passwords_username()}</Table.Head>
										<Table.Head>{m.lps_passwords_action()}</Table.Head>
										<Table.Head>{m.lps_passwords_password()}</Table.Head>
										<Table.Head>{m.lps_passwords_rotated_at()}</Table.Head>
										<Table.Head>{m.lps_passwords_reason()}</Table.Head>
									</Table.Row>
								</Table.Header>
								<Table.Body>
									{#each lpsHistoryPasswords as pw}
										<Table.Row class="opacity-60">
											<Table.Cell class="font-medium">{pw.username}</Table.Cell>
							<Table.Cell>{pw.actionName || (pw.actionId?.value ?? '').slice(0, 8)}</Table.Cell>
											<Table.Cell>
												<SecretCell id={pw.id} {...lpsSecret} />
											</Table.Cell>
											<Table.Cell class="text-sm">{formatTimestampDateTime(pw.rotatedAt)}</Table.Cell>
											<Table.Cell><Badge variant="outline">{getRotationReasonLabel(pw.rotationReason)}</Badge></Table.Cell>
										</Table.Row>
									{/each}
								</Table.Body>
							</Table.Root>
						{/if}
					</div>
				{/if}
			</div>
		</div>
	</section>
	{/if}

	{#if luksCurrentKeys.length === 0 && lpsCurrentPasswords.length === 0}
		<div
			class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface py-12 text-center shadow-plate"
		>
			<Lock class="mb-2 h-8 w-8 text-muted-foreground" />
			<p class="text-muted-foreground">{m.device_secrets_empty()}</p>
		</div>
	{/if}
</div>

<!-- Two routes, because only one of them works for a given operator: the URI
     needs a desktop session with the handler registered, and most of this
     fleet is administered over SSH. The terminal route is therefore complete
     on its own — command AND token — and the token gets its own plate rather
     than sitting beside the command, because it is the credential and the
     command is not. -->
<AlertDialog.Root open={luksToken !== null} onOpenChange={(open) => !open && (luksToken = null)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.luks_set_passphrase()}</AlertDialog.Title>
			<AlertDialog.Description>{m.luks_set_passphrase_description()}</AlertDialog.Description>
		</AlertDialog.Header>
		{#if luksToken}
			<div class="space-y-4 py-2">
				<div>
					{@render sectionLabel(m.luks_token_route_desktop())}
					<div class="mt-2">
						<a href={luksToken.uri} class="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
							<ExternalLink class="h-4 w-4" />
							{m.luks_set_passphrase()}
						</a>
					</div>
				</div>
				<div class="border-t border-hair pt-4">
					{@render sectionLabel(m.luks_token_route_terminal())}
					<p class="mt-2 text-sm text-muted-foreground">{m.luks_set_passphrase_or_manual()}</p>
					<div class="mt-2 flex items-center gap-2">
						<code data-testid="luks-cli-command" class="flex-1 text-xs bg-muted px-3 py-2 rounded font-mono select-all">{luksToken.cliCommand}</code>
						<Button
							variant="ghost"
							size="icon"
							class="h-8 w-8"
							aria-label={m.luks_copy_command()}
							onclick={() => copyToClipboard(luksToken!.cliCommand, m.luks_command_copied())}
						>
							<Copy class="h-4 w-4" />
						</Button>
					</div>
					<div class="mt-3 rounded-lg border border-hair bg-sunken p-3">
						{@render sectionLabel(m.luks_token_label())}
						<div class="mt-2 flex items-center gap-2">
							<code
								data-testid="luks-token"
								class="min-w-0 flex-1 overflow-x-auto rounded-md border bg-surface px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap select-all"
								>{luksToken.token}</code
							>
							<Button
								variant="outline"
								size="icon"
								aria-label={m.luks_copy_token()}
								onclick={() => copyToClipboard(luksToken!.token, m.luks_token_copied())}
							>
								<Copy class="h-4 w-4" />
							</Button>
						</div>
						<p class="mt-2 text-xs text-muted-foreground">{m.luks_token_paste_hint()}</p>
					</div>
				</div>
				<p class="text-xs text-muted-foreground">{m.luks_set_passphrase_note()}</p>
			</div>
		{/if}
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_done()}</AlertDialog.Cancel>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={luksRevokeDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.luks_revoke_device_key()}</AlertDialog.Title>
			<AlertDialog.Description>{m.luks_revoke_device_key_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={revokeLuksDeviceKeyFlow} disabled={luksRevokeDispatching}>
				{luksRevokeDispatching ? m.instant_actions_requesting() : m.luks_revoke_device_key()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
