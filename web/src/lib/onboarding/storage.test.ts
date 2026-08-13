// Onboarding persistence: the scope keeps one server/user's first run from
// consuming another's, and a corrupted payload degrades to "fresh run".
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { onboardingScope, storageKey, readFlags, writeFlags, STORAGE_PREFIX } from './storage';

function memoryStorage(): Storage {
	const map = new Map<string, string>();
	return {
		get length() {
			return map.size;
		},
		clear: () => map.clear(),
		getItem: (k: string) => map.get(k) ?? null,
		key: (i: number) => [...map.keys()][i] ?? null,
		removeItem: (k: string) => void map.delete(k),
		setItem: (k: string, v: string) => void map.set(k, v)
	} as Storage;
}

const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');

beforeEach(() => {
	Object.defineProperty(globalThis, 'localStorage', {
		value: memoryStorage(),
		configurable: true,
		writable: true
	});
});

afterEach(() => {
	if (original) Object.defineProperty(globalThis, 'localStorage', original);
	else Reflect.deleteProperty(globalThis, 'localStorage');
	vi.restoreAllMocks();
});

describe('onboardingScope', () => {
	it('collapses equivalent server URLs to one scope', () => {
		expect(onboardingScope('https://ctl.example/', 'u1')).toBe(onboardingScope('https://ctl.example', 'u1'));
	});

	it('separates servers and users', () => {
		const a = onboardingScope('https://a.example', 'u1');
		expect(onboardingScope('https://b.example', 'u1')).not.toBe(a);
		expect(onboardingScope('https://a.example', 'u2')).not.toBe(a);
	});

	it('keeps an unparseable base distinct instead of folding it into one bucket', () => {
		expect(onboardingScope('/api/', 'u1')).not.toBe(onboardingScope('/other', 'u1'));
		expect(onboardingScope('', null)).toBe('local|anon');
	});

	it('namespaces the storage key', () => {
		expect(storageKey('https://ctl.example|u1')).toBe(`${STORAGE_PREFIX}:https://ctl.example|u1`);
	});
});

describe('flags', () => {
	it('starts blank and round-trips a patch', () => {
		expect(readFlags('s')).toEqual({ welcomeSeen: false, tourCompleted: false, checklistDismissed: false });
		writeFlags('s', { welcomeSeen: true });
		expect(readFlags('s').welcomeSeen).toBe(true);
		writeFlags('s', { tourCompleted: true });
		expect(readFlags('s')).toEqual({ welcomeSeen: true, tourCompleted: true, checklistDismissed: false });
	});

	it('does not leak one scope into another', () => {
		writeFlags('a', { welcomeSeen: true });
		expect(readFlags('b').welcomeSeen).toBe(false);
	});

	it('coerces a hand-edited payload to booleans instead of trusting truthiness', () => {
		localStorage.setItem(storageKey('s'), JSON.stringify({ welcomeSeen: 'yes', tourCompleted: 1 }));
		expect(readFlags('s')).toEqual({ welcomeSeen: false, tourCompleted: false, checklistDismissed: false });
	});

	it('treats unparseable stored state as a fresh run', () => {
		vi.spyOn(console, 'debug').mockImplementation(() => {});
		localStorage.setItem(storageKey('s'), '{not json');
		expect(readFlags('s').welcomeSeen).toBe(false);
	});

	it('survives a storage that throws, without remembering anything', () => {
		vi.spyOn(console, 'debug').mockImplementation(() => {});
		Object.defineProperty(globalThis, 'localStorage', {
			value: {
				getItem: () => {
					throw new Error('denied');
				},
				setItem: () => {
					throw new Error('denied');
				}
			},
			configurable: true,
			writable: true
		});
		expect(() => writeFlags('s', { welcomeSeen: true })).not.toThrow();
		expect(readFlags('s').welcomeSeen).toBe(false);
	});
});
