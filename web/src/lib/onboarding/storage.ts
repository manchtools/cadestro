

export const STORAGE_PREFIX = 'pm.onboarding.v1';

export interface OnboardingFlags {

	welcomeSeen: boolean;

	tourCompleted: boolean;

	checklistDismissed: boolean;
}

const EMPTY: OnboardingFlags = {
	welcomeSeen: false,
	tourCompleted: false,
	checklistDismissed: false
};

export function onboardingScope(serverUrl: string | null | undefined, userId: string | null | undefined): string {
	let server = (serverUrl ?? '').trim();
	if (server) {
		try {
			server = new URL(server).origin;
		} catch {

			server = server.replace(/\/+$/, '');
		}
	}
	return `${server || 'local'}|${(userId ?? '').trim() || 'anon'}`;
}

export function storageKey(scope: string): string {
	return `${STORAGE_PREFIX}:${scope}`;
}

function store(): Storage | null {
	try {
		return typeof localStorage === 'undefined' ? null : localStorage;
	} catch (err) {
		console.debug('onboarding: localStorage unavailable', err);
		return null;
	}
}

export function readFlags(scope: string): OnboardingFlags {
	const s = store();
	if (!s) return { ...EMPTY };
	let raw: string | null = null;
	try {
		raw = s.getItem(storageKey(scope));
	} catch (err) {
		console.debug('onboarding: could not read onboarding flags', err);
		return { ...EMPTY };
	}
	if (!raw) return { ...EMPTY };
	try {
		const parsed: unknown = JSON.parse(raw);
		if (parsed === null || typeof parsed !== 'object') return { ...EMPTY };
		const rec = parsed as Record<string, unknown>;

		return {
			welcomeSeen: rec.welcomeSeen === true,
			tourCompleted: rec.tourCompleted === true,
			checklistDismissed: rec.checklistDismissed === true
		};
	} catch (err) {
		console.debug('onboarding: discarding unparseable onboarding flags', err);
		return { ...EMPTY };
	}
}

export function writeFlags(scope: string, patch: Partial<OnboardingFlags>): OnboardingFlags {
	const next = { ...readFlags(scope), ...patch };
	const s = store();
	if (!s) return next;
	try {
		s.setItem(storageKey(scope), JSON.stringify(next));
	} catch (err) {
		console.debug('onboarding: could not persist onboarding flags', err);
	}
	return next;
}
