import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import PackageParamsForm from './PackageParamsForm.svelte';
import type { PackageFormState } from './types';

function params(overrides: Partial<PackageFormState> = {}): PackageFormState {
	return {
		name: '',
		version: '',
		allowDowngrade: false,
		pin: false,
		aptName: '',
		dnfName: '',
		pacmanName: '',
		zypperName: '',
		...overrides
	};
}

function modeSwitch(): HTMLButtonElement {
	const element = document.querySelector<HTMLButtonElement>('[data-slot="switch"]');
	if (!element) throw new Error('the package mode switch is missing');
	return element;
}

describe('package parameter mode', () => {
	it('reseeds on a new model but keeps an explicitly empty per-manager mode', async () => {
		const view = await render(PackageParamsForm, {
			props: { params: params({ name: 'firefox' }) }
		});

		modeSwitch().click();
		expect(modeSwitch().getAttribute('data-state')).toBe('checked');

		await view.rerender({ params: params({ name: 'chromium' }) });
		await vi.waitFor(() => expect(modeSwitch().getAttribute('data-state')).toBe('unchecked'));

		const empty = params();
		await view.rerender({ params: empty });
		modeSwitch().click();
		expect(modeSwitch().getAttribute('data-state')).toBe('checked');
		await view.rerender({ params: empty });
		expect(modeSwitch().getAttribute('data-state')).toBe('checked');
	});
});
