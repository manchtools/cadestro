

import { describe, expect, it, afterEach } from 'vitest';
import { handle } from './hooks.server';

const PAGE = '<meta name="cadestro-control-url" content="%cadestro.controlUrl%" />';

async function servedPage(controlUrl: string | undefined): Promise<string> {
	if (controlUrl === undefined) {
		delete process.env.PUBLIC_CONTROL_URL;
	} else {
		process.env.PUBLIC_CONTROL_URL = controlUrl;
	}
	const response = await (
		handle as unknown as (input: {
			event: unknown;
			resolve: (
				event: unknown,
				options?: { transformPageChunk?: (input: { html: string }) => string }
			) => Promise<Response>;
		}) => Promise<Response>
	)({
		event: {},
		resolve: async (_event, options) =>
			new Response(options?.transformPageChunk?.({ html: PAGE }) ?? PAGE)
	});
	return response.text();
}

afterEach(() => {
	delete process.env.PUBLIC_CONTROL_URL;
});

describe('control-origin injection', () => {
	it('hands the configured origin to the page', async () => {
		expect(await servedPage('https://manage.example.test')).toBe(
			'<meta name="cadestro-control-url" content="https://manage.example.test" />'
		);
	});

	it('leaves an empty value, not the placeholder, when nothing is configured', async () => {
		const page = await servedPage(undefined);

		expect(page).toBe('<meta name="cadestro-control-url" content="" />');
		expect(page).not.toContain('%cadestro');
	});

	it.each(['javascript:alert(1)', 'not a url', 'file:///etc/passwd'])(
		'refuses %s instead of putting it in the page',
		async (value) => {
			expect(await servedPage(value)).toBe('<meta name="cadestro-control-url" content="" />');
		}
	);

	it('escapes a value that would otherwise break out of the attribute', async () => {
		const page = await servedPage('https://example.test/"><script>alert(1)</script>');

		expect(page).not.toContain('<script>');
		expect(page).toContain('&quot;&gt;&lt;script&gt;');
	});

	it('still sets the security headers on the response', async () => {
		process.env.PUBLIC_CONTROL_URL = 'https://manage.example.test';
		const response = await (
			handle as unknown as (input: {
				event: unknown;
				resolve: (event: unknown) => Promise<Response>;
			}) => Promise<Response>
		)({ event: {}, resolve: async () => new Response('') });

		expect(response.headers.get('X-Frame-Options')).toBe('DENY');
		expect(response.headers.get('X-Content-Type-Options')).toBe('nosniff');
		expect(response.headers.get('Strict-Transport-Security')).toBe(
			'max-age=31536000; includeSubDomains'
		);
	});
});
