

import * as m from '$lib/paraglide/messages';
import {
	Monitor,
	Send,
	ShieldCheck,
	ScrollText,
	Group,
	Key,
	Layers,
	FolderTree,
	Store,
	Users,
	UsersRound,
	Shield,
	Fingerprint,
	Terminal,
	Settings
} from '@lucide/svelte';

export type NavLabel = () => string;

export interface PillEntry {
	href: string;
	label: NavLabel;
	icon: typeof Monitor;
}

export interface PillGroup {
	group: NavLabel;
	items: PillEntry[];
}

export interface NavEntry extends PillEntry {

	permission: string | null;
}

export interface NavGroup {
	group: NavLabel;
	items: NavEntry[];
}

export const PRIMARY_SECTIONS: NavEntry[] = [
	{ href: '/devices', label: m.nav_devices, icon: Monitor, permission: 'ListDevices' },
	{ href: '/actions', label: m.nav_actions, icon: Send, permission: 'ListActions' },

	{ href: '/compliance-policies', label: m.nav_compliance_short, icon: ShieldCheck, permission: 'ListCompliancePolicies' },
	{ href: '/audit', label: m.nav_audit_short, icon: ScrollText, permission: 'ListAuditEvents' },
];

export const OVERFLOW_GROUPS: NavGroup[] = [
	{
		group: m.nav_workspace,
		items: [
			{ href: '/my-devices', label: m.nav_my_devices, icon: Monitor, permission: null },
			{ href: '/device-groups', label: m.nav_device_groups, icon: Group, permission: 'ListDeviceGroups' },
			{ href: '/action-sets', label: m.nav_action_sets, icon: Layers, permission: 'ListActionSets' },
			{ href: '/definitions', label: m.nav_definitions, icon: FolderTree, permission: 'ListDefinitions' },
			{ href: '/marketplace', label: m.nav_marketplace, icon: Store, permission: null }
		]
	},
	{
		group: m.nav_admin,
		items: [
			{ href: '/users', label: m.nav_users, icon: Users, permission: 'ListUsers' },
			{ href: '/roles', label: m.nav_roles, icon: Shield, permission: 'ListRoles' },
			{ href: '/user-groups', label: m.nav_user_groups, icon: UsersRound, permission: 'ListUserGroups' },
			{ href: '/tokens', label: m.nav_tokens, icon: Key, permission: 'ListRegistrationTokens' },
			{ href: '/identity-providers', label: m.nav_identity_providers, icon: Fingerprint, permission: 'ListIdentityProviders' },
			{ href: '/admin/terminal-sessions', label: m.nav_terminal_sessions, icon: Terminal, permission: 'ListTerminalSessions' },
			{ href: '/settings', label: m.nav_settings, icon: Settings, permission: null }
		]
	}
];

export function filterNav(entries: NavEntry[], has: (p: string) => boolean): NavEntry[] {
	return entries.filter((e) => e.permission === null || has(e.permission));
}

export function filterGroups(groups: NavGroup[], has: (p: string) => boolean): NavGroup[] {
	return groups
		.map((g) => ({ group: g.group, items: filterNav(g.items, has) }))
		.filter((g) => g.items.length > 0);
}
