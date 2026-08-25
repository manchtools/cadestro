import type { User } from '../gen/ts/cadestro/v1/control_pb';
import { UserSchema } from '../gen/ts/cadestro/v1/control_pb';
import { fromJson, toJson, type JsonValue } from '@bufbuild/protobuf';
import { logger, describeError } from './logger.js';

const log = logger.named('auth');

const AUTH_KEY = 'cadestro-auth';
const PERSIST_KEY = 'cadestro-persist';

export interface StoredAuth {
	accessToken: string | null;
	refreshToken: string | null;
	expiresAt: Date | null;
	user: User | null;
}

export interface RefreshResult {
	accessToken: string;
	refreshToken: string;
	expiresAt: Date;
}

const emptyAuth: StoredAuth = { accessToken: null, refreshToken: null, expiresAt: null, user: null };

type StoredAuthJSON = {
	accessToken: string | null;
	refreshToken: string | null;
	expiresAt: string | null;
	user: JsonValue | null;
};

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isNullableString(value: unknown): value is string | null {
	return value === null || typeof value === 'string';
}

function parseStoredAuthJSON(value: unknown): StoredAuthJSON {
	if (!isRecord(value)) throw new Error('stored auth must be an object');
	const fields = ['accessToken', 'refreshToken', 'expiresAt', 'user'];
	if (Object.keys(value).some((key) => !fields.includes(key))) throw new Error('stored auth has unknown fields');
	if (!fields.every((field) => Object.hasOwn(value, field))) throw new Error('stored auth is incomplete');
	if (!isNullableString(value.accessToken) || !isNullableString(value.refreshToken) || !isNullableString(value.expiresAt)) {
		throw new Error('stored auth has invalid scalar fields');
	}
	if (value.user !== null && !isRecord(value.user)) throw new Error('stored auth has an invalid user');
	return value as StoredAuthJSON;
}

export function parseAuth(data: string): StoredAuth {
	const stored = parseStoredAuthJSON(JSON.parse(data));
	let expiresAt: Date | null = null;
	if (stored.expiresAt !== null) {
		expiresAt = new Date(stored.expiresAt);
		if (Number.isNaN(expiresAt.getTime()) || expiresAt.toISOString() !== stored.expiresAt) {
			throw new Error('stored auth has an invalid expiry');
		}
	}
	return {
		accessToken: stored.accessToken,
		refreshToken: stored.refreshToken,
		expiresAt,
		user: stored.user === null ? null : fromJson(UserSchema, stored.user)
	};
}

export function serializeAuth(auth: StoredAuth): string {
	if (!isNullableString(auth.accessToken) || !isNullableString(auth.refreshToken)) {
		throw new Error('auth has invalid tokens');
	}
	if (auth.expiresAt !== null && (!(auth.expiresAt instanceof Date) || Number.isNaN(auth.expiresAt.getTime()))) {
		throw new Error('auth has an invalid expiry');
	}
	return JSON.stringify({
		accessToken: auth.accessToken,
		refreshToken: auth.refreshToken,
		expiresAt: auth.expiresAt?.toISOString() ?? null,
		user: auth.user === null ? null : toJson(UserSchema, auth.user)
	});
}

function isPersistent(): boolean {
	if (typeof localStorage === 'undefined') return false;
	return localStorage.getItem(PERSIST_KEY) === 'true';
}

function loadAuth(): StoredAuth {
	if (typeof window === 'undefined') return { ...emptyAuth };

	const persistent = isPersistent();
	const primary = persistent ? localStorage.getItem(AUTH_KEY) : sessionStorage.getItem(AUTH_KEY);
	if (primary) {
		try { return parseAuth(primary); }
		catch (err) {
			log.warn('failed to parse primary stored auth blob; falling back to secondary storage', describeError(err));
		}
	}

	const fallback = persistent ? sessionStorage.getItem(AUTH_KEY) : localStorage.getItem(AUTH_KEY);
	if (fallback) {
		try { return parseAuth(fallback); }
		catch (err) {
			log.warn('failed to parse fallback stored auth blob; starting with empty auth', describeError(err));
		}
	}

	return { ...emptyAuth };
}

function saveAuth(auth: StoredAuth) {
	if (typeof window === 'undefined') return;
	const data = serializeAuth(auth);
	if (isPersistent()) {
		localStorage.setItem(AUTH_KEY, data);
	} else {
		sessionStorage.setItem(AUTH_KEY, data);
	}
}

function clearAuth() {
	if (typeof window === 'undefined') return;
	localStorage.removeItem(AUTH_KEY);
	sessionStorage.removeItem(AUTH_KEY);
	localStorage.removeItem(PERSIST_KEY);
}

