const COOKIE_NAME = 'pm-version';
const LOOP_KEY = 'pm-version-attempted';
// Build-time base path injected via vite.config.ts (matches BASE_PATH env var
// passed to the build, defaulting to "/"). The cookie scope MUST match the
// SvelteKit base path so reload triggers Traefik routing on the same prefix.
const COOKIE_PATH = __BASE_PATH__;

export function getVersionCookie(): string | null {
	const match = document.cookie.match(new RegExp(`(?:^|;\\s*)${COOKIE_NAME}=([^;]*)`));
	return match ? decodeURIComponent(match[1]) : null;
}

export function setVersionCookie(version: string) {
	document.cookie = `${COOKIE_NAME}=${encodeURIComponent(version)}; path=${COOKIE_PATH}; max-age=31536000; SameSite=Lax`;
}

export function clearVersionCookie() {
	document.cookie = `${COOKIE_NAME}=; path=${COOKIE_PATH}; max-age=0; SameSite=Lax`;
}

/**
 * Check if the server version differs from the current app version.
 * If so, set the version cookie and reload. Returns true if redirecting.
 * Includes loop prevention via sessionStorage.
 */
export async function checkAndSwitchVersion(serverUrl: string): Promise<boolean> {
	try {
		const res = await fetch(`${serverUrl}/health`, { signal: AbortSignal.timeout(3000) });
		if (!res.ok) return false;
		const data = await res.json();
		const serverVersion = data.version;

		if (!serverVersion || serverVersion === 'dev' || serverVersion === __APP_VERSION__) {
			sessionStorage.removeItem(LOOP_KEY);
			return false;
		}

		// Version mismatch — check if we already tried this switch
		const attempted = sessionStorage.getItem(LOOP_KEY);
		if (attempted === serverVersion) {
			// Already tried, container doesn't exist — stay on current version
			return false;
		}

		// Switch to the matching version
		sessionStorage.setItem(LOOP_KEY, serverVersion);
		await switchVersion(serverVersion);
		return true;
	} catch {
		return false;
	}
}

/**
 * Clear service worker + caches, set version cookie, and reload.
 * This ensures Traefik routes to the correct container on reload.
 */
export async function switchVersion(version: string) {
	// Unregister all service workers so cached HTML doesn't interfere
	if ('serviceWorker' in navigator) {
		const registrations = await navigator.serviceWorker.getRegistrations();
		for (const reg of registrations) {
			await reg.unregister();
		}
	}
	// Clear all caches
	if ('caches' in window) {
		const cacheNames = await caches.keys();
		for (const name of cacheNames) {
			await caches.delete(name);
		}
	}
	// Set cookie and reload
	setVersionCookie(version);
	window.location.reload();
}
