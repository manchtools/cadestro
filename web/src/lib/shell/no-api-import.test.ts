

import { describe, it, expect } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const ROOTS = ['src/lib/shell', 'src/lib/components/shell', 'src/lib/components/fleet'];

const FORBIDDEN = [/\$lib\/sdk/, /\bauthStore\b/, /@connectrpc\//, /\$lib\/api\b/, /createClient\b/];

function walk(dir: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...walk(p));

		else if (/\.(svelte|ts)$/.test(entry.name) && !/\.(test|spec)\.ts$/.test(entry.name)) out.push(p);
	}
	return out;
}

describe('shell preview stays API-free (AC-9)', () => {
	const files = ROOTS.flatMap(walk);

	it('discovers shell source files to scan', () => {
		expect(files.length).toBeGreaterThan(0);
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
