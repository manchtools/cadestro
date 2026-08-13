// Shell navigation tables for the target design. The pill carries the five
// primary operator sections; every other section lives in the overflow,
// grouped as Workspace and Admin (movement A). Each
// entry declares the permission its RPC requires (F043 parity — strings match
// server/internal/auth/permissions.go; `null` = always visible). The tables
// are DATA: the (app) layout filters them against the session and passes the
// result to the chrome as props; chrome never touches the auth client.
//
// Labels are paraglide message FUNCTIONS, never resolved strings: this module
// is evaluated once per page load, so a resolved label would freeze whatever
// locale was active at import time and the pill would render English forever.
import * as m from '$lib/paraglide/messages';
import {
	Monitor,
	Send,
	ShieldCheck,
	ScrollText,
	Activity,
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

/** A label the chrome CALLS while rendering — a paraglide message accessor.
 *  Keeping it lazy is what makes the pill follow the active locale. */
export type NavLabel = () => string;

/** What the chrome renders — deliberately WITHOUT permission: filtering
 *  happens in the layout, the pill never sees the concept. */
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
	/** Permission required to see this entry. `null` = always visible. */
	permission: string | null;
}

export interface NavGroup {
	group: NavLabel;
	items: NavEntry[];
}

/** The pill's resting sections — the operator loop, exactly five (round 2,
 *  movement A: Devices · Actions · Compliance · Audit · Executions). Adding a
 *  sixth belongs in the overflow, not here. */
export const PRIMARY_SECTIONS: NavEntry[] = [
	{ href: '/devices', label: m.nav_devices, icon: Monitor, permission: 'ListDevices' },
	{ href: '/actions', label: m.nav_actions, icon: Send, permission: 'ListActions' },
	// The pill row wears the SHORT forms: "Compliance Policies" / "Audit Log"
	// are the page titles, but five long labels overflow the capsule.
	{ href: '/compliance-policies', label: m.nav_compliance_short, icon: ShieldCheck, permission: 'ListCompliancePolicies' },
	{ href: '/audit', label: m.nav_audit_short, icon: ScrollText, permission: 'ListAuditEvents' },
	{ href: '/executions', label: m.nav_executions, icon: Activity, permission: 'ListExecutions' }
];

/** Everything else, behind the overflow — two groups.
 *
 *  Round 2 (movement A) fixes the split: the five operator sections above are
 *  the pill's resting row, and "the pill's admin overflow holds users · roles ·
 *  groups · tokens · IdP · terminal sessions" — so registration tokens sit with
 *  Admin (they mint agent identities), while the operator's own build surfaces
 *  (device groups, action sets, definitions, my devices, marketplace) stay in
 *  Workspace. No destination is dropped: every route the sidebar reached is
 *  still exactly one entry in one of these two groups (nav.test.ts proves it
 *  against the route tree). */
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

/** Filter a table against a permission predicate (the layout supplies
 *  session-backed `has`; admin short-circuits there). Groups that end up
 *  empty are dropped. */
export function filterNav(entries: NavEntry[], has: (p: string) => boolean): NavEntry[] {
	return entries.filter((e) => e.permission === null || has(e.permission));
}

export function filterGroups(groups: NavGroup[], has: (p: string) => boolean): NavGroup[] {
	return groups
		.map((g) => ({ group: g.group, items: filterNav(g.items, has) }))
		.filter((g) => g.items.length > 0);
}
