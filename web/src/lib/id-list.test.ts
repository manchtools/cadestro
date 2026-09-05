import { expect, it } from 'vitest';
import { parseIds } from './id-list';
it('normalizes manual membership targets without duplicate mutations', () => {
 expect(parseIds(' device-a , device-b, ,device-a,')).toEqual(['device-a', 'device-b']);
});
