

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

	it('normalises a trailing slash the way the setup form does', async () => {
		const { configStore } = await loadSdkServedWith('https://manage.example.test/');

		expect(configStore.serverUrl).toBe('https://manage.example.test');
	});

	it('leaves the app unconfigured when the page carries no origin', async () => {
		const { configStore } = await loadSdkServedWith(null);

		expect(configStore.isConfigured).toBe(false);
		expect(configStore.serverUrl).toBe('');
	});

	it('treats a blank value as no origin at all', async () => {
		const { configStore } = await loadSdkServedWith('   ');

		expect(configStore.isConfigured).toBe(false);
	});
});
