// Tests for the localized-error pipeline (F004 + F005).
//
// Every call site in the app feeds caught errors through
// `getLocalizedError`. If a code lands in `errorMessages` we want a
// stable, localized string back; otherwise we fall back to the original
// error message and append the request ID.

import { describe, it, expect, vi } from 'vitest';

// Mock the SDK's getErrorCode / getRequestId so we can drive arbitrary
// shapes through getLocalizedError without needing a real ConnectError.
vi.mock('$pmSdk/client', () => ({
	getErrorCode: (e: unknown) => {
		if (e && typeof e === 'object' && 'code' in e) {
			return (e as { code: string }).code;
		}
		return null;
	},
	getRequestId: (e: unknown) => {
		if (e && typeof e === 'object' && 'requestId' in e) {
			return (e as { requestId: string }).requestId;
		}
		return null;
	}
}));

vi.mock('$pmSdk/errors', () => ({}));

import { getLocalizedError } from './errors';

describe('getLocalizedError', () => {
	it('returns a localized message for a known code', () => {
		const msg = getLocalizedError({ code: 'user_not_found' });
		expect(typeof msg).toBe('string');
		expect(msg.length).toBeGreaterThan(0);
		// `user_not_found` is a user-facing code — must NOT have a Request ID
		// suffix (see userFacingCodes set in errors.ts).
		expect(msg).not.toMatch(/Request ID/);
	});

	it('appends a Request ID for non-user-facing codes', () => {
		const msg = getLocalizedError({
			code: 'role_in_use',
			requestId: 'req-123'
		});
		expect(msg).toMatch(/Request ID: req-123/);
	});

	it('falls back to the Error.message when no code is present', () => {
		const msg = getLocalizedError(new Error('connection refused'));
		expect(msg).toMatch(/connection refused/);
	});

	it('uses an internal-error label for non-Error, non-coded values', () => {
		const msg = getLocalizedError(42);
		expect(typeof msg).toBe('string');
		expect(msg.length).toBeGreaterThan(0);
	});
});
