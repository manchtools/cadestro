// Tests for the BASE_PATH-aware navigation wrapper (F004).

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// vi.mock factories are hoisted to the top of the file; vi.hoisted lets
// us share state between the factory and the test.
const navMocks = vi.hoisted(() => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn()
}));

vi.mock('$app/paths', () => ({
	base: '/app'
}));

vi.mock('$app/navigation', () => navMocks);

import { goto, pushState, replaceState } from './navigation';

describe('goto', () => {
	beforeEach(() => navMocks.goto.mockReset());

	it('prepends base to a relative path', async () => {
		await goto('/devices');
		expect(navMocks.goto).toHaveBeenCalledWith('/app/devices', undefined);
	});

	it('forwards options', async () => {
		await goto('/devices', { replaceState: true });
		expect(navMocks.goto).toHaveBeenCalledWith('/app/devices', { replaceState: true });
	});
});

describe('pushState', () => {
	beforeEach(() => navMocks.pushState.mockReset());

	it('prepends base to the URL', () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		pushState('/devices/abc', { actionSheet: { actionId: 'x' } } as any);
		expect(navMocks.pushState).toHaveBeenCalledWith('/app/devices/abc', { actionSheet: { actionId: 'x' } });
	});
});

describe('replaceState', () => {
	beforeEach(() => navMocks.replaceState.mockReset());

	it('prepends base', () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		replaceState('/devices', {} as any);
		expect(navMocks.replaceState).toHaveBeenCalledWith('/app/devices', {});
	});
});

afterEach(() => {
	vi.clearAllMocks();
});
