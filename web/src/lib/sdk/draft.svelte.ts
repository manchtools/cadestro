// Svelte 5 reactive draft hook.
// Uses $state and $effect for auto-save — must stay in the web app.

import type { DraftType } from '$pmSdk/offline';
import { offlineStore } from './wrappers.svelte';

export type { DraftType } from '$pmSdk/offline';

export function useDraft<T extends Record<string, unknown>>(
	type: DraftType,
	id: string = 'default',
	initialData: T
) {
	// `getDraft`'s generic is constrained to the DraftType union so the
	// payload shape can be inferred from the type-key augmentation. This
	// hook lets the caller declare its own payload shape `T` instead, so
	// we cast through `unknown` rather than letting TS try to unify the
	// two generics.
	const persisted = offlineStore.getDraft(type, id) as unknown as T | undefined;
	let data = $state<T>(persisted ?? initialData);
	let saveTimeout: ReturnType<typeof setTimeout> | null = null;

	// Auto-save on changes with debouncing
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
