<script lang="ts">
 import { onMount } from 'svelte';
 import { page } from '$app/state';
 import { goto } from '$lib/navigation';
 import { toast } from 'svelte-sonner';
 import { api } from '$lib/api';
 import { Permission, type IdentityProvider } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { Button } from '$lib/components/ui/button';
 import { Input } from '$lib/components/ui/input';
 import { Label } from '$lib/components/ui/label';
 import { Checkbox } from '$lib/components/ui/checkbox';
 import PageShell from '$lib/components/page-shell.svelte';
 import FormSection from '$lib/components/create/form-section.svelte';
 import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
 import { Chip } from '$lib/components/fleet';
 import { ArrowLeft, RefreshCw, Copy } from '@lucide/svelte';
 import { getLocalizedError } from '$lib/errors';
 import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
 import * as m from '$lib/paraglide/messages';
 const { can } = consoleContext();
 type IdpForm = { name: string; enabled: boolean; clientId: string; issuerUrl: string; scopes: string };
 function formOf(p: IdentityProvider | null): IdpForm { return { name: p?.name ?? '', enabled: p?.enabled ?? false, clientId: p?.clientId?.value ?? '', issuerUrl: p?.issuerUrl ?? '', scopes: p?.scopes.join(', ') ?? 'openid, profile, email' }; }
 let provider = $state<IdentityProvider | null>(null);
 let form = $state(formOf(null));
 let baseline = $state(formOf(null));
 let loading = $state(true);
 let saving = $state(false);
 let deleteDialogOpen = $state(false);
 const providerId = $derived(page.params.id ?? '');
 const redirectUri = $derived(provider ? `${window.location.origin}/auth/callback/${provider.slug}` : '');
 const dirty = $derived(JSON.stringify(form) !== JSON.stringify(baseline));
 bindBuilderContext(`identity-provider:${page.params.id}`, () => provider && !saving ? { title: form.name, route: `/identity-providers/${providerId}`, dirty, valid: !!form.name.trim() && can(Permission.UPDATE_IDENTITY_PROVIDER), commitLabel: m.common_save(), onCommit: () => void saveProvider(), onCancel: () => { form = { ...baseline }; }, extraActions: can(Permission.DELETE_IDENTITY_PROVIDER) ? [{ id: 'delete', label: m.common_delete(), tone: 'danger', onRun: () => { deleteDialogOpen = true; } }] : [] } : null);
 async function loadProvider() { const preserveEdits = dirty; loading = true; try { if (can(Permission.GET_IDENTITY_PROVIDER)) { provider = (await api.getIdentityProvider({ id: { value: providerId } })).provider ?? null; } else if (can(Permission.LIST_IDENTITY_PROVIDERS)) provider = (await api.listIdentityProviders({})).providers.find(item => item.id?.value === providerId) ?? null; if (!preserveEdits) form = formOf(provider); baseline = formOf(provider); } catch(error) { toast.error(getLocalizedError(error)); } finally { loading = false; } }
 async function saveProvider() { if (!provider || !can(Permission.UPDATE_IDENTITY_PROVIDER)) return; saving = true;
 try { const id = provider.id;
 if (form.name !== provider.name) provider = (await api.renameIdentityProvider({ id, name: form.name.trim() })).provider ?? provider;
 provider = (await api.configureIdentityProvider({ id, clientId: { value: form.clientId.trim() }, issuerUrl: form.issuerUrl.trim(), scopes: form.scopes.split(',').map(value => value.trim()).filter(Boolean) })).provider ?? provider;
 if (form.enabled !== provider.enabled) provider = (form.enabled ? await api.enableIdentityProvider({ id }) : await api.disableIdentityProvider({ id })).provider ?? provider;
 form = formOf(provider); baseline = formOf(provider); toast.success(m.idp_detail_updated());
 } catch(error) { baseline = formOf(provider); toast.error(getLocalizedError(error)); } finally { saving = false; } }
 async function deleteProvider() { if (!can(Permission.DELETE_IDENTITY_PROVIDER)) return; try { await api.deleteIdentityProvider({ id: { value: providerId } }); await goto('/identity-providers'); } catch(error) { toast.error(getLocalizedError(error)); } }
 async function copyToClipboard(value: string, notice: string) { try { await navigator.clipboard.writeText(value); toast.success(notice); } catch(error) { toast.error(getLocalizedError(error)); } }
 onMount(loadProvider);
