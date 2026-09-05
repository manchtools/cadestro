

export type TokenDraft = {
	name: string;

	maxUses: number;

	expiresInDays: number;
};

export function emptyDraft(): TokenDraft {
	return { name: '', maxUses: 0, expiresInDays: 7 };
}

export function hydrate(raw: unknown): TokenDraft | null {
	if (!raw || typeof raw !== 'object') return null;
	const d = raw as Partial<TokenDraft>;
	const base = emptyDraft();
	return {
		name: typeof d.name === 'string' ? d.name : base.name,
		maxUses: typeof d.maxUses === 'number' ? d.maxUses : base.maxUses,
		expiresInDays:
			typeof d.expiresInDays === 'number'
				? d.expiresInDays
				: base.expiresInDays,
	};
}

export function draftErrors(draft: TokenDraft): Record<string, string> {
 const errors: Record<string, string> = {};
 if (!draft.name.trim() || draft.name.trim().length > 128) errors.name = 'Enter a token name of at most 128 characters';
 if (!Number.isInteger(draft.maxUses) || draft.maxUses < 0 || draft.maxUses > 2147483647) errors.maxUses = 'Uses must be a whole number between 0 and 2147483647';
 if (!Number.isInteger(draft.expiresInDays) || draft.expiresInDays < 1 || !Number.isFinite(new Date(Date.now() + draft.expiresInDays * 86400000).getTime())) errors.expiresInDays = 'Expiry must be a whole number of days, at least 1';
 return errors;
}
