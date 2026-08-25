

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ShellParamsSchema, ActionType } from '$contract/cadestro/v1/actions_pb';
import { ManagedActionSchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createCompliancePolicy: vi.fn(),
	addCompliancePolicyRule: vi.fn(),
	deleteCompliancePolicy: vi.fn(),
	listActions: vi.fn(),
	listUsers: vi.fn(),
	search: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/compliance-policies/new') }));

vi.mock('$lib/sdk', async () => {
	const control = await import('$contract/cadestro/v1/control_pb');
	const common = await import('$contract/cadestro/v1/common_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		authStore: { user: { id: { value: '01JQZZ0000000000000000000A' }}, hasPermission: () => true },
		configStore: { serverUrl: 'https://control.test' },
		formatTimestamp: () => '2026-08-01',
		formatTimestampDateTime: () => '2026-08-01 09:00',

		fetchAllPages: vi.fn(async () => {
			const response = await api.listActions();
			return response.actions;
		}),
		useDraft: <T>(_type: string, _id: string, initial: T) => {
			let data = initial;
			return {
				get data() {
					return data;
				},
				set data(next: T) {
					data = next;
				},
				update() {},
				clear: async () => {},
				get hasDraft() {
					return false;
				}
			};
		}
	};
});

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));
vi.mock('$app/state', () => ({
	page: {
		get url() {
			return nav.url;
		},
		params: {},

		state: {}
	}
}));

import { goto } from '$app/navigation';
import NewPolicyPage from './+page.svelte';
import PoliciesPage from '../+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';

const ROUTE = '/compliance-policies/new';
const POLICY_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const CHECK_A = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const CHECK_B = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';
const PLAIN_SHELL = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';

const catalogue = [
	create(ManagedActionSchema, {
		id: { value: CHECK_A },
		name: 'Disk encrypted',
		type: ActionType.SHELL,
		params: { case: 'shell', value: create(ShellParamsSchema, { isCompliance: true }) }
	}),
	create(ManagedActionSchema, {
		id: { value: CHECK_B },
		name: 'Firewall on',
		type: ActionType.SHELL,
		params: { case: 'shell', value: create(ShellParamsSchema, { isCompliance: true }) }
	}),
	create(ManagedActionSchema, {
		id: { value: PLAIN_SHELL },
		name: 'Rotate log files',
		type: ActionType.SHELL,
		params: { case: 'shell', value: create(ShellParamsSchema, { isCompliance: false }) }
	})
];

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/compliance-policies/new');
	api.createCompliancePolicy.mockResolvedValue({ id: { value: POLICY_ID }, name: 'Baseline posture' });
	api.addCompliancePolicyRule.mockImplementation(async () => ({ id: { value: POLICY_ID }}));
	api.listActions.mockResolvedValue({ actions: catalogue, nextPageToken: '' });
	api.listUsers.mockResolvedValue({ users: [], nextPageToken: '' });
	api.search.mockResolvedValue({ results: [], totalCount: 0n, nextPageToken: '' });
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);
const area = (id: string) => document.querySelector<HTMLTextAreaElement>(`#${id}`);
const ruleRow = (id: string) =>
	document.querySelector<HTMLButtonElement>(
		`[data-testid="policy-rule-row"][data-action-id="${id}"]`
	);
const selectedCount = () =>
	document.querySelector('[data-testid="policy-selected-count"]')?.textContent?.trim();

