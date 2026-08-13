// The getting-started checklist.
//
// Every row is an answer from a REAL read — there is no seeded "you have not
// done this yet" list. That has three consequences the code is built around:
//
//   1. a check the caller may not run (permission denied) is HIDDEN, not shown
//      as incomplete: nagging an operator about work they are not allowed to do
//      is noise, and pretending to know their state would be a lie;
//   2. a check that failed for any other reason is `unknown` — rendered, but
//      counted in neither half of "N of M complete", because a failed read is
//      not evidence of an empty fleet;
//   3. progress is derived from the rows that answered, never from a constant.
import { ConnectError, Code } from '@connectrpc/connect';
import { apiClient } from '$lib/sdk';
import { getErrorCode } from '$lib/errors';
import * as m from '$lib/paraglide/messages';

export type CheckStatus = 'done' | 'todo' | 'hidden' | 'unknown';

type Outcome = 'yes' | 'no' | 'denied' | 'unknown';

export interface ChecklistCheck {
	id: string;
	/** Route the row links to — the surface that actually does the thing. */
	href: string;
	title: () => string;
	hint: () => string;
	/** Every probe that can answer this row; the first affirmative wins. */
	probes: (() => Promise<boolean>)[];
}

export interface ChecklistRow {
	id: string;
	href: string;
	title: string;
	hint: string;
	status: CheckStatus;
}

function denied(err: unknown): boolean {
	if (getErrorCode(err) === 'permission_denied') return true;
	// Transport-level denial: the server may reject before it attaches a domain
	// error detail, and that is still "you may not read this".
	return err instanceof ConnectError && err.code === Code.PermissionDenied;
}

async function run(probe: () => Promise<boolean>): Promise<Outcome> {
	try {
		return (await probe()) ? 'yes' : 'no';
	} catch (err) {
		if (denied(err)) return 'denied';
		console.warn('onboarding: getting-started check failed', err);
		return 'unknown';
	}
}

/** yes wins, then a real no; an all-denied row disappears; anything else is
 *  honestly unknown. */
export function combine(outcomes: Outcome[]): CheckStatus {
	if (outcomes.length === 0) return 'unknown';
	if (outcomes.includes('yes')) return 'done';
	if (outcomes.includes('no')) return 'todo';
	if (outcomes.every((o) => o === 'denied')) return 'hidden';
	return 'unknown';
}

export const CHECKS: ChecklistCheck[] = [
	// Token FIRST: an agent presents a registration token the first time it
	// connects, so there is nothing to enrol with until one exists. The list is
	// the order of work, so it must not ask for the effect before the cause.
	{
		id: 'token',
		href: '/tokens',
		title: m.onboarding_check_token_title,
		hint: m.onboarding_check_token_hint,
		probes: [async () => (await apiClient.listTokens(1)).tokens.length > 0]
	},
	{
		id: 'device',
		href: '/devices',
		title: m.onboarding_check_device_title,
		hint: m.onboarding_check_device_hint,
		probes: [
			async () => {
				const r = await apiClient.listDevices(1);
				return (r.totalCount ?? 0) > 0 || r.devices.length > 0;
			}
		]
	},
	{
		id: 'action',
		href: '/actions',
		title: m.onboarding_check_action_title,
		hint: m.onboarding_check_action_hint,
		probes: [async () => (await apiClient.listActions(1)).actions.length > 0]
	},
	{
		id: 'assignment',
		href: '/assign',
		title: m.onboarding_check_assignment_title,
		hint: m.onboarding_check_assignment_hint,
		probes: [async () => (await apiClient.listAssignments(1)).assignments.length > 0]
	},
	{
		id: 'people',
		href: '/users',
		title: m.onboarding_check_people_title,
		hint: m.onboarding_check_people_hint,
		// Two honest signals for one goal: a second human exists, or an identity
		// provider is wired up so humans can arrive at all.
		probes: [
			async () => (await apiClient.listUsers(2)).users.length > 1,
			async () => (await apiClient.listIdentityProviders(1)).providers.length > 0
		]
	}
];

export async function loadChecklist(checks: ChecklistCheck[] = CHECKS): Promise<ChecklistRow[]> {
	const rows = await Promise.all(
		checks.map(async (c) => ({
			id: c.id,
			href: c.href,
			title: c.title(),
			hint: c.hint(),
			status: combine(await Promise.all(c.probes.map(run)))
		}))
	);
	return rows.filter((r) => r.status !== 'hidden');
}

/** Rows that produced a real answer are the only ones that can be "complete". */
export function progress(rows: ChecklistRow[]): { done: number; total: number } {
	const answered = rows.filter((r) => r.status === 'done' || r.status === 'todo');
	return { done: answered.filter((r) => r.status === 'done').length, total: answered.length };
}
