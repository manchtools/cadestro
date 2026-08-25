

import { describe, it, expect, afterAll } from 'vitest';
import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { baseLocale, locales, overwriteGetLocale } from '$lib/paraglide/runtime';
import { PRIMARY_SECTIONS, OVERFLOW_GROUPS, filterNav, filterGroups } from './nav';
import {
	PANEL_KIND_PLURAL,
	PANEL_KIND_SINGULAR,
	panelKindPlural,
	panelKindSingular
} from './panel-labels';

const APP_ROUTES = 'src/routes/(app)';

const NON_NAV_ROUTES = ['/assign'];

function discoverSections(dir: string, prefix = ''): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		if (!entry.isDirectory()) continue;
		if (entry.name.includes('[') || entry.name === 'new' || entry.name.startsWith('__')) continue;
		const href = `${prefix}/${entry.name}`;
		const child = join(dir, entry.name);
		const files = readdirSync(child, { withFileTypes: true });
		if (files.some((f) => f.isFile() && f.name === '+page.svelte')) out.push(href);
		out.push(...discoverSections(child, href));
	}
	return out;
}

const discovered = discoverSections(APP_ROUTES);
const overflowEntries = OVERFLOW_GROUPS.flatMap((g) => g.items);
const allEntries = [...PRIMARY_SECTIONS, ...overflowEntries];
const allHrefs = allEntries.map((e) => e.href);

describe('nav tables — the pill row', () => {
	it('discovers the (app) route tree', () => {
		expect(discovered.length).toBeGreaterThan(0);
	});

	it('carries exactly the five round-2 operator sections', () => {
		expect(PRIMARY_SECTIONS.map((s) => s.href)).toEqual([
			'/devices',
			'/actions',
			'/compliance-policies',
			'/audit'
		]);
	});

	it('keeps the admin overflow at users · roles · groups · tokens · IdP · terminal sessions', () => {

		const admin = OVERFLOW_GROUPS.find((g) => g.items.some((i) => i.href === '/users'));
		expect(admin).toBeDefined();

		expect(admin!.items.slice(0, 6).map((i) => i.href)).toEqual([
			'/users',
			'/roles',
			'/user-groups',
			'/tokens',
			'/identity-providers',
			'/admin/terminal-sessions'
		]);
	});
});

describe('nav tables — no destination is lost', () => {
	it('gives every routable section exactly one home', () => {
		const missing = discovered.filter(
			(href) => !allHrefs.includes(href) && !NON_NAV_ROUTES.includes(href)
		);
		expect(missing).toEqual([]);
	});

	it('exempts only routes that still exist', () => {
		expect(NON_NAV_ROUTES.filter((href) => !discovered.includes(href))).toEqual([]);
		expect(NON_NAV_ROUTES.length).toBeGreaterThan(0);
	});

	it('points at no route that does not exist', () => {
		const dangling = allHrefs.filter((href) => !discovered.includes(href));
		expect(dangling).toEqual([]);
	});

	it('lists no destination twice', () => {
		expect([...new Set(allHrefs)]).toHaveLength(allHrefs.length);
	});
});

describe('nav tables — permission gating', () => {
	it('drops entries whose permission is denied and keeps the always-visible ones', () => {
		const denyAll = filterNav(allEntries, () => false);
		expect(denyAll.map((e) => e.href)).toEqual(
			allEntries.filter((e) => e.permission === null).map((e) => e.href)
		);
		expect(denyAll.length).toBeGreaterThan(0);
	});

	it('asks for exactly the permission each entry declares', () => {
		const asked: string[] = [];
		filterNav(allEntries, (p) => (asked.push(p), true));
		expect(asked).toEqual(allEntries.filter((e) => e.permission !== null).map((e) => e.permission));
	});

	it('drops a group that ends up empty rather than rendering an empty heading', () => {
		const groups = filterGroups(OVERFLOW_GROUPS, (p) => p === 'ListUsers');
		expect(groups.every((g) => g.items.length > 0)).toBe(true);
		expect(groups.flatMap((g) => g.items).map((i) => i.href)).toContain('/users');
		expect(groups.flatMap((g) => g.items).map((i) => i.href)).not.toContain('/roles');
	});
});

describe('nav tables — labels follow the locale', () => {
	const labelled = [
		...PRIMARY_SECTIONS.map((s) => ({ what: `section ${s.href}`, label: s.label })),
		...OVERFLOW_GROUPS.flatMap((g, i) => [
			{ what: `overflow group #${i} heading`, label: g.group },
			...g.items.map((item) => ({ what: `overflow ${item.href}`, label: item.label }))
		])
	];

	afterAll(() => overwriteGetLocale(() => baseLocale));

	it('finds every entry label and every group heading', () => {
		expect(labelled).toHaveLength(allEntries.length + OVERFLOW_GROUPS.length);
		expect(labelled.length).toBeGreaterThan(0);
		expect(locales.length).toBeGreaterThan(1);
	});

	it('carries message functions, never pre-resolved strings', () => {
		const literals = labelled.filter((l) => typeof l.label !== 'function').map((l) => l.what);
		expect(literals, `these labels are frozen strings: ${literals.join(', ')}`).toEqual([]);
	});

	it('resolves every label to a non-empty string in every locale', () => {
		const blank: string[] = [];
		for (const locale of locales) {
			overwriteGetLocale(() => locale);
			for (const l of labelled) {
				const text = l.label();
				if (typeof text !== 'string' || text.trim() === '') blank.push(`${l.what} (${locale})`);
			}
		}
		expect(blank, `missing translations: ${blank.join(', ')}`).toEqual([]);
	});

	it('reads the locale at CALL time, so switching language changes the text', () => {

		const devices = PRIMARY_SECTIONS.find((s) => s.href === '/devices');
		expect(devices).toBeDefined();
		overwriteGetLocale(() => 'en');
		expect(devices!.label()).toBe('Devices');
		overwriteGetLocale(() => 'de');
		expect(devices!.label()).toBe('Geräte');
	});
});

describe('stage-rail panel-kind captions', () => {
	const kinds = Object.keys(PANEL_KIND_PLURAL);

	afterAll(() => overwriteGetLocale(() => baseLocale));

	it('names the same kinds in both the plural and singular maps', () => {
		expect(kinds.length).toBeGreaterThan(0);
		expect(Object.keys(PANEL_KIND_SINGULAR).sort()).toEqual([...kinds].sort());
	});

	it('resolves every caption to a non-empty string in every locale', () => {
		const blank: string[] = [];
		for (const locale of locales) {
			overwriteGetLocale(() => locale);
			for (const kind of kinds) {
				if (!panelKindPlural(kind).trim()) blank.push(`${kind} plural (${locale})`);
				if (!panelKindSingular(kind).trim()) blank.push(`${kind} singular (${locale})`);
			}
		}
		expect(blank, `missing translations: ${blank.join(', ')}`).toEqual([]);
	});

	it('falls back to the raw kind for a kind nobody named', () => {

		expect(kinds).not.toContain('pod');
		expect(panelKindPlural('pod')).toBe('Pods');
		expect(panelKindSingular('pod')).toBe('pod');
	});
});
