<script lang="ts">

	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient, configStore, persistDraft, useDraft, type RegistrationToken } from '$lib/sdk';
	import { createTokenSchema } from '$lib/forms/schemas/tokens';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { FieldError } from '$lib/components/ui/field-error';
	import { Key, Copy } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { emptyDraft, hydrate, type TokenDraft } from './draft';

	const CONTEXT_ID = 'token:create';
	const ROUTE = '/tokens/new';

	const persist = useDraft<TokenDraft>('create-token', 'default', emptyDraft());

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<TokenDraft>(hydrate(claimed) ?? hydrate(persist.data) ?? emptyDraft());

	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);

	let created = $state<RegistrationToken | null>(null);

	let caPin = $state('');

	persistDraft(persist, () => draft);

	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		const result = createTokenSchema.safeParse({
			name: draft.name.trim(),
			maxUses: draft.maxUses,
			expiresInDays: draft.expiresInDays
		});
		if (!result.success) {
			for (const issue of result.error.issues) {
				const field = issue.path.length ? String(issue.path[0]) : '_';
				if (!(field in out)) out[field] = issue.message;
			}
		}
		return out;
	});

	const firstError = $derived(Object.values(errors)[0] ?? null);

	const installCommand = $derived(
		created
			? `curl -fsSL https://github.com/manchtools/cadestro/releases/latest/download/cadestrod-install.sh | sudo bash -s -- -s ${configStore.serverUrl} -t ${created.value} -p ${caPin}`
			: ''
	);

	function snapshot() {

		if (saving || parked || created) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.tokens_create(),
			dirty: isDirty(),
			valid: firstError === null,
			commitLabel: m.common_create(),
			subtext: firstError ?? m.common_create_ready(),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.common_create_stash_subtitle({
				entity: draft.name.trim() || m.tokens_create()
			}),
			onCommit: () => void commit(),
			onCancel: cancel,
			onStash: () => (parked = true),
			onRestore: () => (parked = false),

			stashPayload: () => $state.snapshot(draft)
		};
	}

	function cancel() {
		void persist.clear();
		void goto('/tokens');
	}

	function copy(value: string) {
		navigator.clipboard.writeText(value);
		toast.success(m.tokens_copied());
	}

	async function commit() {
		if (firstError) return;
		saving = true;
		try {
			let expiresAt: Date | undefined;
			if (draft.expiresInDays > 0) {
				expiresAt = new Date();
				expiresAt.setDate(expiresAt.getDate() + draft.expiresInDays);
			}
			const response = await apiClient.createToken(
				draft.name.trim(),
				draft.maxUses,
				expiresAt
			);
			if (response?.token) {
				await persist.clear();
				created = response.token;
				caPin = response.caFingerprintPin;
				toast.success(m.tokens_created());
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.tokens_create_dialog_title()}</title></svelte:head>

<div class="flex-1 overflow-auto p-4 md:p-6">
	{#if created}
		<CreatePlate
			icon={Key}
			title={m.tokens_created_dialog_title()}
			description={m.tokens_created_dialog_description()}
			testid="token-created"
		>
			<div>
				<p class="mb-1.5 font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.tokens_created_token_label()}
				</p>
				<div class="flex items-center gap-2">
					<code
						data-testid="token-secret"
						class="min-w-0 flex-1 overflow-x-auto rounded-md border bg-sunken px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap"
						>{created.value}</code
					>
					<Button
						variant="outline"
						size="icon"
						aria-label={m.tokens_copy_token()}
						onclick={() => copy(created!.value)}
					>
						<Copy class="h-4 w-4" />
					</Button>
				</div>
			</div>

			<div>
				<p class="mb-1.5 font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.tokens_created_ca_pin_label()}
				</p>
				<div class="flex items-center gap-2">
					<code
						data-testid="token-ca-pin"
						class="min-w-0 flex-1 overflow-x-auto rounded-md border bg-sunken px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap"
						>{caPin}</code
					>
					<Button
						variant="outline"
						size="icon"
						aria-label={m.tokens_copy_ca_pin()}
						onclick={() => copy(caPin)}
					>
						<Copy class="h-4 w-4" />
					</Button>
				</div>
				<p class="mt-1.5 text-xs text-muted-foreground">{m.tokens_created_ca_pin_help()}</p>
			</div>
			<div>
				<p class="mb-1.5 font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.tokens_created_install_label()}
				</p>
				<p class="mb-2 text-sm text-muted-foreground">{m.tokens_created_install_hint()}</p>
				<div class="relative">
					<pre
						class="overflow-x-auto rounded-md border bg-sunken p-3 pr-10 font-mono text-xs break-all whitespace-pre-wrap">{installCommand}</pre>
					<Button
						variant="ghost"
						size="icon"
						aria-label={m.tokens_copy_install_command()}
						class="absolute top-1.5 right-1.5 h-7 w-7"
						onclick={() => copy(installCommand)}
					>
						<Copy class="h-3.5 w-3.5" />
					</Button>
				</div>
			</div>
			<div class="flex justify-end border-t pt-3">
				<Button onclick={() => goto('/tokens')}>{m.common_done()}</Button>
			</div>
		</CreatePlate>
	{:else}
		<CreatePlate
			icon={Key}
			title={m.tokens_create_dialog_title()}
			description={m.tokens_create_dialog_description()}
			testid="token-create"
		>
			<div class="space-y-1.5">
				<Label for="token-name">{m.tokens_name_label()}</Label>
				<Input
					id="token-name"
					placeholder={m.tokens_name_placeholder()}
					bind:value={draft.name}
					aria-invalid={!!errors.name}
				/>
				<FieldError error={errors.name} />
			</div>

			<div class="grid gap-3 sm:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="token-max-uses">{m.tokens_max_uses_label()}</Label>
					<Input id="token-max-uses" type="number" min="0" bind:value={draft.maxUses} aria-invalid={!!errors.maxUses} />
					<FieldError error={errors.maxUses} />
				</div>

				<div class="space-y-1.5">
					<Label for="token-expires">{m.tokens_expires_label()}</Label>
					<Input
						id="token-expires"
						type="number"
						min="1"
						bind:value={draft.expiresInDays}
						aria-invalid={!!errors.expiresInDays}
					/>
					<FieldError error={errors.expiresInDays} />
					<p class="text-xs text-muted-foreground">
						{#if draft.expiresInDays > 0}
							{m.tokens_expires_on({
								date: new Date(
									Date.now() + draft.expiresInDays * 24 * 60 * 60 * 1000
								).toLocaleDateString()
							})}
						{/if}
					</p>
				</div>
			</div>

		</CreatePlate>
	{/if}
</div>
