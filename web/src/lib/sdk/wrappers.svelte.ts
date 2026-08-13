// Power Manage Web SDK
// Wires the plain TypeScript SDK classes together with Svelte 5 reactive wrappers.

import { ApiClient } from '$pmSdk/client';
import { AuthStore, type RefreshResult } from '$pmSdk/auth';
import { ConfigStore } from '$pmSdk/config';
import { OfflineStore } from '$pmSdk/offline';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import type { User } from '$pmSdk/client';

// --- Create plain TS instances ---

const _config = new ConfigStore();
const _auth = new AuthStore();
const _offline = new OfflineStore();

// --- Wire ApiClient with dependency injection ---
// Closures capture _auth/_config by reference; works because refreshFn/logoutFn
// are called lazily (never during construction).

const _client = new ApiClient({
	getServerUrl: () => _config.serverUrl,
	getAccessToken: () => _auth.accessToken,
	getRefreshToken: () => _auth.refreshToken,
	ensureValidToken: () => _auth.ensureValidToken(),
	refreshToken: () => _auth.refresh(),
	onUnauthenticated: () => {
		if (typeof window !== 'undefined') {
			const base = (import.meta.env.BASE_URL ?? '').replace(/\/$/, '');
			const currentPath = window.location.pathname.replace(base || '', '') + window.location.search;
			window.location.href = (base || '') + '/login?redirect=' + encodeURIComponent(currentPath);
		}
	},
	onAuthResponse: (accessToken: string, refreshToken: string, expiresAt: Date, user: User) => {
		_auth.setAuth(accessToken, refreshToken, expiresAt, user);
	},
	onUserUpdated: (user: User) => {
		_auth.updateUser(user);
	}
});

// Wire auth refresh/logout callbacks (breaks the circular dependency)
_auth.setRefreshFn(async (): Promise<RefreshResult | null> => {
	try {
		const response = await _client.refreshTokenRPC();
		if (response.expiresAt && response.accessToken && response.refreshToken) {
			return {
				accessToken: response.accessToken,
				refreshToken: response.refreshToken,
				expiresAt: timestampDate(response.expiresAt)
			};
		}
	} catch (error) {
		console.error('Token refresh failed:', error);
	}
	return null;
});

_auth.setLogoutFn(async () => { await _client.logoutRPC(); });

// --- Svelte 5 reactive wrappers ---
// Reading _authV / _configV inside getters makes Svelte track the dependency.
// When the plain TS store calls notify() → version increments → components re-render.

let _authV = $state(0);
_auth.onChange(() => _authV++);

export const authStore = {
	get user() { void _authV; return _auth.user; },
	get accessToken() { void _authV; return _auth.accessToken; },
	get refreshToken() { void _authV; return _auth.refreshToken; },
	get isAuthenticated() { void _authV; return _auth.isAuthenticated; },
	get isAdmin() { void _authV; return _auth.isAdmin; },
	get persist() { return _auth.persist; },
	hasPermission(p: string) { void _authV; return _auth.hasPermission(p); },
	setAuth: _auth.setAuth.bind(_auth),
	setPersist: _auth.setPersist.bind(_auth),
	updateUser: _auth.updateUser.bind(_auth),
	refresh: _auth.refresh.bind(_auth),
	ensureValidToken: _auth.ensureValidToken.bind(_auth),
	logout: _auth.logout.bind(_auth),
};

let _configV = $state(0);
_config.onChange(() => _configV++);

export const configStore = {
	get serverUrl() { void _configV; return _config.serverUrl; },
	set serverUrl(url: string) { _config.serverUrl = url; },
	get isConfigured() { void _configV; return _config.isConfigured; },
};

export const offlineStore = _offline;
export const apiClient = _client;

// Re-export Svelte hook
export { useDraft, type DraftType } from './draft.svelte';

// Re-export SDK types
export type { ServerConfig } from '$pmSdk/config';
export type {
	User, Device, RegistrationToken, ManagedAction, ActionSet, Definition,
	DeviceGroup, Assignment, ActionExecution, AuditEvent, InventoryTableResult,
	Role, PermissionInfo, UserGroup, UserGroupMember, IdentityProvider, IdentityLink,
	LpsPassword, LuksKey
} from '$pmSdk/client';

// Re-export generated types
export * from '$sdk/powermanage/v1/control_pb';
export * from '$sdk/powermanage/v1/actions_pb';
export * from '$sdk/powermanage/v1/common_pb';

// Re-export helpers
export { formatTimestamp, formatTimestampDateTime } from '$pmSdk/index';

/**
 * Format a duration in milliseconds to a human-readable string.
 * <1s: "42ms", <1m: "2.73s", <1h: "3m 12s", ≥1h: "1h 23m 4s"
 */
export function formatDuration(ms: bigint | undefined): string {
	if (!ms) return '-';
	const n = Number(ms);
	if (n < 1000) return `${n}ms`;
	if (n < 60_000) return `${(n / 1000).toFixed(2)}s`;
	const hours = Math.floor(n / 3_600_000);
	const mins = Math.floor((n % 3_600_000) / 60_000);
	const secs = Math.floor((n % 60_000) / 1000);
	if (hours > 0) return `${hours}h ${mins}m ${secs}s`;
	return `${mins}m ${secs}s`;
}
