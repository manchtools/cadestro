<script lang="ts">
	// The onboarding host: mounted once in the (app) layout, above the router
	// outlet, so the welcome, the tour and the live region all survive
	// navigation the way the rest of the shell does.
	//
	// The getting-started checklist is NOT here: it belongs to the empty fleet
	// and is rendered by the devices route through fleet-empty's `extra` slot.
	import { authStore, configStore } from '$lib/sdk';
	import { onboardingScope } from '$lib/onboarding/storage';
	import { onboarding, initOnboarding, startTour, closeWelcome } from '$lib/onboarding/tour.svelte';
	import WelcomeDialog from './welcome-dialog.svelte';
	import TourOverlay from './tour-overlay.svelte';

	// Onboarding state is per (server, user). Waiting for the session means a
	// signed-in operator can never inherit the "anonymous" scope's flags.
	$effect(() => {
		const user = authStore.user;
		if (!user) return;
		initOnboarding(onboardingScope(configStore.serverUrl, user.id));
	});
</script>

{#if onboarding.welcomeOpen}
	<WelcomeDialog onstart={() => startTour()} ondismiss={closeWelcome} />
{/if}

<TourOverlay />

<!-- Step changes and tour endings are announced politely; the region outlives
     the overlay so "you can restart it from Settings" is still read out. -->
<div class="sr-only" role="status" aria-live="polite" data-testid="onboarding-live">
	{onboarding.announcement}
</div>
