export type Session = {
	accessToken: string;
	refreshToken: string;
	expiresAt: number;
};

const key = 'cadestro.session';

export function readSession(storage: Storage = localStorage): Session | null {
	const raw = storage.getItem(key);
	if (!raw) return null;
	try {
		const value = JSON.parse(raw) as Partial<Session>;
		if (!value.accessToken || !value.refreshToken || typeof value.expiresAt !== 'number') return null;
		return value as Session;
	} catch {
		return null;
	}
}

export function writeSession(session: Session, storage: Storage = localStorage): void {
	storage.setItem(key, JSON.stringify(session));
}

export function clearSession(storage: Storage = localStorage): void {
	storage.removeItem(key);
}

export function safeRedirect(value: string | null): string {
	if (!value || !value.startsWith('/') || value.startsWith('//') || value.includes('\\')) return '/';
	return value;
}
