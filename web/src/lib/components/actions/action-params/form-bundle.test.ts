// Tests for createFormBundle (F003 + F004).
//
// The bundle is the runtime contract that lets both god-orchestrators
// (action-create-form, edit-params-dialog) treat 19 action types as a
// uniform map. If a registry entry is missing, validate() / set() /
// clearAllErrors() will throw on iteration — the bundle test pins this.

import { describe, it, expect } from 'vitest';
import { createFormBundle } from './form-bundle.svelte';
import { FORM_KEYS } from '../registry';

describe('createFormBundle', () => {
	it('seeds default form-state for every FormKey', () => {
		const bundle = createFormBundle();
		for (const key of FORM_KEYS) {
			expect(bundle.params[key]).toBeDefined();
		}
	});

	it('exposes a validation handle for every FormKey', () => {
		const bundle = createFormBundle();
		for (const key of FORM_KEYS) {
			const v = bundle.validations[key];
			expect(v).toBeDefined();
			expect(typeof v.validate).toBe('function');
			expect(typeof v.clearErrors).toBe('function');
		}
	});

	it('clearAllErrors does not throw and resets every handle', () => {
		const bundle = createFormBundle();
		// Run a validation that will likely fail (default forms aren't
		// always valid) and then clear.
		for (const key of FORM_KEYS) {
			bundle.validate(key);
		}
		bundle.clearAllErrors();
		for (const key of FORM_KEYS) {
			expect(Object.keys(bundle.validations[key].errors).length).toBe(0);
		}
	});

	it('set() replaces the form state for a key without affecting siblings', () => {
		const bundle = createFormBundle();
		const original = bundle.params.SHELL;
		// Use any-cast because the bundle uses `any` for cross-type
		// uniformity — see form-bundle.svelte.ts for the rationale.
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		bundle.set('SHELL', { command: 'echo hi', isCompliance: false } as any);
		expect(bundle.params.SHELL).not.toBe(original);
		// Other keys still have their default state
		expect(bundle.params.PACKAGE).toBeDefined();
	});
});
