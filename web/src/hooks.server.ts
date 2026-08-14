import type { Handle } from '@sveltejs/kit';

// The control origin the deployment configured, handed to the browser through
// the placeholder in app.html. It is read from the process environment rather
// than through `$env/dynamic/public`, deliberately: that module is a build
// artifact of the SvelteKit plugin, and pulling it into the client's module
// graph breaks module mocking in the browser test suite. Injecting the value
// here keeps it a runtime value — one image, any domain — while the client
// stays free of framework virtual modules.
//
// The name keeps its PUBLIC_ prefix because the value genuinely is public: it
// is embedded in every page this server sends.
//
// Validated, not trusted: an unset variable, a value that is not an http(s)
// origin, or a placeholder that somehow survived unsubstituted all resolve to
// the empty string, which the app treats as "not configured".
function controlUrl(): string {
	const raw = (process.env.PUBLIC_CONTROL_URL ?? '').trim().replace(/\/+$/, '');
	if (!raw) return '';
	try {
		const parsed = new URL(raw);
		if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return '';
	} catch {
		return '';
	}
	return raw;
}

// Attribute-escaped: the value comes from the operator's deployment, and it is
// written into an HTML attribute.
function escapeAttribute(value: string): string {
	return value
		.replaceAll('&', '&amp;')
		.replaceAll('<', '&lt;')
		.replaceAll('>', '&gt;')
		.replaceAll('"', '&quot;');
}

const CONTROL_URL_PLACEHOLDER = '%cadestro.controlUrl%';

export const handle: Handle = async ({ event, resolve }) => {
	const injected = escapeAttribute(controlUrl());
	const response = await resolve(event, {
		// The replacement is a function so the substituted text is taken
		// literally: as a plain string, `$&` and friends inside it would be
		// expanded by String.replace.
		transformPageChunk: ({ html }) => html.replace(CONTROL_URL_PLACEHOLDER, () => injected)
	});

	response.headers.set('X-Frame-Options', 'DENY');
	response.headers.set('X-Content-Type-Options', 'nosniff');
	response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
	response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=()');
	response.headers.set('Strict-Transport-Security', 'max-age=31536000; includeSubDomains');

	return response;
};
