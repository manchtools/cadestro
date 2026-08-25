

import { apiClient } from '$lib/sdk';
import { getLocalizedError } from '$lib/errors';

export const WRITE_CONCURRENCY = 4;

export interface BulkOutcome {
	deviceId: string;
	ok: boolean;

	error?: string;
}

async function mapLimit<T, R>(
	items: readonly T[],
	limit: number,
	fn: (item: T) => Promise<R>
): Promise<R[]> {
	const out = new Array<R>(items.length);
	let next = 0;
	const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
		for (;;) {
			const i = next++;
			if (i >= items.length) return;
			out[i] = await fn(items[i]);
		}
	});
	await Promise.all(workers);
	return out;
}

export async function bulkReboot(deviceIds: readonly string[]): Promise<BulkOutcome[]> {
	return mapLimit(deviceIds, WRITE_CONCURRENCY, async (deviceId) => {
		try {
			await apiClient.rebootDevice(deviceId);
			return { deviceId, ok: true };
		} catch (error) {
			return { deviceId, ok: false, error: getLocalizedError(error) };
		}
	});
}

export async function bulkSetLabel(
	deviceIds: readonly string[],
	key: string,
	value: string
): Promise<BulkOutcome[]> {
	return mapLimit(deviceIds, WRITE_CONCURRENCY, async (deviceId) => {
		try {
			await apiClient.setDeviceLabel(deviceId, key, value);
			return { deviceId, ok: true };
		} catch (error) {
			return { deviceId, ok: false, error: getLocalizedError(error) };
		}
	});
}
