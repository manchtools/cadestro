import { create, toJson } from '@bufbuild/protobuf';
import { RoleGrantScopeKind } from '../gen/ts/cadestro/v1/common_pb';
import { RoleGrantSchema, RoleSchema, UserSchema } from '../gen/ts/cadestro/v1/control_pb';
import { parseAuth, serializeAuth, type StoredAuth } from './auth.js';
import { describe, expect, it } from 'vitest';

class MemoryStorage {
	private values = new Map<string, string>();

	getItem(key: string): string | null {
		return this.values.get(key) ?? null;
	}

	setItem(key: string, value: string): void {
		this.values.set(key, value);
	}

	removeItem(key: string): void {
		this.values.delete(key);
	}
}

function fixture(): StoredAuth {
	const role = create(RoleSchema, {
		id: { value: 'role-1' },
		name: 'Administrator',
		permissions: ['ListUsers']
	});
	const user = create(UserSchema, {
		id: { value: 'user-1' },
		email: 'user@example.test',
		roleGrants: [create(RoleGrantSchema, {
			role,
			scopeKind: RoleGrantScopeKind.DEVICE_GROUP,
			scopeId: { value: 'group-1' },
			scopeName: 'Fleet'
		})]
	});
	return {
		accessToken: 'access',
		refreshToken: 'refresh',
		expiresAt: new Date('2099-01-01T00:00:00.000Z'),
		user
	};
}

describe('auth storage', () => {
	it('round-trips nested protobuf users and native dates', () => {
		const auth = fixture();
		const parsed = parseAuth(serializeAuth(auth));

		expect(parsed.accessToken).toBe(auth.accessToken);
		expect(parsed.refreshToken).toBe(auth.refreshToken);
		expect(parsed.expiresAt?.toISOString()).toBe(auth.expiresAt?.toISOString());
		expect(toJson(UserSchema, parsed.user!)).toEqual(toJson(UserSchema, auth.user!));
	});

	it('rejects malformed wrappers, dates, and protobuf JSON', () => {
		const encoded = JSON.parse(serializeAuth(fixture())) as Record<string, unknown>;
		expect(() => parseAuth('{}')).toThrow();
		expect(() => parseAuth(JSON.stringify({ ...encoded, expiresAt: 'not-a-date' }))).toThrow();
		expect(() => parseAuth(JSON.stringify({ ...encoded, user: { '$typeName': 'cadestro.v1.User' } }))).toThrow();
	});

	it('falls back to secondary storage when primary storage is malformed', async () => {
		const local = new MemoryStorage();
		const session = new MemoryStorage();
		local.setItem('cadestro-persist', 'true');
		local.setItem('cadestro-auth', '{bad');
		session.setItem('cadestro-auth', serializeAuth({ ...fixture(), expiresAt: null }));
		Object.defineProperty(globalThis, 'window', { configurable: true, value: {} });
		Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: local });
		Object.defineProperty(globalThis, 'sessionStorage', { configurable: true, value: session });

		const { AuthStore } = await import('./auth.js');
		expect(new AuthStore().accessToken).toBe('access');

		delete (globalThis as { window?: unknown }).window;
		delete (globalThis as { localStorage?: unknown }).localStorage;
		delete (globalThis as { sessionStorage?: unknown }).sessionStorage;
	});
});
