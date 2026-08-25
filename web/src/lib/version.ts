const COOKIE_NAME = 'cadestro-version';
const LOOP_KEY = 'cadestro-version-attempted';

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

		const attempted = sessionStorage.getItem(LOOP_KEY);
		if (attempted === serverVersion) {

			return false;
		}

		sessionStorage.setItem(LOOP_KEY, serverVersion);
		await switchVersion(serverVersion);
		return true;
	} catch {
		return false;
	}
}

export async function switchVersion(version: string) {

	if ('serviceWorker' in navigator) {
		const registrations = await navigator.serviceWorker.getRegistrations();
		for (const reg of registrations) {
			await reg.unregister();
		}
	}

	if ('caches' in window) {
		const cacheNames = await caches.keys();
		for (const name of cacheNames) {
			await caches.delete(name);
		}
	}

	setVersionCookie(version);
	window.location.reload();
}
