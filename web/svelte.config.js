import adapter from 'svelte-adapter-bun';

const marketplaceOrigin = (() => {
	const raw = process.env.PUBLIC_MARKETPLACE_URL || 'https://marketplace.cadestro.manchtools.com';
	try {
		return new URL(raw).origin;
	} catch {
		return '';
	}
})();

const config = {
	kit: {
		adapter: adapter(),
		csp: {
			directives: {
				'default-src': ['self'],

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