</script>
<PageShell contentClass="space-y-4">
	{#snippet header()}
		<div class="flex items-start gap-3">
			<Button
				variant="ghost"
				size="icon"
				aria-label={m.common_back()}
				onclick={() => goto('/identity-providers')}
			>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-2">
					<h1 class="truncate text-2xl font-bold">{provider?.name ?? m.idp_detail_title()}</h1>
					{#if provider}
						<Chip
							tone={provider.enabled ? 'ok' : 'idle'}
							label={provider.enabled ? m.idp_enabled() : m.idp_disabled()}
						/>

					{/if}
				</div>
				<p class="font-mono text-xs text-faint">{provider?.slug ?? ''}</p>
			</div>
			<div class="flex gap-2">
				<Button onclick={loadProvider} variant="outline" size="sm" disabled={loading || saving}>
					<RefreshCw class="mr-2 h-4 w-4 {loading ? 'animate-spin' : ''}" />
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	<div class="max-w-3xl space-y-4">
		{#if loading && !provider}
			<div class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12">
				<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if provider}

			<div class="space-y-4 rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<FormSection title={m.idp_section_connection()} lead>
					<div class="grid gap-3 sm:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="editName">{m.idp_field_name()}</Label>
							<Input id="editName" disabled={!can(Permission.UPDATE_IDENTITY_PROVIDER)} bind:value={form.name} />
						</div>
						<div class="space-y-1.5">
							<Label for="idpSlugRead">{m.idp_field_slug()}</Label>
							<Input id="idpSlugRead" value={provider.slug} disabled class="font-mono" />
						</div>
					</div>
					<div class="space-y-1.5">
						<Label for="idpRedirect">{m.idp_field_redirect_uri()}</Label>
						<div class="flex items-center gap-2">
							<Input id="idpRedirect" value={redirectUri} readonly class="bg-sunken font-mono text-xs" />
							<Button
								variant="outline"
								size="icon"
								aria-label={m.idp_copy_redirect_uri()}
								onclick={() => copyToClipboard(redirectUri, m.idp_redirect_uri_copied())}
							>
								<Copy class="h-4 w-4" />
							</Button>
						</div>
						<p class="text-xs text-muted-foreground">{m.idp_field_redirect_uri_help()}</p>
					</div>
					<div class="grid gap-3 sm:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="editClientId">{m.idp_field_client_id()}</Label>
							<Input id="editClientId" disabled={!can(Permission.UPDATE_IDENTITY_PROVIDER)} bind:value={form.clientId} class="font-mono text-xs" />
						</div>

					</div>
					<div class="flex items-center justify-between gap-3">
						<Label for="editEnabled">{m.idp_field_enabled()}</Label>
						<Checkbox id="editEnabled" disabled={!can(Permission.UPDATE_IDENTITY_PROVIDER)} bind:checked={form.enabled} />
					</div>
				</FormSection>

				<FormSection title={m.idp_section_endpoints()}>

					<div class="space-y-1.5">
						<Label for="editIssuerUrl">{m.idp_field_issuer_url()}</Label>
						<Input id="editIssuerUrl" disabled={!can(Permission.UPDATE_IDENTITY_PROVIDER)} type="url" bind:value={form.issuerUrl} class="font-mono text-xs" />
						<p class="text-xs text-muted-foreground">{m.idp_field_issuer_url_help()}</p>
					</div>



					<div class="space-y-1.5">
						<Label for="editScopes">{m.idp_field_scopes()}</Label>
						<Input
							id="editScopes" disabled={!can(Permission.UPDATE_IDENTITY_PROVIDER)}
							bind:value={form.scopes}
							placeholder="openid, profile, email"
							class="font-mono text-xs"
						/>
						<p class="text-xs text-muted-foreground">{m.idp_field_scopes_help()}</p>
					</div>
				</FormSection>

			</div>
		{/if}
	</div>
</PageShell>
<ConfirmDeleteDialog bind:open={deleteDialogOpen} title={m.common_delete()} description={'Delete this identity provider?'} onconfirm={deleteProvider} />
