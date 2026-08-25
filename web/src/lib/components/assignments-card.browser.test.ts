import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { AssignmentSourceType } from '$contract/cadestro/v1/common_pb';

const api = vi.hoisted(() => ({
	listAssignments: vi.fn(),
	listDevices: vi.fn(),
	listDeviceGroups: vi.fn(),
	listUsers: vi.fn(),
	listUserGroups: vi.fn(),
	batchCreateAssignments: vi.fn()
}));

vi.mock('$lib/sdk', () => ({
	apiClient: api,
	fetchAllPages: vi.fn(async () => [])
}));

import AssignmentsCard from './assignments-card.svelte';

beforeEach(() => {
	api.listAssignments.mockResolvedValue({ assignments: [] });
	api.listDevices.mockResolvedValue({ devices: [] });
	api.listDeviceGroups.mockResolvedValue({ groups: [] });
	api.listUsers.mockResolvedValue({ users: [] });
	api.listUserGroups.mockResolvedValue({ groups: [] });
});

describe('assignments card', () => {
	it('uses the bindable open value for its assignment dialog', async () => {
		const view = await render(AssignmentsCard, {
			props: {
				assignOpen: false,
				sourceType: AssignmentSourceType.ACTION,
				sourceId: 'action-1',
				title: 'Assignments',
				subtitle: 'Targets',
				assignTitle: 'Assign action',
				assignDescription: 'Choose targets'
			}
		});

		await view.rerender({ assignOpen: true });
		await expect.element(page.getByRole('dialog')).toBeVisible();
		expect(api.listDevices).toHaveBeenCalledTimes(1);
		expect(api.listDeviceGroups).toHaveBeenCalledTimes(1);
	});
});
