// The action detail editor commits through the pill, which means the pill's
// state IS the operator's feedback that a save landed.
//
// The baseline it compares against was captured once, at construction, and the
// detail page does not remount the editor when a save returns — so after a
// successful save the buffer still differed from a baseline describing the body
// as it was BEFORE the save. The pill kept saying "something changed" about work
// that was already stored, and Save stayed armed to re-send it forever.
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ManagedActionSchema, type ManagedAction } from '$contract/cadestro/v1/control_pb';
import { ActionType, PackageParamsSchema } from '$contract/cadestro/v1/actions_pb';
import { DesiredState } from '$contract/cadestro/v1/common_pb';
import { shell, resetShell, commitContext } from '$lib/shell/shell.svelte';

const ACTION_ID = '01JQZZACTION0000000000000A';

const api = vi.hoisted(() => ({
	updateActionParams: vi.fn(),
	renameAction: vi.fn(),
	updateActionDescription: vi.fn()
}));

vi.mock('$lib/sdk', async (importOriginal) => ({
	...((await importOriginal()) as object),
	apiClient: api
}));

import ActionParamsEditor from './action-params-editor.svelte';

function packageAction(over: Partial<Pick<ManagedAction, 'timeoutSeconds'>> = {}) {
	return create(ManagedActionSchema, {
		id: ACTION_ID,
		name: 'Install curl',
		description: 'baseline tooling',
		type: ActionType.PACKAGE,
		timeoutSeconds: 300,
		desiredState: DesiredState.PRESENT,
		params: { case: 'package', value: create(PackageParamsSchema, { name: 'curl' }) },
		...over
	});
}

function timeoutInput(): HTMLInputElement {
	const el = document.querySelector<HTMLInputElement>('input#action-timeout');
	if (!el) throw new Error('the editor never rendered its timeout field');
	return el;
}

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

beforeEach(() => {
	document.body.innerHTML = '';
	resetShell();
	vi.clearAllMocks();
	api.renameAction.mockResolvedValue(undefined);
	api.updateActionDescription.mockResolvedValue(undefined);
});

describe('a saved action is not still "changed"', () => {
	it('rebases on the saved body, so the pill goes quiet after a commit', async () => {
		// The server returns the stored body — the same shape a reload would give.
		api.updateActionParams.mockImplementation(async () =>
			packageAction({ timeoutSeconds: 600 })
		);

		render(ActionParamsEditor, { props: { action: packageAction(), onsaved: () => {} } });
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`action:${ACTION_ID}`));
		expect(shell.pill.context?.dirty).toBe(false);

		type(timeoutInput(), '600');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));

		commitContext();
		await vi.waitFor(() => expect(api.updateActionParams).toHaveBeenCalledTimes(1));

		// The whole point: stored work does not keep announcing itself as unsaved,
		// and Save does not stay armed to re-send what the server already has.
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(false));
		expect(commitContext(), 'a clean context refuses a second commit').toBe(false);
		expect(api.updateActionParams).toHaveBeenCalledTimes(1);
	});

	it('stays dirty when the operator kept typing during the round trip', async () => {
		let release!: (a: unknown) => void;
		api.updateActionParams.mockImplementation(
			() => new Promise((resolve) => (release = resolve))
		);

		render(ActionParamsEditor, { props: { action: packageAction(), onsaved: () => {} } });
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`action:${ACTION_ID}`));

		type(timeoutInput(), '600');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));
		commitContext();
		await vi.waitFor(() => expect(api.updateActionParams).toHaveBeenCalledTimes(1));

		// A keystroke that lands while the save is in flight is NOT stored, so
		// rebasing must not silently swallow it.
		type(timeoutInput(), '900');
		release(packageAction({ timeoutSeconds: 600 }));

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));
		expect(timeoutInput().value, 'the in-flight edit survives the rebase').toBe('900');
	});
});
