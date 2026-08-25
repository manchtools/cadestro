

import {
	shell,
	enterContext,
	updateContext,
	exitContext,
	leaveContext,
	claimDraft
} from '$lib/shell/shell.svelte';
import type { ContextState } from '$lib/shell/shell.svelte';

export type BuilderContext = Omit<ContextState, 'id'>;

export function bindBuilderContext(id: string, snapshot: () => BuilderContext | null): unknown {
	const claimed = claimDraft(id);

	$effect(() => {
		const next = snapshot();
		const held = shell.pill.context?.id === id;
		if (!next) {
			if (held) exitContext();
			return;
		}

		if (held) updateContext(next);
		else enterContext({ id, ...next });
	});

	$effect(() => () => {

		leaveContext(id);
	});

	return claimed;
}
