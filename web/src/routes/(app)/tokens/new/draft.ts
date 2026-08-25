

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
		maxUses: typeof d.maxUses === 'number' && d.maxUses >= 0 ? d.maxUses : base.maxUses,
		expiresInDays:
			typeof d.expiresInDays === 'number' && d.expiresInDays >= 1
				? d.expiresInDays
				: base.expiresInDays,
	};
}
