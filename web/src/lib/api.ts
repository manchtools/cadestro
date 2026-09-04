import { timestampMs } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError, createClient, type Interceptor } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { ControlService } from '$contract/cadestro/v1/control_pb';
import { clearSession, readSession, writeSession, type Session } from './session';

function controlURL(): string {
	const configured = document.querySelector('meta[name="cadestro-control-url"]')?.getAttribute('content')?.trim();
	return configured?.replace(/\/+$/, '') || window.location.origin;
}

const credentialedFetch: typeof fetch = (input, init) => fetch(input, { ...init, credentials: 'include' });
const publicClient = createClient(ControlService, createConnectTransport({ baseUrl: controlURL(), fetch: credentialedFetch }));
let refreshInFlight: { session: Session; promise: Promise<Session | null> } | null = null;

function sessionMatches(left: Session | null, right: Session): boolean {
	return left?.accessToken === right.accessToken && left.refreshToken === right.refreshToken;
}

function clearMatchingSession(session: Session): void {
	if (sessionMatches(readSession(), session)) clearSession();
}

async function refresh(session: Session): Promise<Session | null> {
	if (refreshInFlight && sessionMatches(refreshInFlight.session, session)) return refreshInFlight.promise;
	const promise = publicClient
		.refreshToken({ refreshToken: session.refreshToken })
		.then((response) => {
			const expiresAt = response.expiresAt ? timestampMs(response.expiresAt) : Number.NaN;
			if (!response.accessToken || !response.refreshToken || !Number.isFinite(expiresAt)) {
				clearMatchingSession(session);
				return null;
			}
			if (!sessionMatches(readSession(), session)) return null;
			const renewed = { accessToken: response.accessToken, refreshToken: response.refreshToken, expiresAt };
			writeSession(renewed);
			return renewed;
		})
		.catch((error: unknown) => {
			if (error instanceof ConnectError && error.code === Code.Unauthenticated) {
				clearMatchingSession(session);
				return null;
			}
			throw error;
		})
		.finally(() => {
			if (refreshInFlight?.promise === promise) refreshInFlight = null;
		});
	refreshInFlight = { session, promise };
	return promise;
}

const authenticate: Interceptor = (next) => async (request) => {
	const initiatingSession = readSession();
	let session = initiatingSession;
	let refreshAttempted = false;
	if (session && session.expiresAt <= Date.now() + 30_000) {
		session = await refresh(session);
		refreshAttempted = true;
		if (!session || !sessionMatches(readSession(), session)) throw new ConnectError('authentication session expired', Code.Unauthenticated);
	}
	if (session) request.header.set('Authorization', `Bearer ${session.accessToken}`);
	try {
		return await next(request);
	} catch (error) {
		if (!(error instanceof ConnectError) || error.code !== Code.Unauthenticated || !initiatingSession || refreshAttempted) throw error;
		if (!sessionMatches(readSession(), initiatingSession)) throw error;
		const renewed = await refresh(initiatingSession);
		if (!renewed || !sessionMatches(readSession(), renewed)) throw error;
		request.header.set('Authorization', `Bearer ${renewed.accessToken}`);
		return next(request);
	}
};

export const api = createClient(ControlService, createConnectTransport({ baseUrl: controlURL(), fetch: credentialedFetch, interceptors: [authenticate] }));
export const publicAPI = publicClient;

export async function logout(): Promise<void> {
	const session = readSession();
	clearSession();
	if (session) await publicClient.logout({ refreshToken: session.refreshToken });
}

export function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : 'Request failed';
}
