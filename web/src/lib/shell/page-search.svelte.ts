

import { untrack } from 'svelte';

export interface PageSearchRegistration {

	scope: number | null;

	label: () => string;

	readonly query: string;

	setQuery(value: string): void;

	clear(): void;
}

let entry = $state.raw<PageSearchRegistration | null>(null);

export function registerPageSearch(next: PageSearchRegistration): () => void {
	entry = next;
	return () => {

		if (untrack(() => entry) !== next) return;
		entry = null;
	};
}

export function activePageSearch(): PageSearchRegistration | null {
	return entry;
}

export function resetPageSearch() {
	entry = null;
}
