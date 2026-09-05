

import { describe, it, expect } from 'vitest';
import { codecs, readURLParam } from './url-state';

describe('codecs.string', () => {
	const c = codecs.string('default');
	it('parse: returns default for null', () => {
		expect(c.parse(null)).toBe('default');
	});
	it('parse: passes value through', () => {
		expect(c.parse('foo')).toBe('foo');
	});
	it('serialize: omits the default value', () => {
		expect(c.serialize('default')).toBeNull();
	});
	it('serialize: passes non-default through', () => {
		expect(c.serialize('foo')).toBe('foo');
	});
});

describe('codecs.stringArray', () => {
	const c = codecs.stringArray();
	it('parse: empty string -> empty array', () => {
		expect(c.parse('')).toEqual([]);
	});
	it('parse: comma split + empty filter', () => {
		expect(c.parse('a,b,,c')).toEqual(['a', 'b', 'c']);
	});
	it('parse: null -> default', () => {
		expect(c.parse(null)).toEqual([]);
	});
	it('serialize: joins with commas, omits when matches default', () => {
		expect(c.serialize(['a', 'b'])).toBe('a,b');
		expect(c.serialize([])).toBeNull();
	});
});

describe('codecs.bool', () => {
	const c = codecs.bool(false);
	it('parse: 1 / true -> true', () => {
		expect(c.parse('1')).toBe(true);
		expect(c.parse('true')).toBe(true);
	});
	it('parse: anything else -> false', () => {
		expect(c.parse('0')).toBe(false);
		expect(c.parse('nope')).toBe(false);
	});
	it('serialize: omits default, emits 1/0 otherwise', () => {
		expect(c.serialize(false)).toBeNull();
		expect(c.serialize(true)).toBe('1');
	});
});

describe('codecs.int', () => {
	const c = codecs.int(50);
	it('parse: numeric string -> number', () => {
		expect(c.parse('100')).toBe(100);
	});
	it('parse: invalid -> default', () => {
		expect(c.parse('not-a-number')).toBe(50);
	});
	it('serialize: omits default', () => {
		expect(c.serialize(50)).toBeNull();
		expect(c.serialize(25)).toBe('25');
	});
});

describe('codecs.enum', () => {
	const c = codecs.enum(['asc', 'desc'] as const, 'asc');
	it('parse: known value -> value', () => {
		expect(c.parse('desc')).toBe('desc');
	});
	it('parse: unknown -> default', () => {
		expect(c.parse('sideways')).toBe('asc');
	});
	it('serialize: omits default', () => {
		expect(c.serialize('asc')).toBeNull();
		expect(c.serialize('desc')).toBe('desc');
	});
});

describe('readURLParam', () => {
	it('reads a value through the codec', () => {
		const url = new URL('https://example.com/?x=42');
		expect(readURLParam(url, 'x', codecs.int(0))).toBe(42);
	});
	it('returns the codec default for a missing param', () => {
		const url = new URL('https://example.com/');
		expect(readURLParam(url, 'missing', codecs.string('fallback'))).toBe('fallback');
	});
});
