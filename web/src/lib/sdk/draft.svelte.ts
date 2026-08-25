

import type { DraftType } from '$contractClient/offline';
import { offlineStore } from './wrappers.svelte';

export type { DraftType } from '$contractClient/offline';

export function useDraft<T extends Record<string, unknown>>(
	type: DraftType,
	id: string = 'default',
	initialData: T
) {

	const persisted = offlineStore.getDraft(type, id) as unknown as T | undefined;
	let data = $state<T>(persisted ?? initialData);
	let saveTimeout: ReturnType<typeof setTimeout> | null = null;

	$effect(() => {
		const currentData = { ...data };

		if (saveTimeout) {
			clearTimeout(saveTimeout);
		}

		saveTimeout = setTimeout(() => {
			offlineStore.saveDraft(type, currentData, id);
		}, 500);

		return () => {
			if (saveTimeout) {
				clearTimeout(saveTimeout);
			}
		};
	});

	return {
		get data() {
			return data;
		},
		set data(value: T) {
			data = value;
		},
		update(partial: Partial<T>) {
			data = { ...data, ...partial };
		},
		async clear() {
			data = initialData;
			await offlineStore.clearDraft(type, id);
		},
		get hasDraft() {
			return offlineStore.hasDraft(type, id);
		}
	};
}
