// prefers-reduced-motion, behind a stubbable seam.
//
// The media query is read through a mutable holder so tests can force either
// answer deterministically — Playwright's media emulation is not reachable from
// the component under test, and an untested reduced-motion path is exactly the
// kind of accessibility promise that silently rots.

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
