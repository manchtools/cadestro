// Self-discovering parity guard for execution-status labels.
//
// The status-badge label maps — ./execution-status.ts (the list page and the
// execution watch window) plus the ones still local to the execution detail
// page and the device overview tab — switch over ExecutionStatus and fall back to
// "Unknown" for values they don't know. That default silently swallows any
// enum value added in the SDK without a label — exactly how NOT_APPLICABLE
// would have rendered as "Unknown". This test iterates the enum from the
// SDK and requires an i18n key for every value, so a new status forces a
// conscious label decision here before it ships.

import { describe, it, expect } from 'vitest';
import { ExecutionStatus } from '$sdk/powermanage/v1/common_pb';
import en from '../../messages/en.json';
import de from '../../messages/de.json';

// Pre-existing keys that use the singular "execution_status_" prefix.
const KEY_OVERRIDES: Record<string, string> = {
	SCHEDULED: 'execution_status_scheduled',
	CANCELLED: 'execution_status_cancelled'
};

function labelKey(name: string): string {
	return KEY_OVERRIDES[name] ?? `executions_status_${name.toLowerCase()}`;
}

describe('execution status label parity', () => {
	const names = Object.keys(ExecutionStatus).filter((k) => isNaN(Number(k)));

	it('every ExecutionStatus value has an i18n label in en and de', () => {
		const missing: string[] = [];
		for (const name of names) {
			if (name === 'UNSPECIFIED') continue;
			const key = labelKey(name);
			if (!(key in en)) missing.push(`${name} → ${key} (en)`);
			if (!(key in de)) missing.push(`${name} → ${key} (de)`);
		}
		expect(missing, `add i18n labels for new ExecutionStatus values: ${missing.join(', ')}`).toEqual(
			[]
		);
	});

	it('matches-zero guard: enum iteration actually found values', () => {
		// If the SDK enum shape ever changes so the iteration finds nothing,
		// the parity check above would pass vacuously — fail loudly instead.
		expect(names.length).toBeGreaterThan(3);
	});
});
