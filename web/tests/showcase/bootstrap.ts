

import type { Page } from '@playwright/test';
import { create } from '@bufbuild/protobuf';
import { RoleGrantScopeKind } from '$contract/cadestro/v1/common_pb';
import { RoleGrantSchema, RoleSchema, UserSchema } from '$contract/cadestro/v1/control_pb';
import { serializeAuth, type StoredAuth } from '$contractClient/auth';
import { ALL_PERMISSIONS } from './dummy';

const CONFIG_KEY = 'cadestro-config';
const AUTH_KEY = 'cadestro-auth';
const PERSIST_KEY = 'cadestro-persist';
const MODE_KEY = 'mode-watcher-mode';
const SHOWCASE_CONFIG = JSON.stringify({ serverUrl: 'https://localhost:5179' });

export type Theme = 'light' | 'dark';
export const ADMIN_ROLE_ID = '00000000000000000000000001';

export function buildAuthStorage(opts: {
	roleId: string;
	roleName: string;
	permissions: string[];
}): string {
	const role = create(RoleSchema, {
		id: { value: opts.roleId },
		name: opts.roleName,
		description: '',
		permissions: opts.permissions,
		isSystem: opts.roleId === ADMIN_ROLE_ID
	});
	const user = create(UserSchema, {
		id: { value: '01J6XYZSHOWCASEADMINUSR01' },
		email: 'sam.reiter@cadestro.example',
		displayName: 'Sam Reiter',
		givenName: 'Sam',
		familyName: 'Reiter',
		preferredUsername: 'sam.reiter',
		locale: 'en',
		picture: '',
		disabled: false,
		identityLinks: [],
		roleGrants: [
			create(RoleGrantSchema, {
				role,
				scopeKind: RoleGrantScopeKind.UNSPECIFIED,
				scopeId: { value: '' },
				scopeName: ''
			})
		],
		inheritedRoles: []
	});
	const auth: StoredAuth = {
		accessToken: 'showcase-jwt-access',
		refreshToken: 'showcase-jwt-refresh',
		expiresAt: new Date('2099-01-01T00:00:00.000Z'),
		user
	};
	return serializeAuth(auth);
}

const SHOWCASE_AUTH_STORAGE = buildAuthStorage({
	roleId: ADMIN_ROLE_ID,
	roleName: 'Administrator',
	permissions: ALL_PERMISSIONS
});

async function seedSession(page: Page, theme: Theme, auth: string): Promise<void> {
	await page.addInitScript(
		({ configKey, authKey, persistKey, modeKey, auth, config, mode }) => {
			try {
				localStorage.setItem(configKey, config);
				localStorage.setItem(persistKey, 'true');
				localStorage.setItem(authKey, auth);
				localStorage.setItem(modeKey, mode);
			} catch {

			}
		},
		{
			configKey: CONFIG_KEY,
			authKey: AUTH_KEY,
			persistKey: PERSIST_KEY,
			modeKey: MODE_KEY,
			auth,
			config: SHOWCASE_CONFIG,
			mode: theme
		}
	);

	await page.addInitScript(() => {
		const css = 'a[href$="/marketplace"], a[href*="/marketplace?"] { display: none !important; }';
		const apply = () => {
			if (!document.head) return;
			const style = document.createElement('style');
			style.setAttribute('data-showcase-hide-marketplace', '');
			style.textContent = css;
			document.head.appendChild(style);
		};
		if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', apply);
		else apply();
	});
}

export async function primeStorage(page: Page, theme: Theme): Promise<void> {
	await seedSession(page, theme, SHOWCASE_AUTH_STORAGE);
}

export async function primeStorageAs(
	page: Page,
	theme: Theme,
	opts: { roleId: string; roleName: string; permissions: string[] }
): Promise<void> {
	await seedSession(page, theme, buildAuthStorage(opts));
}
