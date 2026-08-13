// Chrome-stays-API-free guard:
// shell CHROME modules never import an SDK/auth/Connect-RPC client — data
// reaches them only as props/plain view-models from the owning route (the
// adaptor seam). Route files themselves are data-bearing and NOT in this set.
// Structural invariant enforced by scanning source; self-discovering with a
// matches-zero guard, so a moved/renamed file can't make it silently pass.
import { describe, it, expect } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const ROOTS = ['src/lib/shell', 'src/lib/components/shell', 'src/lib/components/fleet'];

// Anything that would pull a network/auth surface into the chrome-only slice.
const FORBIDDEN = [/\$lib\/sdk/, /\bauthStore\b/, /@connectrpc\//, /\$lib\/api\b/, /createClient\b/];

function walk(dir: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...walk(p));
		// scan shippable source only — a test file naming the forbidden patterns
		// (like this one) must not count as an offender.
		else if (/\.(svelte|ts)$/.test(entry.name) && !/\.(test|spec)\.ts$/.test(entry.name)) out.push(p);
	}
	return out;
}

describe('shell preview stays API-free (AC-9)', () => {
	const files = ROOTS.flatMap(walk);

	it('discovers shell source files to scan', () => {
		expect(files.length).toBeGreaterThan(0); // matches-zero guard
	});

	it('no shell file imports an API/auth client', () => {
		const offenders: string[] = [];
		for (const f of files) {
			const src = readFileSync(f, 'utf8');
			for (const rx of FORBIDDEN) {
				if (rx.test(src)) offenders.push(`${f} :: ${rx}`);
			}
		}
		expect(offenders).toEqual([]);
	});
});
