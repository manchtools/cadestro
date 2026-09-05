import { api } from '$lib/api';
import { collectPages } from '$lib/console';
import { Permission, type Device, type DeviceGroup } from '$contract/cadestro/v1/control_pb';

export type FleetSnapshot = {
 devices: Device[];
 groups: DeviceGroup[];
 membership: Map<string, string[]>;
 total: number;
 truncated: boolean;
 groupsError: boolean;
};

export async function loadFleet(can: (permission: Permission) => boolean): Promise<FleetSnapshot> {
 const devices = can(Permission.LIST_DEVICES) ? await collectPages(async (pageToken) => {
  const response = await api.listDevices({ pageSize: 100, pageToken });
  return { items: response.devices, nextPageToken: response.nextPageToken };
 }) : [];
 const snapshot: FleetSnapshot = { devices, groups: [], membership: new Map(), total: devices.length, truncated: false, groupsError: false };
 if (!can(Permission.LIST_DEVICE_GROUPS) || !can(Permission.GET_DEVICE_GROUP)) return snapshot;
 try {
  snapshot.groups = await collectPages(async (pageToken) => {
   const response = await api.listDeviceGroups({ pageSize: 100, pageToken });
   return { items: response.groups, nextPageToken: response.nextPageToken };
  });
  for (const group of snapshot.groups) {
   const detail = await api.getDeviceGroup({ id: group.id });
   snapshot.membership.set(group.id?.value ?? '', detail.devices.flatMap((member) => member.deviceId ? [member.deviceId.value] : []));
  }
 } catch {
  snapshot.groupsError = true;
 }
 return snapshot;
}
