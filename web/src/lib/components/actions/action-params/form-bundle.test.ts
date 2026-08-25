

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
		bundle.set('SHELL', {
			script: 'echo hi',
			interpreter: '/bin/sh',
			runAsRoot: false,
			detectionScript: '',
			isCompliance: false
		});
		expect(bundle.params.SHELL).not.toBe(original);

		expect(bundle.params.PACKAGE).toBeDefined();
	});
});
