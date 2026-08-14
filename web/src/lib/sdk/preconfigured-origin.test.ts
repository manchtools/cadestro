// The deployment serves this app beside control on one hostname and writes that
// origin into the page it sends, so a fresh install must reach a working UI
// without anyone answering /setup. The seed is what makes that true, and it has
// to stay a seed: it may not overrule an operator who pointed this browser
// elsewhere, and its absence must leave the old behaviour exactly as it was.
//
// Each case imports the module tree fresh, because the seed runs once at module
// scope — that is the point, since the route guards read `isConfigured` inside
// `load`, after the import and before anything renders. The seam is the meta tag
// app.html declares and hooks.server.ts fills; that the deployment actually puts
// the value into the container is asserted where it is decided, in the deploy
// tests against the resolved Compose configuration.
import { describe, expect, it, vi, afterEach } from 'vitest';

function documentServing(content: string | null) {
	return {
		querySelector: (selector: string) =>
			selector === 'meta[name="cadestro-control-url"]' && content !== null
				? { getAttribute: () => content }
				: null
	};
}

afterEach(() => {
	vi.unstubAllGlobals();
	vi.resetModules();
});

async function loadSdkServedWith(content: string | null) {
	vi.resetModules();
	vi.stubGlobal('document', documentServing(content));
	return import('$lib/sdk');
}

describe('preconfigured control origin', () => {
	it('seeds the origin the page was served with, so the first load is already configured', async () => {
		const { configStore } = await loadSdkServedWith('https://manage.example.test');

		expect(configStore.isConfigured).toBe(true);
		expect(configStore.serverUrl).toBe('https://manage.example.test');
	});

	// Every RPC path is built by joining onto this value, and /setup normalises
	// what a human types the same way. A trailing slash arriving from the
	// deployment must not produce a different base than the same URL typed in.
	it('normalises a trailing slash the way the setup form does', async () => {
		const { configStore } = await loadSdkServedWith('https://manage.example.test/');

		expect(configStore.serverUrl).toBe('https://manage.example.test');
	});

	// The negative control: with no tag in the page nothing is invented, so a UI
	// deployed away from its control server still gets asked where that is. This
	// is also every component test's situation.
	it('leaves the app unconfigured when the page carries no origin', async () => {
		const { configStore } = await loadSdkServedWith(null);

		expect(configStore.isConfigured).toBe(false);
		expect(configStore.serverUrl).toBe('');
	});

	// A tag with an empty or blank value is a deployment that configured nothing.
	// It must behave like the missing tag rather than configure the app with an
	// origin that cannot be reached.
	it('treats a blank value as no origin at all', async () => {
		const { configStore } = await loadSdkServedWith('   ');

		expect(configStore.isConfigured).toBe(false);
	});
});
