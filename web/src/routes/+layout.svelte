<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { base } from '$app/paths';
	import { pwaInfo } from 'virtual:pwa-info';
	import { Toaster } from '$lib/components/ui/sonner';
	import { toast } from 'svelte-sonner';
	 import { ModeWatcher } from "mode-watcher";
	import * as m from '$lib/paraglide/messages';

	let { children } = $props();

	onMount(async () => {
		if (pwaInfo) {
			const { registerSW } = await import('virtual:pwa-register');
			registerSW({
				immediate: true,
				onRegisteredSW(swUrl, r) {
					if (import.meta.env.DEV) {
						console.log(`Service worker registered: ${swUrl}`);
					}
				},
				onOfflineReady() {
					toast.success(m.pwa_offline_ready());
				},
				onNeedRefresh() {
					toast.info(m.pwa_update_available(), {
						action: {
							label: m.pwa_refresh(),
							onClick: () => window.location.reload()
						},
						duration: Infinity
					});
				}
			});
		}
	});

	const webManifestHref = $derived(pwaInfo ? pwaInfo.webManifest.href : null);
</script>

<svelte:head>
	<!-- Favicons + theme-color live in app.html with media-queried
	     light/dark variants. The PWA-specific tags below stay here
	     because they need to be present on every route. -->
	<meta name="mobile-web-app-capable" content="yes" />
	<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
	<meta name="apple-mobile-web-app-title" content="Cadestro" />
	{#if webManifestHref}
		<link rel="manifest" href={webManifestHref} />
	{/if}
</svelte:head>
<ModeWatcher />
{@render children()}
<Toaster richColors position="bottom-right" />
