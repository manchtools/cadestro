

import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';

const LAYOUT = 'src/routes/(app)/+layout.svelte';

describe('pill height reservation (flicker regression)', () => {
	const src = readFileSync(LAYOUT, 'utf8');
	const mainTag = /<main[^>]*>/s.exec(src)?.[0];

	it('the layout still reserves the measured pill height', () => {

		expect(mainTag, '<main> not found in layout').toBeTruthy();
		expect(mainTag).toContain('var(--pill-block');
	});

	it('never animates the measured reservation — only the stage-rail inset may transition', () => {
		expect(mainTag).toBeTruthy();

		expect(mainTag).not.toMatch(/transition-\[padding\]/);

		expect(mainTag).toMatch(/transition-\[padding-right\]/);
	});
});
