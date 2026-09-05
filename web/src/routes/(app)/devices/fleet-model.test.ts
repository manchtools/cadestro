import { describe, expect, it } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { DeviceSchema } from '$contract/cadestro/v1/control_pb';
import { ComplianceStatus, DeviceStatus } from '$contract/cadestro/v1/common_pb';
import { deviceTone, summarize, toFleetDevice, buildBubbles, UNGROUPED_ID } from './fleet-model';
describe('historical fleet classification and groups', () => {
 it('prioritizes absence of a heartbeat and reachability before compliance', () => {
  const device = create(DeviceSchema, { status: DeviceStatus.OFFLINE, complianceStatus: ComplianceStatus.NON_COMPLIANT });
  expect(deviceTone(device)).toBe('idle');
  device.lastSeenAt = { $typeName: 'google.protobuf.Timestamp', seconds: 10n, nanos: 0 };
  expect(deviceTone(device)).toBe('crit');
  device.status = DeviceStatus.ONLINE;
  expect(deviceTone(device)).toBe('warn');
 });
 it('preserves overlapping membership without duplicating the fleet totals', () => {
  const devices = ['a', 'b'].map(id => toFleetDevice(create(DeviceSchema, { id: { value: id }, hostname: id }), 20));
  const bubbles = buildBubbles(devices, [{ id: 'one', name: 'One' }, { id: 'two', name: 'Two' }], new Map([['one', ['a']], ['two', ['a']]]), 'Ungrouped');
  expect(summarize(devices).total).toBe(2);
  expect(bubbles.find(group => group.id === UNGROUPED_ID)?.devices.map(device => device.id)).toEqual(['b']);
  expect(bubbles.filter(group => group.id !== UNGROUPED_ID).map(group => group.devices.map(device => device.id))).toEqual([['a'], ['a']]);
 });
});
