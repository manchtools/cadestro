import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const state = vi.hoisted(() => ({
	session: null as { accessToken: string; refreshToken: string; expiresAt: number } | null,
	refreshToken: vi.fn(),
	logout: vi.fn(),
	afterWrite: undefined as (() => void) | undefined,
	interceptor: undefined as ((next: (request: { header: Headers }) => Promise<unknown>) => (request: { header: Headers }) => Promise<unknown>) | undefined
}));

vi.mock('./session', () => ({
	clearSession: () => { state.session = null; },
	readSession: () => state.session,
	writeSession: (session: typeof state.session) => {
		state.session = session;
		state.afterWrite?.();
	}
}));

vi.mock('@connectrpc/connect-web', () => ({
	createConnectTransport: (options: { interceptors?: typeof state.interceptor[] }) => {
		if (options.interceptors?.[0]) state.interceptor = options.interceptors[0];
		return options;
	}
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
	const actual = await importOriginal<typeof import('@connectrpc/connect')>();
	return { ...actual, createClient: () => ({ refreshToken: state.refreshToken, logout: state.logout }) };
});

function deferred<T>() {
	let resolve!: (value: T) => void;
	let reject!: (reason: unknown) => void;
	const promise = new Promise<T>((resolvePromise, rejectPromise) => {
		resolve = resolvePromise;
		reject = rejectPromise;
	});
	return { promise, resolve, reject };
}

describe('authenticate interceptor', () => {
	beforeEach(async () => {
		vi.resetModules();
		state.session = { accessToken: 'old', refreshToken: 'refresh', expiresAt: Date.now() + 60_000 };
		state.refreshToken.mockReset();
		state.logout.mockReset();
		state.afterWrite = undefined;
		state.interceptor = undefined;
		vi.stubGlobal('document', { querySelector: () => null });
		vi.stubGlobal('window', { location: { origin: 'https://control.example' } });
		await import('./api');
	});

	it('refreshes once and retries an unauthenticated request with the renewed token', async () => {
		state.refreshToken.mockResolvedValue({ accessToken: 'new', refreshToken: 'next', expiresAt: { seconds: 2_000_000_000n, nanos: 0 } });
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
		const refreshError = new ConnectError('refresh failed', Code.Unavailable);
		state.refreshToken.mockRejectedValue(refreshError);
		const next = vi.fn().mockRejectedValue(new ConnectError('expired', Code.Unauthenticated));
		await expect(state.interceptor!(next)({ header: new Headers() })).rejects.toBe(refreshError);
		expect(next).toHaveBeenCalledTimes(1);
		expect(state.refreshToken).toHaveBeenCalledTimes(1);
		expect(state.session?.refreshToken).toBe('refresh');
	});

	it('clears only the rejected refresh session', async () => {
		state.refreshToken.mockRejectedValue(new ConnectError('revoked', Code.Unauthenticated));
		const next = vi.fn().mockRejectedValue(new ConnectError('expired', Code.Unauthenticated));
		const failure = state.interceptor!(next)({ header: new Headers() });
		await expect(failure).rejects.toMatchObject({ code: Code.Unauthenticated });
		expect(state.session).toBeNull();
	});

	it('clears the initiating session when refresh returns an invalid response', async () => {
		state.refreshToken.mockResolvedValue({ accessToken: 'new', refreshToken: 'next' });
		const next = vi.fn().mockRejectedValue(new ConnectError('expired', Code.Unauthenticated));
		const failure = state.interceptor!(next)({ header: new Headers() });
		await expect(failure).rejects.toMatchObject({ code: Code.Unauthenticated });
		expect(state.session).toBeNull();
	});

	it('coalesces concurrent refreshes for the same session', async () => {
		const refresh = deferred<{ accessToken: string; refreshToken: string; expiresAt: { seconds: bigint; nanos: number } }>();
		state.refreshToken.mockReturnValue(refresh.promise);
		const first = vi.fn().mockRejectedValueOnce(new ConnectError('expired', Code.Unauthenticated)).mockResolvedValueOnce('first');
		const second = vi.fn().mockRejectedValueOnce(new ConnectError('expired', Code.Unauthenticated)).mockResolvedValueOnce('second');
		const firstRequest = state.interceptor!(first)({ header: new Headers() });
		const secondRequest = state.interceptor!(second)({ header: new Headers() });
		await Promise.resolve();
		expect(state.refreshToken).toHaveBeenCalledTimes(1);
		refresh.resolve({ accessToken: 'renewed', refreshToken: 'rotated', expiresAt: { seconds: 2_000_000_000n, nanos: 0 } });
		await expect(Promise.all([firstRequest, secondRequest])).resolves.toEqual(['first', 'second']);
		expect(first).toHaveBeenCalledTimes(2);
		expect(second).toHaveBeenCalledTimes(2);
	});

	it('does not dispatch with a stale bearer after proactive refresh fails', async () => {
		state.session = { accessToken: 'old', refreshToken: 'refresh', expiresAt: Date.now() };
		const refreshError = new ConnectError('refresh failed', Code.Unavailable);
		state.refreshToken.mockRejectedValue(refreshError);
		const next = vi.fn();
		await expect(state.interceptor!(next)({ header: new Headers() })).rejects.toBe(refreshError);
		expect(next).not.toHaveBeenCalled();
		expect(state.refreshToken).toHaveBeenCalledTimes(1);
		expect(state.session?.refreshToken).toBe('refresh');
	});

	it('checks the refreshed session again before proactive dispatch', async () => {
		state.session = { accessToken: 'old', refreshToken: 'refresh', expiresAt: Date.now() };
		state.refreshToken.mockResolvedValue({ accessToken: 'renewed', refreshToken: 'rotated', expiresAt: { seconds: 2_000_000_000n, nanos: 0 } });
		state.afterWrite = () => {
			state.session = { accessToken: 'other-access', refreshToken: 'other-refresh', expiresAt: Date.now() + 60_000 };
		};
		const next = vi.fn();
		const failure = state.interceptor!(next)({ header: new Headers() });
		await expect(failure).rejects.toMatchObject({ code: Code.Unauthenticated });
		expect(next).not.toHaveBeenCalled();
		expect(state.session?.accessToken).toBe('other-access');
	});

	it('checks the refreshed session again before retrying a request', async () => {
		state.refreshToken.mockResolvedValue({ accessToken: 'renewed', refreshToken: 'rotated', expiresAt: { seconds: 2_000_000_000n, nanos: 0 } });
		state.afterWrite = () => {
			state.session = { accessToken: 'other-access', refreshToken: 'other-refresh', expiresAt: Date.now() + 60_000 };
		};
		const originalError = new ConnectError('expired', Code.Unauthenticated);
		const next = vi.fn().mockRejectedValueOnce(originalError).mockResolvedValueOnce('replayed');
		await expect(state.interceptor!(next)({ header: new Headers() })).rejects.toBe(originalError);
		expect(next).toHaveBeenCalledTimes(1);
		expect(state.session?.accessToken).toBe('other-access');
	});

	it('preserves a newer login when an older refresh is rejected', async () => {
		const refresh = deferred<never>();
		state.refreshToken.mockReturnValue(refresh.promise);
		const originalError = new ConnectError('expired', Code.Unauthenticated);
		const next = vi.fn().mockRejectedValue(originalError);
		const request = state.interceptor!(next)({ header: new Headers() });
		await Promise.resolve();
		expect(state.refreshToken).toHaveBeenCalledTimes(1);
		state.session = { accessToken: 'other-access', refreshToken: 'other-refresh', expiresAt: Date.now() + 60_000 };
		refresh.reject(new ConnectError('revoked', Code.Unauthenticated));
		await expect(request).rejects.toBe(originalError);
		expect(state.session?.accessToken).toBe('other-access');
	});

	it('does not restore a session after logout while refresh is in flight', async () => {
		const refresh = deferred<{ accessToken: string; refreshToken: string; expiresAt: { seconds: bigint; nanos: number } }>();
		const logoutResponse = deferred<object>();
		state.refreshToken.mockReturnValue(refresh.promise);
		state.logout.mockReturnValue(logoutResponse.promise);
		const next = vi.fn().mockRejectedValue(new ConnectError('expired', Code.Unauthenticated));
		const request = state.interceptor!(next)({ header: new Headers() });
		await Promise.resolve();
		expect(state.refreshToken).toHaveBeenCalledTimes(1);
		const { logout } = await import('./api');
		const signingOut = logout();
		expect(state.session).toBeNull();
		refresh.resolve({ accessToken: 'renewed', refreshToken: 'rotated', expiresAt: { seconds: 2_000_000_000n, nanos: 0 } });
		await expect(request).rejects.toThrow('expired');
		expect(state.session).toBeNull();
		logoutResponse.resolve({});
		await signingOut;
	});

	it('does not overwrite a newer login with an older refresh response', async () => {
		const refresh = deferred<{ accessToken: string; refreshToken: string; expiresAt: { seconds: bigint; nanos: number } }>();
		state.refreshToken.mockReturnValue(refresh.promise);
		const next = vi.fn().mockRejectedValue(new ConnectError('expired', Code.Unauthenticated));
		const request = state.interceptor!(next)({ header: new Headers() });
		await Promise.resolve();
		expect(state.refreshToken).toHaveBeenCalledTimes(1);
		state.session = { accessToken: 'other-access', refreshToken: 'other-refresh', expiresAt: Date.now() + 60_000 };
		refresh.resolve({ accessToken: 'renewed', refreshToken: 'rotated', expiresAt: { seconds: 2_000_000_000n, nanos: 0 } });
		await expect(request).rejects.toThrow('expired');
		expect(state.session?.accessToken).toBe('other-access');
	});

	it('does not replay an old request under a replacement session', async () => {
		const response = deferred<never>();
		state.refreshToken.mockResolvedValue({ accessToken: 'renewed', refreshToken: 'rotated', expiresAt: { seconds: 2_000_000_000n, nanos: 0 } });
		const next = vi.fn().mockImplementationOnce(() => response.promise).mockResolvedValueOnce('replayed');
		const request = state.interceptor!(next)({ header: new Headers() });
		state.session = { accessToken: 'other-access', refreshToken: 'other-refresh', expiresAt: Date.now() + 60_000 };
		response.reject(new ConnectError('expired', Code.Unauthenticated));
		await expect(request).rejects.toThrow('expired');
		expect(next).toHaveBeenCalledTimes(1);
		expect(state.refreshToken).not.toHaveBeenCalled();
	});

	it('does not let a delayed logout clear a newer login', async () => {
		const logoutResponse = deferred<object>();
		state.logout.mockReturnValue(logoutResponse.promise);
		const { logout } = await import('./api');
		const signingOut = logout();
		expect(state.session).toBeNull();
		state.session = { accessToken: 'other-access', refreshToken: 'other-refresh', expiresAt: Date.now() + 60_000 };
		logoutResponse.resolve({});
		await signingOut;
		expect(state.session?.accessToken).toBe('other-access');
	});
});
