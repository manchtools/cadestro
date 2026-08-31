import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const state = vi.hoisted(() => ({
	session: null as { accessToken: string; refreshToken: string; expiresAt: number } | null,
	refreshToken: vi.fn(),
	interceptor: undefined as ((next: (request: { header: Headers }) => Promise<unknown>) => (request: { header: Headers }) => Promise<unknown>) | undefined
}));

vi.mock('./session', () => ({
	clearSession: vi.fn(),
	readSession: () => state.session,
	writeSession: (session: typeof state.session) => { state.session = session; }
}));

vi.mock('@connectrpc/connect-web', () => ({
	createConnectTransport: (options: { interceptors?: typeof state.interceptor[] }) => {
		if (options.interceptors?.[0]) state.interceptor = options.interceptors[0];
		return options;
	}
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
	const actual = await importOriginal<typeof import('@connectrpc/connect')>();
	return { ...actual, createClient: () => ({ refreshToken: state.refreshToken }) };
});

describe('authenticate interceptor', () => {
	beforeEach(async () => {
		vi.resetModules();
		state.session = { accessToken: 'old', refreshToken: 'refresh', expiresAt: Date.now() + 60_000 };
		state.refreshToken.mockReset();
		state.interceptor = undefined;
		vi.stubGlobal('document', { querySelector: () => null });
		vi.stubGlobal('window', { location: { origin: 'https://control.example' } });
		await import('./api');
	});

	it('refreshes once and retries an unauthenticated request with the renewed token', async () => {
		state.refreshToken.mockResolvedValue({ accessToken: 'new', refreshToken: 'next', expiresAt: { seconds: 1n, nanos: 0 } });
		const authorizations: string[] = [];
		const next = vi.fn(async (value: { header: Headers }) => {
			authorizations.push(value.header.get('Authorization') ?? '');
			if (authorizations.length === 1) throw new ConnectError('expired', Code.Unauthenticated);
			return 'ok';
		});
		const request = { header: new Headers() };
		await expect(state.interceptor!(next)(request)).resolves.toBe('ok');
		expect(next).toHaveBeenCalledTimes(2);
		expect(authorizations).toEqual(['Bearer old', 'Bearer new']);
		expect(state.refreshToken).toHaveBeenCalledTimes(1);
	});

	it('does not retry when refresh fails', async () => {
		state.refreshToken.mockRejectedValue(new Error('refresh failed'));
		const next = vi.fn().mockRejectedValue(new ConnectError('expired', Code.Unauthenticated));
		await expect(state.interceptor!(next)({ header: new Headers() })).rejects.toThrow('expired');
		expect(next).toHaveBeenCalledTimes(1);
	});
});
