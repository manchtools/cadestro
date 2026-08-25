

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
		id: { value: POLICY_ID },
		name: 'Security baseline',

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

		expect(shell.pill.context?.commitLabel).toBe(m.common_save());
		expect(shell.pill.context?.valid).toBe(true);

		expect(api.updateCompliancePolicyRule).not.toHaveBeenCalled();
	});

	it('commits exactly the rules whose grace period changed', async () => {
		await mountSheet();

		await graceInput('Disk encryption').fill('12');
		await vi.waitFor(() => expect(shell.pill.context).not.toBeNull(), { timeout: 3000 });

		expect(commitContext()).toBe(true);

		await expect
			.element(browser.getByText(m.compliance_policy_detail_grace_period_hours({ count: 12 })))
			.toBeVisible();

		expect(api.updateCompliancePolicyRule.mock.calls).toEqual([[POLICY_ID, RULE_ONE, 12]]);
	});

	it('marks an out-of-range grace period invalid so the pill cannot commit it', async () => {
		await mountSheet();

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
