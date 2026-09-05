import * as m from '$lib/paraglide/messages';
import { Activity, Boxes, ClipboardList, Fingerprint, KeyRound, Monitor, Shield, Users } from '@lucide/svelte';
import { Permission } from '$contract/cadestro/v1/control_pb';

export type NavLabel = () => string;

export type PillEntry = {
	href: string;
	label: NavLabel;
	icon: typeof Monitor;
};

export type PillGroup = {
	group: NavLabel;
	items: PillEntry[];
};

type NavEntry = PillEntry & { permissions: Permission[] };
type NavGroup = { group: NavLabel; items: NavEntry[] };

export const PRIMARY_SECTIONS: NavEntry[] = [
	{ href: '/devices', label: m.nav_devices, icon: Monitor, permissions: [Permission.LIST_DEVICES] },
	{ href: '/actions', label: m.nav_actions, icon: Activity, permissions: [Permission.LIST_ACTIONS, Permission.CREATE_ACTION] },
	{ href: '/audit', label: m.nav_audit_short, icon: ClipboardList, permissions: [Permission.LIST_AUDIT_EVENTS] }
];

export const OVERFLOW_GROUPS: NavGroup[] = [
	{
		group: m.nav_workspace,
		items: [
			{ href: '/device-groups', label: m.nav_device_groups, icon: Boxes, permissions: [Permission.LIST_DEVICE_GROUPS, Permission.CREATE_DEVICE_GROUP] },
			{ href: '/assignments', label: m.assignments_title, icon: ClipboardList, permissions: [Permission.LIST_ASSIGNMENTS, Permission.CREATE_ASSIGNMENT, Permission.DELETE_ASSIGNMENT] }
		]
	},
	{
		group: m.nav_admin,
		items: [
			{ href: '/users', label: m.nav_users, icon: Users, permissions: [Permission.LIST_USERS] },
			{ href: '/roles', label: m.nav_roles, icon: Shield, permissions: [Permission.LIST_ROLES, Permission.CREATE_ROLE] },
			{ href: '/tokens', label: m.nav_tokens, icon: KeyRound, permissions: [Permission.LIST_TOKENS, Permission.CREATE_TOKEN, Permission.RENAME_TOKEN, Permission.DELETE_TOKEN] },
			{ href: '/identity-providers', label: m.nav_identity_providers, icon: Fingerprint, permissions: [Permission.LIST_IDENTITY_PROVIDERS, Permission.CREATE_IDENTITY_PROVIDER] }
		]
	}
];

export function filterNav(entries: NavEntry[], has: (permission: Permission) => boolean): PillEntry[] {
	return entries.filter((entry) => entry.permissions.some(has));
}

export function filterGroups(groups: NavGroup[], has: (permission: Permission) => boolean): PillGroup[] {
	return groups
		.map((group) => ({ group: group.group, items: filterNav(group.items, has) }))
		.filter((group) => group.items.length > 0);
}
