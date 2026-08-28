import adapter from 'svelte-adapter-bun';

export default {
	kit: {
		adapter: adapter(),
		alias: { $contract: '../contract/gen/ts' },
		csp: {
			directives: {
				'default-src': ['self'],
				'script-src': ['self'],
				'style-src': ['self', 'unsafe-inline'],
				'connect-src': ['self', 'https:'],
				'img-src': ['self', 'data:'],
				'frame-ancestors': ['none']
			}
		}
	}
};
