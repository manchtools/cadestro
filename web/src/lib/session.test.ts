import { describe, expect, it } from 'vitest';
import { safeRedirect } from './session';

describe('safeRedirect', () => {
	it('accepts local paths and rejects external paths', () => {
		expect(safeRedirect('/devices?online=true')).toBe('/devices?online=true');
		expect(safeRedirect('//evil.example')).toBe('/');
		expect(safeRedirect('/\\evil.example')).toBe('/');
	});
});
