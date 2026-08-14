import adapter from 'svelte-adapter-bun';

// Derive the marketplace origin at build time from the same env var
// the web client uses. Allowed in frame-src so the /marketplace route
// can embed the marketplace UI; everything else stays same-origin.
const marketplaceOrigin = (() => {
	const raw = process.env.PUBLIC_MARKETPLACE_URL || 'https://marketplace.cadestro.manchtools.com';
	try {
		return new URL(raw).origin;
	} catch {
		return '';
	}
})();

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter(),
		csp: {
			directives: {
				'default-src': ['self'],
				// 'unsafe-inline' is a CSP Level 1 fallback: when SvelteKit
				// emits a nonce (prod, CSP Level 2+ browsers) the browser
				// ignores 'unsafe-inline' automatically and only trusts
				// nonced scripts. It matters during dev where Vite and
				// SvelteKit inject un-nonced inline scripts.
				'script-src': ['self', 'unsafe-inline'],
				'style-src': ['self', 'unsafe-inline'],
				'connect-src': ['self', 'https:', 'wss:'],
				'img-src': ['self', 'data:'],
				'font-src': ['self'],
				'frame-src': marketplaceOrigin ? ['self', marketplaceOrigin] : ['self'],
				'frame-ancestors': ['none']
			}
		},
		paths: {
			base: process.env.BASE_PATH || ''
		},
		alias: {
			$contract: '../contract/gen/ts',
			$contractClient: '../contract/ts'
		}
	}
};

export default config;
