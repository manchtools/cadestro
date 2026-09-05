import { describe, expect, it } from 'vitest';
import { Permission } from '$contract/cadestro/v1/control_pb';
import { PRIMARY_SECTIONS, OVERFLOW_GROUPS, filterNav, filterGroups } from './nav';
describe('permission filtered historical navigation', () => {
 it('keeps independently authorized creation entry points', () => {
  expect(filterNav(PRIMARY_SECTIONS, permission => permission === Permission.CREATE_ACTION).map(entry => entry.href)).toEqual(['/actions']);
  const items = filterGroups(OVERFLOW_GROUPS, permission => [Permission.CREATE_ROLE, Permission.CREATE_TOKEN, Permission.CREATE_DEVICE_GROUP, Permission.CREATE_IDENTITY_PROVIDER, Permission.CREATE_ASSIGNMENT].includes(permission)).flatMap(group => group.items).map(entry => entry.href);
  expect(items).toEqual(['/device-groups', '/assignments', '/roles', '/tokens', '/identity-providers']);
 });
 it('removes empty groups and inaccessible sections', () => {
  expect(filterGroups(OVERFLOW_GROUPS, () => false)).toEqual([]);
  expect(filterNav(PRIMARY_SECTIONS, () => false)).toEqual([]);
 });
});
