

import { apiClient } from '$lib/sdk';
import { fetchAllPages } from '$lib/sdk/paginate';
import { getLocalizedError } from '$lib/errors';
import {
	AssignmentMode,
	AssignmentSourceType,
	AssignmentTargetType
} from '$contract/cadestro/v1/common_pb';
import type { ActionSet, ActionSetMember } from '$contract/cadestro/v1/control_pb';
import type { CarriedDevice } from './eligibility';

export const MAX_CARRIED = 256;

const READ_CONCURRENCY = 8;
const WRITE_CONCURRENCY = 4;

export type AssignSchedule = 'now' | 'on_schedule';

async function mapLimit<T, R>(
	items: readonly T[],
	limit: number,
	fn: (item: T) => Promise<R>
): Promise<R[]> {
	const out = new Array<R>(items.length);
	let next = 0;
	const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
		for (;;) {
			const i = next++;
			if (i >= items.length) return;
			out[i] = await fn(items[i]);
		}
	});
	await Promise.all(workers);
	return out;
}

export async function loadCarriedDevices(ids: readonly string[]): Promise<CarriedDevice[]> {
	return mapLimit(ids, READ_CONCURRENCY, async (id) => {
		try {
			const device = await apiClient.getDevice(id);
			return { id, hostname: device?.hostname ?? '', status: device?.status };
		} catch {

			return { id, hostname: '', status: undefined };
		}
	});
}

export async function loadActionSets(): Promise<ActionSet[]> {
	return fetchAllPages(async (pageSize, pageToken) => {
		const page = await apiClient.listActionSets(pageSize, pageToken);
		return { items: page.sets ?? [], nextPageToken: page.nextPageToken ?? '' };
	});
}

export async function loadSetSteps(setId: string): Promise<ActionSetMember[]> {
	const response = await apiClient.getActionSet(setId);
	return [...(response.members ?? [])].sort((a, b) => a.sortOrder - b.sortOrder);
}

export async function loadAssignedDeviceIds(setId: string): Promise<Set<string>> {
	const assignments = await fetchAllPages(async (pageSize, pageToken) => {
		const page = await apiClient.listAssignments(
			pageSize,
			pageToken,
			AssignmentSourceType.ACTION_SET,
			setId,
			AssignmentTargetType.DEVICE,
			''
		);
		return { items: page.assignments ?? [], nextPageToken: page.nextPageToken ?? '' };
	});
	return new Set(assignments.map((a) => (a.targetId?.value ?? '')));
}

export interface RuleGroup {
	id: string;
	name: string;
}

export async function createRuleGroup(name: string, query: string): Promise<RuleGroup> {
	const group = await apiClient.createDeviceGroup(name, '', true, query);

	if (!group?.id) throw new Error('CreateDeviceGroup returned no group');
	return { id: (group.id?.value ?? ''), name: group.name || name };
}

export async function assignSetToGroup(setId: string, groupId: string): Promise<void> {
	await apiClient.createAssignment(
		AssignmentSourceType.ACTION_SET,
		setId,
		AssignmentTargetType.DEVICE_GROUP,
		groupId,
		AssignmentMode.REQUIRED
	);
}

export interface AssignOutcome {
	deviceId: string;
	ok: boolean;

	error?: string;
}

export async function commitAssign(
	deviceIds: readonly string[],
	setId: string,
	schedule: AssignSchedule
): Promise<AssignOutcome[]> {
	return mapLimit(deviceIds, WRITE_CONCURRENCY, async (deviceId) => {
		try {
			await apiClient.createAssignment(
				AssignmentSourceType.ACTION_SET,
				setId,
				AssignmentTargetType.DEVICE,
				deviceId,
				AssignmentMode.REQUIRED
			);
			if (schedule === 'now') {
				await apiClient.syncDevice(deviceId);
			}
			return { deviceId, ok: true };
		} catch (error) {
			return { deviceId, ok: false, error: getLocalizedError(error) };
		}
	});
}
