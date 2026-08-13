// The device overview's recent-activity table must cost ONE list call.
//
// It used to follow ListExecutions with a sequential GetAction per distinct
// action, purely to read a name the list response already carries. Opening one
// device page could therefore issue 21 requests, which is enough to trip the
// authenticated rate limiter and fail the very table it was decorating — every
// row then falling back to a truncated ULID.
//
// What this pins is the shape, not the symptom: the names come from the
// execution rows, and no per-row lookup is issued at all.
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ActionExecutionSchema, DeviceSchema } from '$sdk/powermanage/v1/control_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import { DeviceStatus, ExecutionStatus } from '$sdk/powermanage/v1/common_pb';

const DEVICE_ID = '01JQZZDEVICE00000000000000';

const api = vi.hoisted(() => ({
	listExecutions: vi.fn(),
	listDeviceAssignees: vi.fn(),
	// Present so a reintroduced lookup is caught as an assertion, not as an
	// "api.getAction is not a function" crash that reads like harness breakage.
	getAction: vi.fn()
}));

vi.mock('$lib/sdk', async (importOriginal) => ({
	...((await importOriginal()) as object),
	apiClient: api
}));

import OverviewTab from './overview-tab.svelte';

/** Two executions of the SAME action plus one inline action with no stored row
 *  — the case the removed loop deduplicated, and the case it could not name. */
function executions() {
	return [
		create(ActionExecutionSchema, {
			id: '01JQZZEXEC00000000000000A0',
			deviceId: DEVICE_ID,
			actionId: '01JQZZACTION0000000000000A',
			actionName: 'Install curl',
			type: ActionType.PACKAGE,
			status: ExecutionStatus.SUCCESS
		}),
		create(ActionExecutionSchema, {
			id: '01JQZZEXEC00000000000000B0',
			deviceId: DEVICE_ID,
			actionId: '01JQZZACTION0000000000000A',
			actionName: 'Install curl',
			type: ActionType.PACKAGE,
			status: ExecutionStatus.FAILED
		}),
		create(ActionExecutionSchema, {
			id: '01JQZZEXEC00000000000000C0',
			deviceId: DEVICE_ID,
			// Inline dispatch: no stored action, so the server resolves no name.
			actionId: '',
			actionName: '',
			type: ActionType.REBOOT,
			status: ExecutionStatus.PENDING
		})
	];
}

function device() {
	return create(DeviceSchema, {
		id: DEVICE_ID,
		hostname: 'workstation-01',
		status: DeviceStatus.ONLINE
	});
}

function mount() {
	return render(OverviewTab, {
		props: {
			device: device(),
			deviceId: DEVICE_ID,
			inventory: [],
			refreshKey: 0,
			ondeviceupdate: () => {}
		}
	});
}

beforeEach(() => {
	vi.clearAllMocks();
	api.listExecutions.mockResolvedValue({ executions: executions(), nextPageToken: '' });
	api.listDeviceAssignees.mockResolvedValue([]);
	api.getAction.mockResolvedValue(null);
});

describe('recent activity costs one call', () => {
	it('names rows from the execution rows, never a per-row lookup', async () => {
		mount();

		await vi.waitFor(() => expect(document.body.textContent).toContain('Install curl'));

		// The whole point: the list response was already sufficient.
		expect(api.getAction).not.toHaveBeenCalled();
		expect(api.listExecutions).toHaveBeenCalledTimes(1);
	});

	it('falls back to the type label for an inline action with no stored row', async () => {
		mount();

		// `action_name` is empty for inline dispatches. Naming that row after a
		// truncated ULID (what the old lookup produced when it 404ed or was rate
		// limited) tells the operator nothing.
		await vi.waitFor(() => expect(document.body.textContent).toContain('Install curl'));
		expect(document.body.textContent).not.toContain('01JQZZEX');
		expect(api.getAction).not.toHaveBeenCalled();
	});
});
