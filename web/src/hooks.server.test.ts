import { describe, expect, it } from 'vitest';
import { configuredControlURL } from './hooks.server';

describe('configuredControlURL', () => {
	it('accepts a configured HTTPS control origin', () => {
		expect(configuredControlURL(' https://control.example.test/ ')).toBe('https://control.example.test');
	});

	it.each(['http://control.example.test', 'not a URL'])('fails closed for %s', (value) => {
		expect(() => configuredControlURL(value)).toThrow('HTTPS URL');
	});

	it('leaves same-origin deployments unconfigured', () => {
		expect(configuredControlURL(undefined)).toBe('');
	});
});
