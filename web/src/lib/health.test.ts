import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchHealth } from './health';

describe('fetchHealth', () => {
	afterEach(() => vi.restoreAllMocks());

	it('returns the version from a healthy response', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"version":"v1"}', { status: 200 })));

		await expect(fetchHealth('https://control.test')).resolves.toMatchObject({ version: 'v1' });
	});

	it('keeps non-OK and malformed responses usable without a version', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('not-json', { status: 503 })));
		await expect(fetchHealth('https://control.test')).resolves.toMatchObject({ version: null });

		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"version":7}', { status: 200 })));
		await expect(fetchHealth('https://control.test')).resolves.toMatchObject({ version: null });
	});
});
