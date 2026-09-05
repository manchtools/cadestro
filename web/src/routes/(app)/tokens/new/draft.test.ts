import { describe, expect, it } from 'vitest';
import { draftErrors, hydrate } from './draft';
describe('registration token validity across stashing', () => {
 it('preserves invalid expiration so zero cannot silently become an indefinite or seven-day token', () => {
  const draft = hydrate({ name: 'Enrollment', maxUses: 0, expiresInDays: 0 });
  expect(draft?.expiresInDays).toBe(0);
  if (!draft) throw new Error('Expected restored draft');
  expect(draftErrors(draft).expiresInDays).toBeTruthy();
 });
 it('rejects fractional days and non-int32 use limits', () => {
  expect(draftErrors({ name: 'Enrollment', maxUses: 1.5, expiresInDays: 0.5 })).toMatchObject({ maxUses: expect.any(String), expiresInDays: expect.any(String) });
  expect(draftErrors({ name: 'Enrollment', maxUses: 2147483648, expiresInDays: 7 }).maxUses).toBeTruthy();
  expect(draftErrors({ name: 'Enrollment', maxUses: 0, expiresInDays: 7 })).toEqual({});
 });
});
