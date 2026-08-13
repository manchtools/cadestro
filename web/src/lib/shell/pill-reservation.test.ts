// The pill-reservation contract between the shell chrome and the app layout.
//
// MorphBar publishes the pill column's MEASURED height as `--pill-block` from a
// ResizeObserver (morph-bar.svelte, the `column` effect). That observer fires
// for every transient height while the pill's own 220ms width/height morph is
// running and whenever the caption strip appears, disappears or rewraps — a
// continuous stream of values, not one settled number.
//
// The layout's <main> reserves `padding-top: calc(… var(--pill-block) …)`. If
// that padding is ALSO animated (a `transition-[padding]` utility), every
// republished measurement RESTARTS a 200ms transition toward a target that is
// itself still moving — a transition restarted per observer tick produces the
// stepped, stuttering shift of the whole content area the operator reported as
// "the pill flickers like crazy between state changes" (regressed in 3aeb5e2,
// which introduced the measured reservation onto an element that already
// carried `transition-[padding]` for the stage rail).
//
// The contract pinned here: the reservation FOLLOWS the measurement instantly —
// the pill's own morph stays the single source of motion and the page tracks
// it frame-locked. Only padding-right (the stage-rail inset, a one-shot
// discrete change) may animate.
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';

const LAYOUT = 'src/routes/(app)/+layout.svelte';

describe('pill height reservation (flicker regression)', () => {
	const src = readFileSync(LAYOUT, 'utf8');
	const mainTag = /<main[^>]*>/s.exec(src)?.[0];

	it('the layout still reserves the measured pill height', () => {
		// matches-zero guard: if the reservation moves, this test must move with it.
		expect(mainTag, '<main> not found in layout').toBeTruthy();
		expect(mainTag).toContain('var(--pill-block');
	});

	it('never animates the measured reservation — only the stage-rail inset may transition', () => {
		expect(mainTag).toBeTruthy();
		// The all-paddings utility re-animates padding-top on every ResizeObserver
		// publish of --pill-block: 200ms transitions restarted per tick toward a
		// moving target. That is the flicker.
		expect(mainTag).not.toMatch(/transition-\[padding\]/);
		// The stage rail's pr inset still glides — a real, discrete change.
		expect(mainTag).toMatch(/transition-\[padding-right\]/);
	});
});
