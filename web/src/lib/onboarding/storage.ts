// Onboarding persistence.
//
// First-run state is per SERVER and per USER: the same browser pointed at a
// second control instance, or signed in as a second operator, is a first run
// again. The scope is therefore baked into the storage key rather than into the
// stored value, so switching scope can never read another scope's flags.
//
// Every access is defensive: localStorage throws in private modes and is absent
// during SSR. A storage failure must never keep the app from rendering — it
// degrades to "treat this as a fresh run, remember nothing".

export const STORAGE_PREFIX = 'pm.onboarding.v1';

export interface OnboardingFlags {
	/** The welcome dialog has been presented once; never present it again. */
	welcomeSeen: boolean;
	/** The tour ran to its last step (Done), as opposed to being skipped. */
	tourCompleted: boolean;
	/** The getting-started checklist was dismissed by the operator. */
	checklistDismissed: boolean;
}

const EMPTY: OnboardingFlags = {
	welcomeSeen: false,
	tourCompleted: false,
	checklistDismissed: false
};

/** A stable scope id for one (server, user) pair. Both parts are normalised so
 *  `https://ctl.example/` and `https://ctl.example` are one scope, not two. */
export function onboardingScope(serverUrl: string | null | undefined, userId: string | null | undefined): string {
	let server = (serverUrl ?? '').trim();
	if (server) {
		try {
			server = new URL(server).origin;
		} catch {
			// Not a parseable URL (a relative base, a test fixture): use it verbatim
			// rather than collapsing every unparseable value into one shared scope.
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
		// Field-by-field coercion: a hand-edited or older payload must not be able
		// to inject non-boolean truthiness into the flags.
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
