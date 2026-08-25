

import { apiClient } from '$lib/sdk';
import { DeviceStatus } from '$contract/cadestro/v1/common_pb';
import type { Device, DeviceGroup } from '$contract/cadestro/v1/control_pb';

export const PAGE_SIZE = 100;

export const MAX_PAGES = 20;

const MEMBER_BATCH = 8;

export interface FleetSnapshot {
	devices: Device[];

	total: number;
	truncated: boolean;
	groups: DeviceGroup[];

	membership: Map<string, string[]>;

	groupsError: string | null;
}

async function sweepDevices(myDevicesOnly: boolean): Promise<{ devices: Device[]; total: number; truncated: boolean }> {
	const devices: Device[] = [];
	let total = 0;
	let token = '';
	for (let i = 0; i < MAX_PAGES; i++) {
		const resp = await apiClient.listDevices(PAGE_SIZE, token, DeviceStatus.UNSPECIFIED, {}, myDevicesOnly);
		devices.push(...resp.devices);
		total = resp.totalCount || devices.length;
		token = resp.nextPageToken;
		if (!token) return { devices, total, truncated: false };
	}
	return { devices, total, truncated: true };
}

async function sweepGroups(): Promise<DeviceGroup[]> {
	const groups: DeviceGroup[] = [];
	let token = '';
	for (let i = 0; i < MAX_PAGES; i++) {
		const resp = await apiClient.listDeviceGroups(PAGE_SIZE, token);
		groups.push(...resp.groups);
		token = resp.nextPageToken;
		if (!token) break;
	}
	return groups;
}

async function loadMembership(groups: DeviceGroup[]): Promise<Map<string, string[]>> {
	const membership = new Map<string, string[]>();
	for (let i = 0; i < groups.length; i += MEMBER_BATCH) {
		const batch = groups.slice(i, i + MEMBER_BATCH);
		const resps = await Promise.all(batch.map((g) => apiClient.getDeviceGroup((g.id?.value ?? ''))));
		resps.forEach((resp, n) => membership.set(batch[n].id?.value ?? '', (resp.deviceIds ?? []).map((id) => id.value)));
	}
	return membership;
}

export async function loadFleet(
	opts: { myDevicesOnly?: boolean } = {}
): Promise<FleetSnapshot> {
	const { devices, total, truncated } = await sweepDevices(opts.myDevicesOnly === true);

	let groups: DeviceGroup[] = [];
	let membership = new Map<string, string[]>();
	let groupsError: string | null = null;
	try {
		groups = await sweepGroups();
		membership = await loadMembership(groups);
	} catch (err) {
		groups = [];
		membership = new Map();
		groupsError = err instanceof Error ? err.message : String(err);
		console.error('fleet: device-group reads failed, falling back to one ungrouped bubble', err);
	}

	return { devices, total, truncated, groups, membership, groupsError };
}
