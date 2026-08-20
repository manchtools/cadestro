// The fleet's bulk-write seam — the two selection actions that write, beside
// Assign (which lives on its own surface, see ../assign/assign-data.ts).
//
// Both writes are PER-DEVICE RPCs. There is no bulk reboot and no bulk label on
// the wire, so the fan-out is explicit and BOUNDED rather than an unbounded
// Promise.all storm.
//
//   Reboot — RebootDevice(device_id), a live RPC requiring an active connection.
//   Label  — SetDeviceLabel(id, key, value), the same call the device detail
//     surface makes for a single device.
//
// Per-device outcomes are RETURNED, never thrown: a partial failure is a real
// result the operator has to see, and one failing device must not abort the
// devices behind it in the queue.
import { apiClient } from '$lib/sdk';
import { getLocalizedError } from '$lib/errors';

/** Writes in flight. The same bound the assign lane commits with — enough to
 *  keep a large selection moving, small enough that one bulk action is not a
 *  self-inflicted burst against the control process. */
export const WRITE_CONCURRENCY = 4;

export interface BulkOutcome {
	deviceId: string;
	ok: boolean;
	/** Localized failure text — present only when `ok` is false. */
	error?: string;
}

/** Order-preserving bounded-concurrency map (the assign lane runs the same
 *  pattern; kept local so the fleet's write seam carries no dependency on
 *  another surface's data module). */
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

/** Reboot every selected device through the live RPC. */
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

/** Set the same label key/value on every selected device. */
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
