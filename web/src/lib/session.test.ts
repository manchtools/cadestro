import { describe, expect, it } from 'vitest';
import { readSession } from './session';

describe('readSession', () => {
	it('accepts only nonempty tokens and a finite expiration time', () => {
		let raw = JSON.stringify({ accessToken: 'access', refreshToken: 'refresh', expiresAt: 123 });
		const storage = {
			getItem: () => raw,
			setItem: () => {},
			removeItem: () => {},
			clear: () => {},
			key: () => null,
			length: 1
		} satisfies Storage;

		expect(readSession(storage)).toEqual({ accessToken: 'access', refreshToken: 'refresh', expiresAt: 123 });

		for (const value of [
			{ accessToken: '', refreshToken: 'refresh', expiresAt: 123 },
			{ accessToken: 'access', refreshToken: '', expiresAt: 123 },
			{ accessToken: {}, refreshToken: 'refresh', expiresAt: 123 },
			{ accessToken: 'access', refreshToken: {}, expiresAt: 123 }
		]) {
			raw = JSON.stringify(value);
			expect(readSession(storage)).toBeNull();
		}
		raw = '{"accessToken":"access","refreshToken":"refresh","expiresAt":1e400}';
		expect(readSession(storage)).toBeNull();
	});
});
