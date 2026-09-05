import { describe, expect, it, vi } from 'vitest';
import { addMemberships } from './membership';

describe('membership mutation reconciliation', () => {
 it('removes acknowledged and ambiguously committed additions before retrying', async () => {
  const assigned = new Set<string>();
  const add = vi.fn(async (id: string) => { assigned.add(id); if (id === 'b') throw new Error('response lost'); });
  const first = await addMemberships(['a', 'b', 'c'], add, async () => [...assigned]);
  expect(first.remaining).toEqual(['c']);
  expect(first.ready).toBe(true);
  expect(first.error).toBeInstanceOf(Error);
  const retry = await addMemberships(first.remaining, add, async () => [...assigned]);
  expect(retry.remaining).toEqual([]);
  expect(add.mock.calls.map(([id]) => id)).toEqual(['a', 'b', 'c']);
 });
 it('blocks a retry when the authoritative membership cannot be loaded', async () => {
  const result = await addMemberships(['a'], async () => { throw new Error('response lost'); }, async () => { throw new Error('refresh unavailable'); });
  expect(result.ready).toBe(false);
  expect(result.remaining).toEqual(['a']);
 });
});
