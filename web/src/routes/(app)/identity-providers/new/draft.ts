export type IdpDraft = {
		name: string;
		slug: string;
		clientId: string;
		issuerUrl: string;
		scopes: string;

	};

export function emptyDraft(): IdpDraft {
		return {
			name: '',
			slug: '',
			clientId: '',
			issuerUrl: '',
			scopes: '',
		};
	}

export function hydrate(raw: unknown): IdpDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<IdpDraft>;
		const base = emptyDraft();
		const str = (v: unknown, fallback: string) => (typeof v === 'string' ? v : fallback);
		return {
			name: str(d.name, base.name),
			slug: str(d.slug, base.slug),
			clientId: str(d.clientId, base.clientId),
			issuerUrl: str(d.issuerUrl, base.issuerUrl),
			scopes: str(d.scopes, base.scopes),
		};
	}

export function draftErrors(draft: IdpDraft): Record<string, string> {
 const errors: Record<string, string> = {};
 if (!draft.name.trim() || draft.name.trim().length > 64) errors.name = 'Enter a provider name of at most 64 characters';
 if (!/^[a-zA-Z0-9]{1,64}$/.test(draft.slug.trim())) errors.slug = 'Use letters and numbers, at most 64 characters';
 if (!draft.clientId.trim()) errors.clientId = 'Enter the OIDC client ID';
 try { if (new URL(draft.issuerUrl.trim()).protocol !== 'https:') errors.issuerUrl = 'Enter an HTTPS issuer URL'; }
 catch { errors.issuerUrl = 'Enter an HTTPS issuer URL'; }
 return errors;
}
