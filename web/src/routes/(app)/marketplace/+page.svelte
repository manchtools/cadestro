<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { isEmbedMessage, originOf } from '$lib/marketplace/embed';
	import { importTemplate, ImportError, type Template, type TemplateType } from '$lib/marketplace/import';
	import * as m from '$lib/paraglide/messages';
	import { Store } from '@lucide/svelte';

	// The marketplace is a standalone hosted app; we embed its /embed
	// route in an iframe, talk via postMessage, and let it own the UI.
	// The only thing that happens on this side is the import handoff
	// back into the control server via the existing apiClient.

	const marketplaceUrl = __MARKETPLACE_URL__;
	const marketplaceOrigin = originOf(marketplaceUrl);
	const embedSrc = marketplaceUrl.replace(/\/$/, '') + '/embed';

	let iframeRef: HTMLIFrameElement | null = $state(null);
	let ready = $state(false);
	let importing = $state(false);
	let loadError = $state<string | null>(null);
	let readyTimer: ReturnType<typeof setTimeout> | null = null;

	function onMessage(event: MessageEvent) {
		// Strict origin + source checks. Only messages from the iframe
		// we mounted, from the marketplace origin, count. Ignore
		// anything else silently — other iframes / extensions may
		// post messages to the window.
		if (!marketplaceOrigin || event.origin !== marketplaceOrigin) return;
		if (iframeRef === null || event.source !== iframeRef.contentWindow) return;
		if (!isEmbedMessage(event.data)) return;

		const msg = event.data;
		if (msg.kind === 'pm.marketplace.hello') {
			// Reverse handshake: the embed signals that its message
			// listener is attached. Responding here (instead of at
			// iframe-load) eliminates the race between the iframe's
			// load event and Svelte hydration inside the iframe.
			sendInit();
			return;
		}
		if (msg.kind === 'pm.marketplace.ready') {
			ready = true;
			loadError = null;
			if (readyTimer !== null) {
				clearTimeout(readyTimer);
				readyTimer = null;
			}
			return;
		}
		if (msg.kind === 'pm.marketplace.close') {
			// User clicked "close" inside the iframe — take them back
			// to the actions list for now; a proper host flow would
			// dismiss a modal.
			goto('/actions');
			return;
		}
		if (msg.kind === 'pm.marketplace.import') {
			void doImport({
				id: msg.templateId,
				name: msg.templateName,
				templateType: msg.templateType as TemplateType,
				content: msg.content
			});
		}
	}

	async function doImport(template: Template) {
		importing = true;
		try {
			const result = await importTemplate(template);
			toast.success(m.marketplace_import_success({ name: result.name }));
			goto(result.redirect);
		} catch (err) {
			if (err instanceof ImportError) {
				toast.error(m.marketplace_import_failed({ message: err.message }));
			} else {
				toast.error(m.marketplace_import_failed({ message: err instanceof Error ? err.message : String(err) }));
			}
		} finally {
			importing = false;
		}
	}

	function sendInit() {
		if (iframeRef === null || iframeRef.contentWindow === null) return;
		// If the iframe failed to load (CSP block, DNS miss, 404) the
		// browser serves an internal error page whose origin is 'null'
		// or 'chrome-error://...'. postMessage against that target
		// origin silently no-ops — we can't tell synchronously. Arm
		// a readiness timeout and surface an explicit error so the
		// operator sees something actionable instead of a blank page.
		try {
			iframeRef.contentWindow.postMessage(
				{
					kind: 'pm.marketplace.init',
					// No subscription token — the marketplace app owns
					// authentication on its own domain. Browsers send the
					// marketplace's own session cookie with the iframe
					// request. Passed as null here so the contract stays
					// forward-compatible with deployments that want to
					// pre-seed a token.
					subscriptionToken: null,
					parentOrigin: window.location.origin
				},
				marketplaceOrigin
			);
		} catch (err) {
			loadError = err instanceof Error ? err.message : String(err);
			return;
		}

		if (readyTimer !== null) clearTimeout(readyTimer);
		readyTimer = setTimeout(() => {
			if (!ready) {
				loadError = m.marketplace_embed_no_response();
			}
		}, 4000);
	}

	onMount(() => {
		window.addEventListener('message', onMessage);
		// The embed drives the handshake by posting pm.marketplace.hello
		// once its own message listener is attached (see the /embed
		// route in the marketplace repo). No iframe onload wiring
		// needed — and keeping the iframe element free of inline
		// handlers also avoids the CSP 'inline event handler' rule.
	});

	onDestroy(() => {
		if (readyTimer !== null) {
			clearTimeout(readyTimer);
			readyTimer = null;
		}
		if (typeof window !== 'undefined') {
			window.removeEventListener('message', onMessage);
		}
	});
</script>

{#if !marketplaceOrigin}
	<div class="p-6">
		<div class="rounded border border-destructive/50 bg-destructive/10 p-4 text-sm">
			<p class="font-semibold">{m.marketplace_misconfigured_title()}</p>
			<p class="text-muted-foreground mt-1">{m.marketplace_misconfigured_body()}</p>
		</div>
	</div>
{:else}
	<div class="flex flex-col h-full">
		<header class="flex items-center gap-3 border-b px-6 py-3">
			<div class="flex h-9 w-9 items-center justify-center rounded-md bg-primary/10">
				<Store class="h-4 w-4 text-primary" />
			</div>
			<div class="flex-1">
				<h1 class="truncate text-2xl font-bold">{m.marketplace_title()}</h1>
				<p class="text-xs text-muted-foreground">{m.marketplace_subtitle()}</p>
			</div>
			{#if importing}
				<span class="text-xs text-muted-foreground">{m.marketplace_importing()}</span>
			{/if}
		</header>

		<div class="flex-1 relative">
			{#if loadError}
				<div class="absolute inset-0 flex items-center justify-center p-6">
					<div class="max-w-lg rounded border border-destructive/50 bg-destructive/10 p-4 text-sm">
						<p class="font-semibold">{m.marketplace_misconfigured_title()}</p>
						<p class="text-muted-foreground mt-1">{loadError}</p>
					</div>
				</div>
			{:else if !ready}
				<div class="absolute inset-0 flex items-center justify-center text-sm text-muted-foreground">
					{m.marketplace_embed_loading()}
				</div>
			{/if}
			<iframe
				bind:this={iframeRef}
				title={m.marketplace_title()}
				src={embedSrc}
				class="absolute inset-0 h-full w-full border-0 bg-background"
				class:invisible={loadError !== null}
				sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
				referrerpolicy="strict-origin"
			></iframe>
		</div>
	</div>
{/if}