export class AuthStore {
	private state: StoredAuth = loadAuth();
	private refreshPromise: Promise<void> | null = null;
	private refreshTimeoutId: ReturnType<typeof setTimeout> | null = null;
	private listeners = new Set<() => void>();

	private refreshFn: (() => Promise<RefreshResult | null>) | null = null;
	private logoutFn: (() => Promise<void>) | null = null;

	constructor() {
		if (typeof window !== 'undefined') {
			this.scheduleRefresh();
		}
	}

	private notify() {
		for (const fn of this.listeners) fn();
	}

	onChange(listener: () => void): () => void {
		this.listeners.add(listener);
		return () => this.listeners.delete(listener);
	}

	setRefreshFn(fn: () => Promise<RefreshResult | null>) {
		this.refreshFn = fn;
	}

	setLogoutFn(fn: () => Promise<void>) {
		this.logoutFn = fn;
	}

	get persist(): boolean {
		return isPersistent();
	}

	setPersist(value: boolean) {
		if (typeof localStorage === 'undefined') return;
		if (value) {
			localStorage.setItem(PERSIST_KEY, 'true');
		} else {
			localStorage.removeItem(PERSIST_KEY);

			localStorage.removeItem(AUTH_KEY);
		}
	}

	get user() {
		return this.state.user;
	}

	get accessToken() {
		return this.state.accessToken;
	}

	get refreshToken() {
		return this.state.refreshToken;
	}

	get isAuthenticated() {
		return this.state.user !== null && this.state.accessToken !== null && !this.isExpired();
	}

	get isAdmin() {
		const adminRoleID = '00000000000000000000000001';
		for (const grant of this.state.user?.roleGrants ?? []) {
			if (grant.role?.id?.value === adminRoleID) return true;
		}
		const inherited = this.state.user?.inheritedRoles ?? [];
		for (const ir of inherited) {
			if (ir.roleId?.value === adminRoleID) return true;
		}
		return false;
	}

	hasPermission(permission: string) {
		for (const grant of this.state.user?.roleGrants ?? []) {
			if (grant.role?.permissions.includes(permission)) return true;
		}
		return false;
	}

	private isExpired() {
		if (!this.state.expiresAt) return true;
		return new Date() >= new Date(this.state.expiresAt.getTime() - 30000);
	}

	private scheduleRefresh() {
		if (this.refreshTimeoutId) {
			clearTimeout(this.refreshTimeoutId);
			this.refreshTimeoutId = null;
		}

		if (!this.state.expiresAt || !this.state.user) return;

		const refreshAt = this.state.expiresAt.getTime() - 60000;
		const delay = refreshAt - Date.now();

		if (delay > 0) {
			this.refreshTimeoutId = setTimeout(() => this.refresh(), delay);
		} else if (this.state.user) {
			this.refresh();
		}
	}

	setAuth(accessToken: string, refreshToken: string, expiresAt: Date, user: User) {
		this.state = { accessToken, refreshToken, expiresAt, user };
		saveAuth(this.state);
		this.scheduleRefresh();
		this.notify();
	}

	updateUser(user: User) {
		this.state.user = user;
		saveAuth(this.state);
		this.notify();
	}

	async refresh(): Promise<boolean> {
		if (!this.state.user || !this.state.refreshToken) return false;

		if (this.refreshPromise) {
			await this.refreshPromise;
			return this.isAuthenticated;
		}

		this.refreshPromise = this.doRefresh();
		try {
			await this.refreshPromise;
			return this.isAuthenticated;
		} finally {
			this.refreshPromise = null;
		}
	}

	private async doRefresh(): Promise<void> {
		if (!this.refreshFn) return;

		try {
			const result = await this.refreshFn();
			if (result) {
				this.state.accessToken = result.accessToken;
				this.state.refreshToken = result.refreshToken;
				this.state.expiresAt = result.expiresAt;
				saveAuth(this.state);
				this.scheduleRefresh();
				this.notify();
			}
		} catch (error) {
			log.error('token refresh failed', describeError(error));
		}
	}

	async ensureValidToken(): Promise<void> {
		if (this.isExpired() && this.state.user) {
			await this.refresh();
		}
	}

	async logout() {
		if (this.refreshTimeoutId) {
			clearTimeout(this.refreshTimeoutId);
			this.refreshTimeoutId = null;
		}

		if (this.state.user && this.logoutFn) {
			try {
				await this.logoutFn();
			} catch (err) {

				log.debug('server-side logout failed; local session cleared anyway', describeError(err));
			}
		}

		this.state = { accessToken: null, refreshToken: null, expiresAt: null, user: null };
		clearAuth();
		this.notify();
	}
}