function type(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillPolicy(name: string, description: string) {
	await vi.waitFor(() => expect(field('policy-name')).toBeTruthy());
	type(field('policy-name')!, name);
	type(area('policy-description')!, description);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('/compliance-policies/new — one page, no wizard footer', () => {
	it('declares a route and offers only the compliance checks', async () => {
		render(NewPolicyPage);
		await fillPolicy('Baseline posture', 'What every laptop must satisfy');
		await vi.waitFor(() => expect(ruleRow(CHECK_A)).toBeTruthy(), { timeout: 3000 });

		expect(shell.pill.context?.route).toBe(ROUTE);
		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
		expect(ruleRow(CHECK_B)).toBeTruthy();

		expect(ruleRow(PLAIN_SHELL)).toBeNull();
	});

	it('creates the policy and adds every picked check with a zero grace period', async () => {
		render(NewPolicyPage);
		await fillPolicy('  Baseline posture  ', '  What every laptop must satisfy  ');
		await vi.waitFor(() => expect(ruleRow(CHECK_B)).toBeTruthy(), { timeout: 3000 });

		ruleRow(CHECK_B)!.click();
		ruleRow(CHECK_A)!.click();
		await vi.waitFor(() => expect(selectedCount()).toBe(m.picker_selected({ count: '2' })));

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createCompliancePolicy).toHaveBeenCalledTimes(1));
		expect(api.createCompliancePolicy.mock.calls[0]).toEqual([
			'Baseline posture',
			'What every laptop must satisfy'
		]);

		await vi.waitFor(() => expect(api.addCompliancePolicyRule).toHaveBeenCalledTimes(2));
		expect(api.addCompliancePolicyRule.mock.calls[0]).toEqual([POLICY_ID, CHECK_B, 0]);
		expect(api.addCompliancePolicyRule.mock.calls[1]).toEqual([POLICY_ID, CHECK_A, 0]);
		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(`/compliance-policies/${POLICY_ID}`)
		);
	});

	it('creates a ruleless policy when nothing is picked, exactly as "Skip" did', async () => {
		render(NewPolicyPage);
		await fillPolicy('Baseline posture', '');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createCompliancePolicy).toHaveBeenCalledTimes(1));
		expect(api.addCompliancePolicyRule).not.toHaveBeenCalled();
	});

	it('blocks the commit at the STORE while the name is missing', async () => {
		render(NewPolicyPage);
		await fillPolicy('Baseline posture', '');

		type(field('policy-name')!, '   ');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(api.createCompliancePolicy).not.toHaveBeenCalled();
	});
});

describe('/compliance-policies/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer — rules included — that still commits', async () => {
		const first = await render(NewPolicyPage);
		await fillPolicy('Baseline posture', 'What every laptop must satisfy');
		await vi.waitFor(() => expect(ruleRow(CHECK_A)).toBeTruthy(), { timeout: 3000 });
		ruleRow(CHECK_A)!.click();
		await vi.waitFor(() => expect(selectedCount()).toBe(m.picker_selected({ count: '1' })));

		expect(stashContext()).toBe('draft:compliance-policy:create');
		expect(shell.drafts[0].route).toBe(ROUTE);
		await new Promise((r) => setTimeout(r, 50));
		expect(pillMode()).toBe('nav');

		await first.unmount();
		setShellPath('/devices');
		const rail = await render(StageRail);
		(document.querySelector('[data-testid="stage-draft"]') as HTMLElement).click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(ROUTE));
		expect(pillMode()).toBe('nav');

		expect(shell.drafts).toHaveLength(0);
		await rail.unmount();

		setShellPath(ROUTE);
		render(NewPolicyPage);

		await vi.waitFor(() => expect(field('policy-name')?.value).toBe('Baseline posture'));
		expect(area('policy-description')?.value).toBe('What every laptop must satisfy');
		await vi.waitFor(() => expect(selectedCount()).toBe(m.picker_selected({ count: '1' })));
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.addCompliancePolicyRule).toHaveBeenCalledTimes(1));
		expect(api.addCompliancePolicyRule.mock.calls[0]).toEqual([POLICY_ID, CHECK_A, 0]);
	});
});

describe('/compliance-policies — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {

		nav.url = new URL('https://control.test/compliance-policies?zoom=list');

		api.listActions.mockResolvedValue({ actions: [], nextPageToken: '' });
		const list = await render(PoliciesPage);

		await vi.waitFor(() => expect(api.search).toHaveBeenCalled(), { timeout: 3000 });

		const createButton = await vi.waitFor(() => {
			const button = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
				(b) => b.textContent?.trim() === m.compliance_policies_create()
			);
			expect(button).toBeTruthy();
			return button!;
		});
		createButton.click();

		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/compliance-policies/new')
		);
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
		await list.unmount();
	});
});
