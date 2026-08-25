import { describe, expect, it } from 'vitest';
import { ApiClient } from './client';

describe('createToken keeps the CA fingerprint pin beside the token', () => {
	it('returns the full response, token and pin together', async () => {
		const pin = 'ab'.repeat(32);
		const stubClient = {
			createToken: async () => ({
				token: { id: '01ARZ3NDEKTSV4RRFFQ69G5FAV', value: 'TOK-SECRET' },
				caFingerprintPin: pin
			})
		};

		const createToken = ApiClient.prototype.createToken as unknown as (
			this: { getClient: () => typeof stubClient },
			name: string,
			maxUses: number,
			expiresAt?: Date
		) => ReturnType<ApiClient['createToken']>;
		const result = await createToken.call({ getClient: () => stubClient }, 'first-device', 0, new Date());
		expect(result.token?.value).toBe('TOK-SECRET');
		expect(result.caFingerprintPin).toBe(pin);
	});
});
