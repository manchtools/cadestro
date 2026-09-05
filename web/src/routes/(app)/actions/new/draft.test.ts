import { describe, expect, it } from 'vitest';
import { draftErrors, draftForType, hydrate, serialize } from './draft';
import { DesiredState } from '$contract/cadestro/v1/common_pb';
describe('guided action drafts', () => {
 it('preserves the complete compliance shell configuration across stashing', () => {
  const draft = draftForType('COMPLIANCE_CHECK');
  if (draft.params.case !== 'shell') throw new Error('Expected shell params');
  draft.name = 'Check baseline';
  draft.params.value.detectionScript = 'test -f /etc/baseline';
  draft.params.value.script = 'touch /etc/baseline';
  draft.params.value.environment = { MODE: 'strict', VALUE: 'a=b' };
  draft.params.value.workingDirectory = '/var/lib';
  draft.timeoutSeconds = 60;
  draft.schedule = { $typeName: 'cadestro.v1.ActionSchedule', intervalHours: 12 };
  const restored = hydrate(serialize(draft));
  expect(restored).toEqual(draft);
  expect(draft.params.value.isCompliance).toBe(true);
  expect(draftErrors(draft)).toEqual({});
 });
 it('keeps package absence and version while rejecting incomplete input', () => {
  const draft = draftForType('PACKAGE');
  expect(draftErrors(draft)).toMatchObject({ name: expect.any(String), packageName: expect.any(String) });
  if (draft.params.case !== 'package') throw new Error('Expected package params');
  draft.name = 'Remove package'; draft.params.value.name = 'example'; draft.params.value.version = '1.2'; draft.desiredState = DesiredState.ABSENT;
  expect(hydrate(serialize(draft))).toEqual(draft);
  expect(draftErrors(draft)).toEqual({});
 });
 it('requires a detection script for compliance and rejects corrupt parked data', () => {
  const draft = draftForType('COMPLIANCE_CHECK');
  if (draft.params.case !== 'shell') throw new Error('Expected shell params');
  draft.params.value.script = 'echo remediation';
  expect(draftErrors(draft).detectionScript).toBeTruthy();
  expect(hydrate('{')).toBeNull();
  expect(hydrate({ name: 'old shape' })).toBeNull();
 });
});
