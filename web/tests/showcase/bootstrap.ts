

import type { Page } from '@playwright/test';
import { ALL_PERMISSIONS } from './dummy';

const CONFIG_KEY = 'cadestro-config';
const AUTH_KEY = 'cadestro-auth';
const PERSIST_KEY = 'cadestro-persist';
const MODE_KEY = 'mode-watcher-mode';
const SHOWCASE_CONFIG = JSON.stringify({ serverUrl: 'https://localhost:5179' });

export type Theme = 'light' | 'dark';
export const ADMIN_ROLE_ID = '00000000000000000000000001';

export function buildAuthSuperjson(opts: {
	roleId: string;
	roleName: string;
	permissions: string[];
}): string {
	const role = {
		$typeName: 'cadestro.v1.Role',
		id: opts.roleId,
		name: opts.roleName,
		description: '',
		permissions: opts.permissions,
		isSystem: opts.roleId === ADMIN_ROLE_ID
	};
	const auth = {
		accessToken: 'showcase-jwt-access',
		refreshToken: 'showcase-jwt-refresh',
		expiresAt: '2099-01-01T00:00:00.000Z',
		user: {
			$typeName: 'cadestro.v1.User',
			id: '01J6XYZSHOWCASEADMINUSR01',
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
				{
					$typeName: 'cadestro.v1.RoleGrant',
					role,
					scopeKind: 0,
					scopeId: '',
					scopeName: ''
				}
			],
			inheritedRoles: []
		}
	};
	return JSON.stringify({ json: auth, meta: { values: { expiresAt: ['Date'] }, v: 1 } });
}

const SHOWCASE_AUTH_SUPERJSON = buildAuthSuperjson({
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
	await seedSession(page, theme, SHOWCASE_AUTH_SUPERJSON);
}

export async function primeStorageAs(
	page: Page,
	theme: Theme,
	opts: { roleId: string; roleName: string; permissions: string[] }
): Promise<void> {
	await seedSession(page, theme, buildAuthSuperjson(opts));
}
