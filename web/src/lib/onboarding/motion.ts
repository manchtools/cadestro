

export const motion = {
	reduced(): boolean {
		if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
		try {
			return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		} catch (err) {
			console.debug('onboarding: prefers-reduced-motion query failed', err);
			return false;
		}
	}
};
