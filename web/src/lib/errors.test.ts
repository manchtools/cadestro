

import { describe, it, expect, vi } from 'vitest';
import { ErrorCode } from '$contract/cadestro/v1/common_pb';

vi.mock('$contractClient/client', () => ({
	getErrorCode: (e: unknown) => {
		if (e && typeof e === 'object' && 'code' in e) {
			return (e as { code: ErrorCode }).code;
		}
		return undefined;
	},
	getRequestId: (e: unknown) => {
		if (e && typeof e === 'object' && 'requestId' in e) {
			return (e as { requestId: string }).requestId;
		}
		return null;
	}
}));

vi.mock('$contractClient/errors', () => ({}));

import { getLocalizedError } from './errors';

describe('getLocalizedError', () => {
	it('returns a localized message for a known code', () => {
		const msg = getLocalizedError({ code: ErrorCode.USER_NOT_FOUND });
		expect(typeof msg).toBe('string');
		expect(msg.length).toBeGreaterThan(0);

		expect(msg).not.toMatch(/Request ID/);
	});

	it('appends a Request ID for non-user-facing codes', () => {
		const msg = getLocalizedError({
			code: ErrorCode.ROLE_IN_USE,
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
