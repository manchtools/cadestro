<script lang="ts">

	import { authStore, configStore } from '$lib/sdk';
	import { onboardingScope } from '$lib/onboarding/storage';
	import { onboarding, initOnboarding, startTour, closeWelcome } from '$lib/onboarding/tour.svelte';
	import WelcomeDialog from './welcome-dialog.svelte';
	import TourOverlay from './tour-overlay.svelte';

	$effect(() => {
		const user = authStore.user;
		if (!user) return;
		initOnboarding(onboardingScope(configStore.serverUrl, (user.id?.value ?? '')));
	});
</script>

{#if onboarding.welcomeOpen}
	<WelcomeDialog onstart={() => startTour()} ondismiss={closeWelcome} />
{/if}

<TourOverlay />

<div class="sr-only" role="status" aria-live="polite" data-testid="onboarding-live">
	{onboarding.announcement}
</div>
