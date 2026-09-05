import { beforeEach, describe, expect, it, vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { DeviceSchema, DeviceGroupSchema, Permission } from '$contract/cadestro/v1/control_pb';
import { loadFleet } from './fleet-data';
const rpc = vi.hoisted(() => ({ listDevices: vi.fn(), listDeviceGroups: vi.fn(), getDeviceGroup: vi.fn() }));
vi.mock('$lib/api', () => ({ api: rpc }));
beforeEach(() => { vi.resetAllMocks(); });
const granted = (...permissions: Permission[]) => (permission: Permission) => permissions.includes(permission);
describe('fleet retained API access', () => {
 it('sweeps opaque pages for devices and groups before grouping every member', async () => {
  rpc.listDevices.mockResolvedValueOnce({ devices: [create(DeviceSchema, { id: { value: 'one' } })], nextPageToken: 'device-page' }).mockResolvedValueOnce({ devices: [create(DeviceSchema, { id: { value: 'two' } })], nextPageToken: '' });
  rpc.listDeviceGroups.mockResolvedValueOnce({ groups: [create(DeviceGroupSchema, { id: { value: 'a' } })], nextPageToken: 'group-page' }).mockResolvedValueOnce({ groups: [create(DeviceGroupSchema, { id: { value: 'b' } })], nextPageToken: '' });
  rpc.getDeviceGroup.mockResolvedValueOnce({ devices: [{ deviceId: { value: 'one' } }] }).mockResolvedValueOnce({ devices: [{ deviceId: { value: 'two' } }] });
  const snapshot = await loadFleet(granted(Permission.LIST_DEVICES, Permission.LIST_DEVICE_GROUPS, Permission.GET_DEVICE_GROUP));
  expect(snapshot.devices.map(device => device.id?.value)).toEqual(['one', 'two']);
  expect(snapshot.membership).toEqual(new Map([['a', ['one']], ['b', ['two']]]));
  expect(rpc.listDevices).toHaveBeenNthCalledWith(2, { pageSize: 100, pageToken: 'device-page' });
  expect(rpc.listDeviceGroups).toHaveBeenNthCalledWith(2, { pageSize: 100, pageToken: 'group-page' });
 });
 it('does not request group membership without GET_DEVICE_GROUP', async () => {
  rpc.listDevices.mockResolvedValue({ devices: [create(DeviceSchema)], nextPageToken: '' });
  const snapshot = await loadFleet(granted(Permission.LIST_DEVICES, Permission.LIST_DEVICE_GROUPS));
  expect(snapshot.devices).toHaveLength(1);
  expect(rpc.getDeviceGroup).not.toHaveBeenCalled();
  expect(rpc.listDeviceGroups).not.toHaveBeenCalled();
 });
 it('keeps loaded devices and a warning when a group lookup fails', async () => {
  rpc.listDevices.mockResolvedValue({ devices: [create(DeviceSchema)], nextPageToken: '' });
  rpc.listDeviceGroups.mockRejectedValue(new Error('lookup failed'));
  const snapshot = await loadFleet(granted(Permission.LIST_DEVICES, Permission.LIST_DEVICE_GROUPS, Permission.GET_DEVICE_GROUP));
  expect(snapshot.devices).toHaveLength(1);
  expect(snapshot.groupsError).toBe(true);
 });
});
