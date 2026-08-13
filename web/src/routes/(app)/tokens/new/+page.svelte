<script lang="ts">
	// /tokens/new — registration-token creation as a pill-committed surface.
	//
	// It used to be a modal, which meant the operator could not park a half-filled
	// token and go look something up: a dialog owns its own footer and dies on
	// navigation, so it can never take part in the pill's three exits
	// (Save / Stash / Cancel). Declaring `route` is what buys the Stash button.
	//
	// The created secret is returned exactly once, by the create RPC, so the
	// reveal lives HERE rather than on the list page it used to pop over —
	// navigating away with the value on screen would destroy it.
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient, authStore, configStore, useDraft, type RegistrationToken } from '$lib/sdk';
	import { createTokenSchema } from '$lib/forms/schemas/tokens';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import { Key, Copy } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { emptyDraft, hydrate, type TokenDraft } from './draft';

	const CONTEXT_ID = 'token:create';
	const ROUTE = '/tokens/new';

	// Reload survival: the token buffer has its own SDK draft bucket already.
	const persist = useDraft<TokenDraft>('create-token', 'default', emptyDraft());

	// Two ways back into an unfinished token, in precedence order: a STASHED card
	// (the operator parked it deliberately, and it is the only one that survives a
	// restore from another route), then the autosave, which also survives a reload.
	// `bindBuilderContext` performs the claim, so the binding and the claim cannot
	// drift apart.
	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());
	// svelte-ignore state_referenced_locally
	let draft = $state<TokenDraft>(hydrate(claimed) ?? hydrate(persist.data) ?? emptyDraft());

	/** A create surface opens EMPTY: there is nothing to save and nothing worth
	 *  parking. Saying `dirty: true` regardless is what made an untouched form
	 *  offer Save, and auto-stash itself onto the stage on the way out. */
	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);
	/** Parked on the stage — the pill must NOT re-enter this context. */
	let parked = $state(false);
	/** The one and only sighting of the secret; never read from a stored token. */
	let created = $state<RegistrationToken | null>(null);
	/** Per-CA enrollment pin, delivered BESIDE the token by the create RPC. Not
	 *  secret the way the bearer is, but the installer's `-p` refuses to enroll
	 *  without it, so it is presented (and copyable) right next to the token. */
	let caPin = $state('');

	$effect(() => {
		persist.data = $state.snapshot(draft) as TokenDraft;
	});

	// The same schema the dialog submitted against, evaluated live so the pill's
	// commit is closed before it is pressed rather than after.
	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		const result = createTokenSchema.safeParse({
			name: draft.name.trim(),
			oneTime: draft.oneTime,
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
			? `curl -fsSL https://github.com/MANCHTOOLS/power-manage-agent/releases/latest/download/install.sh | sudo bash -s -- -s ${configStore.serverUrl} -t ${created.value} -p ${caPin}`
			: ''
	);

	function snapshot() {
		// Nothing to commit once the token exists — the pill goes back to nav and
		// the reveal owns the surface.
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
			// The buffer rides ON the card too, not only in the autosave: a claim
			// after a cross-route restore must not depend on the debounced write.
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
			// Bulk enrollment ⇒ ownerless token (the server stores owner_id NULL and
			// devices enrolled through it are not auto-assigned). Otherwise the
			// current user owns the token.
			const ownerId = draft.bulkEnrollment ? '' : (authStore.user?.id ?? '');

			const response = await apiClient.createToken(
				draft.name.trim(),
				draft.oneTime,
				draft.maxUses,
				expiresAt,
				ownerId
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
			<!-- The CA pin lives beside the token because enrollment needs BOTH:
			     the installer's -p refuses to run without it. Per-CA, not secret
			     like the bearer — but un-copyable here meant un-enrollable. -->
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

			<div class="flex items-center justify-between rounded-lg border p-4">
				<div class="space-y-0.5">
					<Label for="token-one-time">{m.tokens_one_time_label()}</Label>
					<p class="text-xs text-muted-foreground">{m.tokens_one_time_hint()}</p>
				</div>
				<Switch id="token-one-time" bind:checked={draft.oneTime} />
			</div>

			<!-- Two counts, side by side: a use budget and a lifetime are the same
			     kind of small number, and a plate-wide field for "14" reads as an
			     invitation to type a sentence. One-time tokens drop the use budget
			     and the lifetime keeps the first column. -->
			<div class="grid gap-3 sm:grid-cols-2">
				{#if !draft.oneTime}
					<div class="space-y-1.5">
						<Label for="token-max-uses">{m.tokens_max_uses_label()}</Label>
						<Input
							id="token-max-uses"
							type="number"
							min="0"
							bind:value={draft.maxUses}
							aria-invalid={!!errors.maxUses}
						/>
						<FieldError error={errors.maxUses} />
					</div>
				{/if}

				<div class="space-y-1.5">
					<Label for="token-expires">{m.tokens_expires_label()}</Label>
					<Input
						id="token-expires"
						type="number"
						min="0"
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
						{:else}
							{m.tokens_expires_never_hint()}
						{/if}
					</p>
				</div>
			</div>

			<div class="flex items-center justify-between rounded-lg border p-4">
				<div class="space-y-0.5">
					<Label for="token-bulk">{m.tokens_bulk_enrollment_label()}</Label>
					<p class="text-xs text-muted-foreground">{m.tokens_bulk_enrollment_hint()}</p>
				</div>
				<Switch id="token-bulk" bind:checked={draft.bulkEnrollment} />
			</div>
		</CreatePlate>
	{/if}
</div>
