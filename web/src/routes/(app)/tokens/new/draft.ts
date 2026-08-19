// The /tokens/new working buffer.
//
// A plain, JSON-shaped object so the same value can be autosaved through
// `useDraft` (an IndexedDB put cannot clone a $state proxy) AND ride on the
// stage card as the stash payload. Both paths land in `hydrate`, the only place
// that decides what a half-trusted stored object may become.
//
// A type alias, not an interface: TypeScript gives an alias of an object type an
// implicit index signature, so the same value satisfies `useDraft`'s
// `Record<string, unknown>` constraint without a cast.
export type TokenDraft = {
	name: string;
	/** 0 = unlimited global device enrollments. */
	maxUses: number;
	/** Required TTL in days. */
	expiresInDays: number;
};

export function emptyDraft(): TokenDraft {
	return { name: '', maxUses: 0, expiresInDays: 7 };
}

/** Rebuild a buffer from a persisted autosave or a claimed stage payload. The
 *  stored object is plain JSON that may be a release old, so every field is
 *  re-checked against the empty draft rather than trusted. */
export function hydrate(raw: unknown): TokenDraft | null {
	if (!raw || typeof raw !== 'object') return null;
	const d = raw as Partial<TokenDraft>;
	const base = emptyDraft();
	return {
		name: typeof d.name === 'string' ? d.name : base.name,
		maxUses: typeof d.maxUses === 'number' && d.maxUses >= 0 ? d.maxUses : base.maxUses,
		expiresInDays:
			typeof d.expiresInDays === 'number' && d.expiresInDays >= 1
				? d.expiresInDays
				: base.expiresInDays,
	};
}
