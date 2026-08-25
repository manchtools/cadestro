

import { describe, it, expect } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const projectSettings = join(srcRoot, '..', 'project.inlang', 'settings.json');

function walk(dir: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir)) {

		if (entry === 'paraglide' || entry === 'node_modules') continue;
		const p = join(dir, entry);
		if (statSync(p).isDirectory()) {
			out.push(...walk(p));
		} else if (/\.(ts|svelte)$/.test(entry) && !/\.(test|spec)\.ts$/.test(entry)) {
			out.push(p);
		}
	}
	return out;
}

function scan(re: RegExp): string[] {
	const flags = 'g' + (re.flags.includes('s') ? 's' : '');
	const hits: string[] = [];
	for (const f of files) {
		const src = readFileSync(f, 'utf8');
		const g = new RegExp(re.source, flags);
		let m: RegExpExecArray | null;
		while ((m = g.exec(src)) !== null) {
			const line = src.slice(0, m.index).split('\n').length;
			hits.push(`${relative(srcRoot, f)}:${line}: ${m[0].replace(/\s+/g, ' ')}`);
		}
	}
	return hits;
}

const files = walk(srcRoot);

describe('web source invariants (NIS2 / CLAUDE)', () => {

	it('scans a non-empty set of source files', () => {
		expect(files.length).toBeGreaterThan(0);
	});

	it('G5: no silently-swallowing .catch(() => {}) / .catch(() => undefined)', () => {
		const hits = scan(/\.catch\(\s*(?:async\s*)?\(\s*\)\s*=>\s*(?:\{\s*\}|undefined)\s*\)/s);
		expect(
			hits,
			`empty .catch() swallows errors — log at >= debug instead:\n${hits.join('\n')}`
		).toEqual([]);
	});

	it('G6: no crypto.randomUUID() / uuid package — IDs are ULIDs', () => {
		const hits = scan(/\brandomUUID\s*\(|from\s+['"]uuid['"]|require\(\s*['"]uuid['"]\s*\)|\buuidv4\s*\(/);
		expect(hits, `use a ULID, not randomUUID/uuid:\n${hits.join('\n')}`).toEqual([]);
	});

	it('inlang modules use exact versions', () => {
		const settings = JSON.parse(readFileSync(projectSettings, 'utf8')) as { modules: string[] };
		expect(settings.modules).toHaveLength(3);
		expect(settings.modules.every((module) => /@\d+\.\d+\.\d+\/dist\/index\.js$/.test(module))).toBe(true);
	});
});
