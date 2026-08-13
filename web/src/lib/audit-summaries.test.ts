// Every covered audit event type renders a human-readable summary, and no
// summary can leak a payload field the
// builder does not explicitly name — including server-redacted values.

import { describe, it, expect } from 'vitest';
import {
	auditEventSummary,
	auditEventOutcome,
	SUMMARIZED_EVENT_TYPES,
	type AuditSummaryContext
} from './audit-summaries';

const ctx: AuditSummaryContext = {
	deviceName: (id) => (id === 'dev-1' ? 'host-alpha' : id || '?')
};

// One realistic payload per covered event type (the wire shapes from
// control API exposes. Keyed by event type so the
// drift guard below can require an entry for every covered type.
const PAYLOADS: Record<(typeof SUMMARIZED_EVENT_TYPES)[number], Record<string, unknown>> = {
	OSQueryDispatched: { device_id: 'dev-1', query_id: 'q1', table_name: 'processes' },
	DeviceLogsQueried: { device_id: 'dev-1', query_id: 'q2', unit: 'sshd.service', priority: 'err' },
	DeviceInventoryRefreshRequested: { device_id: 'dev-1' },
	LuksTokenCreated: { device_id: 'dev-1', action_id: 'act-1' },
	LpsPasswordsViewed: {
		device_id: 'dev-1',
		entries: [
			{ rotation_id: 'r1', username: 'root', current: true },
			{ rotation_id: 'r2', username: 'root', current: false }
		]
	},
	LuksKeysViewed: {
		device_id: 'dev-1',
		entries: [{ rotation_id: 'r3', device_path: '/dev/sda2', current: true }]
	},
	LpsPasswordsViewDenied: { device_id: 'dev-1', reason: 'device_not_found' },
	LuksKeysViewDenied: { device_id: 'dev-1', reason: 'decrypt_failed' },
	UserLoggedOut: { jti: 'jti-1' },
	UserSessionRefreshed: { old_jti: 'jti-0' },
	TerminalSessionStarted: { session_id: 's1', tty_user: 'pm-tty-alice', cols: 80, rows: 24 },
	TerminalSessionStopped: { session_id: 's1', reason: 'user_stopped' },
	TerminalSessionTerminated: { session_id: 's1', reason: 'operator_disconnect' }
};

describe('auditEventSummary', () => {
	it('renders a summary for EVERY covered event type (drift guard)', () => {
		for (const eventType of SUMMARIZED_EVENT_TYPES) {
			const payload = PAYLOADS[eventType];
			expect(payload, `PAYLOADS is missing a fixture for ${eventType}`).toBeDefined();
			const summary = auditEventSummary({ eventType, data: JSON.stringify(payload) }, ctx);
			expect(summary, `${eventType} must render a summary`).toBeTruthy();
		}
	});

	it('surfaces the identifying details, resolved through the context', () => {
		const cases: Array<[string, string[]]> = [
			['OSQueryDispatched', ['processes', 'host-alpha']],
			['DeviceLogsQueried', ['sshd.service', 'host-alpha']],
			['LpsPasswordsViewed', ['2', 'host-alpha']],
			['LuksKeysViewed', ['1', 'host-alpha']],
			['LpsPasswordsViewDenied', ['device_not_found', 'host-alpha']],
			['TerminalSessionStarted', ['pm-tty-alice']],
			['TerminalSessionTerminated', ['operator_disconnect']]
		];
		for (const [eventType, fragments] of cases) {
			const summary = auditEventSummary(
				{ eventType, data: JSON.stringify(PAYLOADS[eventType as keyof typeof PAYLOADS]) },
				ctx
			);
			for (const fragment of fragments) {
				expect(summary, `${eventType} summary`).toContain(fragment);
			}
		}
	});

	it('never leaks fields the builder does not name — not even redacted ones', () => {
		const SENTINEL = 'SENTINEL_LEAK_4af1';
		for (const eventType of SUMMARIZED_EVENT_TYPES) {
			const poisoned = {
				...PAYLOADS[eventType],
				password: SENTINEL,
				passphrase: SENTINEL,
				script: SENTINEL,
				secret_encrypted: SENTINEL,
				redacted_marker: '[REDACTED]'
			};
			const summary = auditEventSummary({ eventType, data: JSON.stringify(poisoned) }, ctx);
			expect(summary, `${eventType} summary must not leak unrelated payload fields`).not.toContain(SENTINEL);
			expect(summary).not.toContain('[REDACTED]');
		}
	});

	it('returns null for unknown event types and malformed payloads', () => {
		expect(auditEventSummary({ eventType: 'UserCreatedWithRoles', data: '{}' }, ctx)).toBeNull();
		expect(auditEventSummary({ eventType: 'OSQueryDispatched', data: 'not json' }, ctx)).toBeNull();
		expect(auditEventSummary({ eventType: 'OSQueryDispatched', data: '[1,2]' }, ctx)).toBeNull();
	});

	it('tolerates absent identifier fields without throwing', () => {
		for (const eventType of SUMMARIZED_EVENT_TYPES) {
			expect(() => auditEventSummary({ eventType, data: '{}' }, ctx)).not.toThrow();
		}
	});
});

describe('auditEventOutcome', () => {
	it('classifies *Denied as denied, everything else as success', () => {
		expect(auditEventOutcome('LpsPasswordsViewDenied')).toBe('denied');
		expect(auditEventOutcome('LuksKeysViewDenied')).toBe('denied');
		expect(auditEventOutcome('LpsPasswordsViewed')).toBe('success');
		expect(auditEventOutcome('UserCreatedWithRoles')).toBe('success');
	});
});
