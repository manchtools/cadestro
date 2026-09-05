import { beforeEach, describe, expect, it, vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { Permission, RoleSchema } from '$contract/cadestro/v1/control_pb';
import { saveRoleEdits } from './role-editor';
const rpc = vi.hoisted(() => ({ getRole: vi.fn(), listRoles: vi.fn(), renameRole: vi.fn(), setRoleDescription: vi.fn(), grantRolePermission: vi.fn(), revokeRolePermission: vi.fn() }));
vi.mock('$lib/api', () => ({ api: rpc }));
beforeEach(() => vi.resetAllMocks());
describe('role editor authoritative baseline', () => {
 it('retains a committed grant after a lost response so retry only applies outstanding edits', async () => {
  const role = create(RoleSchema, { id: { value: 'role' }, name: 'Operators' });
  const current = create(RoleSchema, { ...role, permissions: [Permission.LIST_DEVICES] });
  rpc.grantRolePermission.mockRejectedValueOnce(new Error('response lost')).mockResolvedValueOnce({ role: create(RoleSchema, { ...current, permissions: [Permission.LIST_DEVICES, Permission.GET_DEVICE] }) });
  rpc.listRoles.mockResolvedValue({ roles: [current], nextPageToken: '' });
  const desired = [Permission.LIST_DEVICES, Permission.GET_DEVICE];
  const first = await saveRoleEdits(role, role.name, '', desired, permission => permission === Permission.LIST_ROLES);
  expect(first.error).toBeInstanceOf(Error);
  expect(first.role?.permissions).toEqual([Permission.LIST_DEVICES]);
  if (!first.role) throw new Error('Expected reconciled role');
  const retry = await saveRoleEdits(first.role, role.name, '', desired, permission => permission === Permission.LIST_ROLES);
  expect(retry.error).toBeNull();
  expect(rpc.grantRolePermission.mock.calls.map(([request]) => request.permission)).toEqual(desired);
  expect(rpc.getRole).not.toHaveBeenCalled();
 });
 it('marks the baseline unavailable after mutation and reload failures', async () => {
  const role = create(RoleSchema, { id: { value: 'role' }, name: 'Operators' });
  rpc.grantRolePermission.mockRejectedValue(new Error('response lost'));
  rpc.getRole.mockRejectedValue(new Error('refresh failed'));
  const result = await saveRoleEdits(role, role.name, '', [Permission.LIST_DEVICES], () => true);
  expect(result.role).toBeNull();
  expect(result.error).toBeInstanceOf(Error);
 });
});
