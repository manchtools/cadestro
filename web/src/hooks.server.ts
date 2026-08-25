import type { Handle } from '@sveltejs/kit';

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

		transformPageChunk: ({ html }) => html.replace(CONTROL_URL_PLACEHOLDER, () => injected)
	});

	response.headers.set('X-Frame-Options', 'DENY');
	response.headers.set('X-Content-Type-Options', 'nosniff');
	response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
	response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=()');
	response.headers.set('Strict-Transport-Security', 'max-age=31536000; includeSubDomains');

	return response;
};
