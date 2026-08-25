

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { ComplianceStatus } from '$contract/cadestro/v1/common_pb';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/compliance-policies'),
	search: vi.fn(),
	listDevices: vi.fn(),
	listCompliancePolicies: vi.fn(),
	goto: vi.fn()
}));

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return mocks.url;
		},

		get state() {
			return {};
		}
	}
}));
vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));
vi.mock('$lib/navigation', () => ({
	goto: (path: string) => mocks.goto(path),
	pushState: vi.fn(),
	replaceState: vi.fn()
}));
vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');

	const { fetchAllPages } = await import('$lib/sdk/paginate');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			search: mocks.search,
			listDevices: mocks.listDevices,
			listCompliancePolicies: mocks.listCompliancePolicies,
			deleteCompliancePolicy: vi.fn(),
			getCompliancePolicy: vi.fn(async () => ({ policy: null })),
			listActions: vi.fn(async () => ({ actions: [], nextPageToken: '' }))
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages
	};
});

import CompliancePoliciesPage from './+page.svelte';

const DEVICES = [
	{ id: { value: '01HZDEVCOMPLIANT000000000A' }, hostname: 'ok-01', complianceStatus: ComplianceStatus.COMPLIANT },
	{
		id: { value: '01HZDEVVIOLATION000000000B' },
		hostname: 'drift-01',
		complianceStatus: ComplianceStatus.NON_COMPLIANT
	},
	{ id: { value: '01HZDEVUNKNOWN0000000000C' }, hostname: 'new-01', complianceStatus: ComplianceStatus.UNKNOWN }
];
const POLICY = {
	id: { value: '01HZPOLICY000000000000000A' },
	name: 'CIS baseline',
	description: '',
	rules: [],
	ruleCount: 2
};

const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="compliance-tile"]'));

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	mocks.url = new URL('http://localhost/compliance-policies');
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listDevices.mockResolvedValue({ devices: DEVICES, nextPageToken: '', totalCount: 3 });
	mocks.listCompliancePolicies.mockResolvedValue({ policies: [POLICY], nextPageToken: '' });
});

describe('/compliance-policies — the overview is the landing level', () => {
	it('lands on the device-tile grid coloured by compliance state, never colour-alone', async () => {
		render(CompliancePoliciesPage);

		await vi.waitFor(() => expect(mocks.listDevices).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(() => expect(tiles()).toHaveLength(3), { timeout: 3000 });

		const byId = (id: string) => tiles().find((t) => t.dataset.deviceId === id)!;
		const compliant = byId(DEVICES[0].id.value);
		const violating = byId(DEVICES[1].id.value);
		const unknown = byId(DEVICES[2].id.value);

		expect(compliant.dataset.tone).toBe('ok');
		expect(violating.dataset.tone).toBe('warn');
		expect(unknown.dataset.tone).toBe('idle');

		expect(violating.querySelector('[data-marker="dot"]')).not.toBeNull();
		expect(compliant.querySelector('[data-marker="dot"]')).toBeNull();
		expect(compliant.getAttribute('aria-label')).toContain('Compliant');
		expect(violating.getAttribute('aria-label')).toContain('Non-Compliant');
		expect(unknown.getAttribute('aria-label')).toContain('Unknown');

		const chip = document.querySelector<HTMLElement>('[data-testid="compliance-policy-chip"]');
		expect(chip).not.toBeNull();
		expect(chip!.textContent).toContain('CIS baseline');
		expect(chip!.textContent).toContain('2 rules');

		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('a tile opens the device\'s existing compliance view', async () => {
		render(CompliancePoliciesPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(3), { timeout: 3000 });

		tiles()
			.find((t) => t.dataset.deviceId === DEVICES[1].id.value)!
			.click();

		expect(mocks.goto).toHaveBeenCalledWith(`/devices/${DEVICES[1].id.value}?tab=compliance`);
	});

	it('the level pill zooms to the existing policy list one level down', async () => {
		render(CompliancePoliciesPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(3), { timeout: 3000 });

		document.querySelector<HTMLButtonElement>('[data-testid="compliance-zoom-list"]')!.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull(),
			{ timeout: 3000 }
		);
	});
});
