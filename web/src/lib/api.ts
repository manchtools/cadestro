import { timestampMs } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError, createClient, type Interceptor } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { ControlService } from '$contract/cadestro/v1/control_pb';
import { clearSession, readSession, writeSession } from './session';

function controlURL(): string {
	const configured = document.querySelector('meta[name="cadestro-control-url"]')?.getAttribute('content')?.trim();
	return configured?.replace(/\/+$/, '') || window.location.origin;
}

const publicClient = createClient(ControlService, createConnectTransport({ baseUrl: controlURL() }));
let refreshInFlight: Promise<boolean> | null = null;

async function refresh(): Promise<boolean> {
	const session = readSession();
	if (!session) return false;
	if (!refreshInFlight) {
		refreshInFlight = publicClient
			.refreshToken({ refreshToken: session.refreshToken })
			.then((response) => {
				if (!response.expiresAt) return false;
				writeSession({ accessToken: response.accessToken, refreshToken: response.refreshToken, expiresAt: timestampMs(response.expiresAt) });
				return true;
			})
			.catch(() => false)
			.finally(() => {
				refreshInFlight = null;
			});
	}
	return refreshInFlight;
}

const authenticate: Interceptor = (next) => async (request) => {
	let session = readSession();
	if (session && session.expiresAt <= Date.now() + 30_000) {
		if (await refresh()) session = readSession();
	}
	if (session) request.header.set('Authorization', `Bearer ${session.accessToken}`);
	try {
		return await next(request);
	} catch (error) {
		if (error instanceof ConnectError && error.code === Code.Unauthenticated && (await refresh())) {
			const renewed = readSession();
			if (renewed) request.header.set('Authorization', `Bearer ${renewed.accessToken}`);
			return next(request);
		}
		throw error;
	}
};

export const api = createClient(ControlService, createConnectTransport({ baseUrl: controlURL(), interceptors: [authenticate] }));
export const publicAPI = publicClient;

export async function logout(): Promise<void> {
	const session = readSession();
	try {
		if (session) await publicClient.logout({ refreshToken: session.refreshToken });
	} finally {
		clearSession();
	}
}

export function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : 'Request failed';
}
