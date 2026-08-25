// The compliance policy sheet's editing contract.
//
// Two things this surface promises and a re-skin could silently break:
//
//   * the section blocks explain what a rule checks next to the control that
//     changes it — the explanation is not decoration, it is the left column of
//     the block, so it must render with the rules control;
//   * grace periods are a *committable* edit state: typing a new value arms the
//     shell's context pill, and only the pill's commit writes. Nothing is sent
//     while the operator is still typing, and the commit sends exactly the rules
//     whose value changed — an unchanged rule is never re-written.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { CompliancePolicySchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';
import { shell, resetShell, commitContext } from '$lib/shell/shell.svelte';

const POLICY_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const RULE_ONE = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const RULE_TWO = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';

const api = vi.hoisted(() => ({
	getCompliancePolicy: vi.fn(),
	updateCompliancePolicyRule: vi.fn(),
	removeCompliancePolicyRule: vi.fn(),
	addCompliancePolicyRule: vi.fn(),
	listActions: vi.fn()
}));
// Shallow-routing state the sheet derives its open/closed posture from.
const nav = vi.hoisted(() => ({ state: {} as { compliancePolicySheet?: string } }));

vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		// Reached through the action barrel's pipeline builders.
		useDraft: <T>(_type: string, _id: string, initial: T) => ({
			data: { ...initial },
			update: () => {},
			clear: async () => {}
		}),
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		formatDuration: () => '—',
		fetchAllPages: vi.fn(async () => [])
	};
});

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return new URL('https://control.test/compliance-policies');
		},
		get state() {
			return nav.state;
		}
	}
}));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));

vi.mock('$app/navigation', () => ({
	pushState: vi.fn(),
	replaceState: vi.fn(),
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

import CompliancePolicyDetailSheet from './compliance-policy-detail-sheet.svelte';

function policy(rules: { actionId: { value: string }; actionName: string; gracePeriodHours: number }[]) {
	return create(CompliancePolicySchema, {
		id: POLICY_ID,
		name: 'Security baseline',
		// Deliberately shares no words with a rule name — otherwise a rule lookup
		// also matches the description and the locator goes ambiguous.
		description: 'Baseline for managed laptops',
		ruleCount: rules.length,
		rules
	});
}

const RULES = [
		{ actionId: { value: RULE_ONE }, actionName: 'Disk encryption', gracePeriodHours: 0 },
		{ actionId: { value: RULE_TWO }, actionName: 'Screen lock', gracePeriodHours: 24 }
];

beforeEach(() => {
	document.body.innerHTML = '';
	resetShell();
	nav.state = { compliancePolicySheet: POLICY_ID };
	api.getCompliancePolicy.mockReset();
	api.updateCompliancePolicyRule.mockReset();
	// The fake server accumulates, so the rendered chip after a commit is a real
	// settle signal: it can only read "12 hours" once the write round trip is
	// done and the sheet has reseeded its draft from the answer.
	let stored = RULES.map((r) => ({ ...r }));
	api.getCompliancePolicy.mockImplementation(async () => policy(stored));
	api.updateCompliancePolicyRule.mockImplementation(
		async (_policyId: string, actionId: string, hours: number) => {
		stored = stored.map((r) => (r.actionId.value === actionId ? { ...r, gracePeriodHours: hours } : r));
			return policy(stored);
		}
	);
});

function graceInput(ruleName: string) {
	return browser.getByLabelText(
		`${m.compliance_policy_detail_grace_period_label()} — ${ruleName}`
	);
}

async function mountSheet() {
	render(CompliancePolicyDetailSheet);
	await vi.waitFor(() => expect(api.getCompliancePolicy).toHaveBeenCalledWith(POLICY_ID), {
		timeout: 3000
	});
	await expect.element(browser.getByText('Disk encryption', { exact: true })).toBeVisible();
}

describe('compliance policy sheet — explanation sits beside its control', () => {
	it('renders the rules block with both its explanation and its rule rows', async () => {
		await mountSheet();

		await expect
			.element(browser.getByText(m.compliance_policy_section_rules_title()))
			.toBeVisible();
		await expect
			.element(browser.getByText(m.compliance_policy_section_rules_help()))
			.toBeVisible();
		await expect
			.element(browser.getByText(m.compliance_policy_section_details_title()))
			.toBeVisible();
		await expect.element(browser.getByText('Screen lock', { exact: true })).toBeVisible();
	});
});

describe('compliance policy sheet — grace periods are a committable edit state', () => {
	it('arms the context pill on the first change and writes nothing before the commit', async () => {
		await mountSheet();
		expect(shell.pill.context).toBeNull();

		await graceInput('Disk encryption').fill('12');

		await vi.waitFor(
			() => expect(shell.pill.context?.id).toBe(`compliance-policy:${POLICY_ID}`),
			{ timeout: 3000 }
		);
		// One commit grammar app-wide: edit surfaces say Save. This said "Save grace
		// periods" while its siblings said "Save" — the pill's most-looked-at
		// control was saying twelve different things across the app.
		expect(shell.pill.context?.commitLabel).toBe(m.common_save());
		expect(shell.pill.context?.valid).toBe(true);
		// Typing is not saving.
		expect(api.updateCompliancePolicyRule).not.toHaveBeenCalled();
	});

	it('commits exactly the rules whose grace period changed', async () => {
		await mountSheet();

		await graceInput('Disk encryption').fill('12');
		await vi.waitFor(() => expect(shell.pill.context).not.toBeNull(), { timeout: 3000 });

		expect(commitContext()).toBe(true);

		// The writes are sequential and the first one is issued synchronously, so
		// counting calls right after the commit would pass even for a commit that
		// goes on to rewrite every rule. The rendered chip is the honest settle
		// signal: it can only read "12 hours" once the whole batch has landed and
		// the sheet has reseeded from the server's answer.
		await expect
			.element(browser.getByText(m.compliance_policy_detail_grace_period_hours({ count: 12 })))
			.toBeVisible();
		// "Screen lock" was never touched, so it is never re-written.
		expect(api.updateCompliancePolicyRule.mock.calls).toEqual([[POLICY_ID, RULE_ONE, 12]]);
	});

	it('marks an out-of-range grace period invalid so the pill cannot commit it', async () => {
		await mountSheet();

		// UpdateCompliancePolicyRuleRequest validates grace_period_hours <= 8760.
		await graceInput('Screen lock').fill('99999');

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false), { timeout: 3000 });
		expect(commitContext()).toBe(false);
		expect(api.updateCompliancePolicyRule).not.toHaveBeenCalled();
	});

	it('releases the pill again when the value is typed back to the stored one', async () => {
		await mountSheet();

		await graceInput('Screen lock').fill('48');
		await vi.waitFor(() => expect(shell.pill.context).not.toBeNull(), { timeout: 3000 });

		await graceInput('Screen lock').fill('24');

		await vi.waitFor(() => expect(shell.pill.context).toBeNull(), { timeout: 3000 });
	});
});
