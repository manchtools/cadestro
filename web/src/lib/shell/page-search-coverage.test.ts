// Structural guard for lane PILL-SEARCH: search belongs to the pill.
//
// Every list surface that owns a query (createSearchListState /
// createClientListState) must publish it through the page-search seam, and none
// of them may keep a search box of its own — otherwise ⌘K silently falls back
// to the global palette on that page, or two boxes drive the same list.
//
// Self-discovering (a new list page is covered the day it lands) with a
// matches-zero guard, in the style of no-api-import.test.ts.
import { describe, it, expect } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..'); // .../src
const routes = join(srcRoot, 'routes');

function walk(dir: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...walk(p));
		else if (entry.name.endsWith('.svelte')) out.push(p);
	}
	return out;
}

const LIST_FACTORY = /create(Search|Client)ListState\s*</;
/** A search box bound to the list's own query — the thing that moved. */
const OWN_SEARCH_BOX = /setSearch\(e\.currentTarget\.value\)/;

const listPages = walk(routes).filter((f) => LIST_FACTORY.test(readFileSync(f, 'utf8')));

describe('list pages publish their query to the pill (PILL-SEARCH)', () => {
	it('discovers list surfaces to check', () => {
		expect(listPages.length).toBeGreaterThan(0); // matches-zero guard
	});

	it('every list surface registers itself as the pill’s search scope', () => {
		const missing = listPages
			.filter((f) => !readFileSync(f, 'utf8').includes('registerPageSearch('))
			.map((f) => relative(srcRoot, f));
		expect(
			missing,
			`these list surfaces own a query but never publish it, so ⌘K cannot search them:\n${missing.join('\n')}`
		).toEqual([]);
	});

	it('no list surface keeps a search box of its own', () => {
		const offenders = listPages
			.filter((f) => OWN_SEARCH_BOX.test(readFileSync(f, 'utf8')))
			.map((f) => relative(srcRoot, f));
		expect(
			offenders,
			`search moved into the pill — remove the in-page box:\n${offenders.join('\n')}`
		).toEqual([]);
	});
});
