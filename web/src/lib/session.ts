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
		if (
			typeof value.accessToken !== 'string' || !value.accessToken.trim() ||
			typeof value.refreshToken !== 'string' || !value.refreshToken.trim() ||
			typeof value.expiresAt !== 'number' || !Number.isFinite(value.expiresAt)
		) return null;
		return { accessToken: value.accessToken, refreshToken: value.refreshToken, expiresAt: value.expiresAt };
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
