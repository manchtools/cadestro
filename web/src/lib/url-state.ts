import { onMount } from 'svelte';
import { pushState, replaceState } from '$app/navigation';

export type Codec<T> = {

	parse(param: string | null): T;

	serialize(value: T): string | null;
};

export const codecs = {
	string(defaultValue = ''): Codec<string> {
		return {
			parse: (p) => p ?? defaultValue,
			serialize: (v) => (v === defaultValue ? null : v)
		};
	},

	stringArray(defaultValue: string[] = []): Codec<string[]> {
		const def = defaultValue.join(',');
		return {
			parse: (p) => {
				if (p === null) return [...defaultValue];
				if (p === '') return [];
				return p.split(',').filter((s) => s.length > 0);
			},
			serialize: (v) => {
				const s = v.join(',');
				return s === def ? null : s;
			}
		};
	},

	bool(defaultValue = false): Codec<boolean> {
		return {
			parse: (p) => {
				if (p === null) return defaultValue;
				return p === '1' || p === 'true';
			},
			serialize: (v) => (v === defaultValue ? null : v ? '1' : '0')
		};
	},

	int(defaultValue = 0): Codec<number> {
		return {
			parse: (p) => {
				if (p === null) return defaultValue;
				const n = Number(p);
				return Number.isFinite(n) ? n : defaultValue;
			},
			serialize: (v) => (v === defaultValue ? null : String(v))
		};
	},

	enum<T extends string>(allowed: readonly T[], defaultValue: T): Codec<T> {
		const set = new Set<string>(allowed);
		return {
			parse: (p) => (p !== null && set.has(p) ? (p as T) : defaultValue),
			serialize: (v) => (v === defaultValue ? null : v)
		};
	}
};

export function readURLParam<T>(url: URL, key: string, codec: Codec<T>): T {
	return codec.parse(url.searchParams.get(key));
}

export type AnyCodecEntry = [string, unknown, Codec<unknown>];

export function syncToURL(updates: AnyCodecEntry[], mode: 'push' | 'replace' = 'push') {
	if (typeof window === 'undefined') return;
	const url = new URL(window.location.href);
	for (const [key, value, codec] of updates) {
		const serialized = codec.serialize(value);
		if (serialized === null) {
			url.searchParams.delete(key);
		} else {
			url.searchParams.set(key, serialized);
		}
	}

	const queryString = url.searchParams.toString().replaceAll('%2C', ',');
	const target = url.pathname + (queryString ? '?' + queryString : '') + url.hash;
	if (mode === 'push') {
		pushState(target, {});
	} else {
		replaceState(target, {});
	}
}

export function syncOneToURL<T>(
	key: string,
	value: T,
	codec: Codec<T>,
	mode: 'push' | 'replace' = 'push'
) {
	syncToURL([[key, value, codec]], mode);
}

export function onPopstate(handler: (url: URL) => void) {
	onMount(() => {
		const fn = () => handler(new URL(window.location.href));
		window.addEventListener('popstate', fn);
		return () => window.removeEventListener('popstate', fn);
	});
}
