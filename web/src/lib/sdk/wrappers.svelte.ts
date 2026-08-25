

import { ApiClient } from '$contractClient/client';
import { AuthStore, type RefreshResult } from '$contractClient/auth';
import { ConfigStore } from '$contractClient/config';
import { OfflineStore } from '$contractClient/offline';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import type { User } from '$contractClient/client';

const _config = new ConfigStore();
const _auth = new AuthStore();
const _offline = new OfflineStore();

function preconfiguredControlUrl(): string {
	if (typeof document === 'undefined') return '';
	const configured = document
		.querySelector('meta[name="cadestro-control-url"]')
		?.getAttribute('content');
	return (configured ?? '').trim().replace(/\/+$/, '');
}

const _preconfigured = preconfiguredControlUrl();
if (_preconfigured && !_config.isConfigured) {
	_config.serverUrl = _preconfigured;
}

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

export { useDraft, type DraftType } from './draft.svelte';

export type { ServerConfig } from '$contractClient/config';
export type {
	User, Device, RegistrationToken, ManagedAction, ActionSet, Definition,
	DeviceGroup, Assignment, AuditEvent, InventoryTableResult,
	Role, PermissionInfo, UserGroup, UserGroupMember, IdentityProvider, IdentityLink,
	LpsPassword, LuksKey
} from '$contractClient/client';

export * from '$contract/cadestro/v1/control_pb';
export * from '$contract/cadestro/v1/actions_pb';
export * from '$contract/cadestro/v1/common_pb';

export { formatTimestamp, formatTimestampDateTime } from '$contractClient/index';

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
