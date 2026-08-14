// Tests for the per-action-type adapter registry (F003 + F004).
//
// The registry is the contract between the orchestrator god-components
// and the individual ParamsForm files. If a type drifts (registry says
// "package" but proto says "shell") the create-action and edit-params
// flows silently send malformed data. Pin the contract here.

import { describe, it, expect } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { ActionType } from '$contract/cadestro/v1/actions_pb';
import {
	ManagedEncryptionParamsSchema,
	ManagedWifiParamsSchema
} from '$contract/cadestro/v1/control_pb';
import { encryptionParamsSchema, wifiParamsSchema } from '$lib/forms/schemas/actions';
import {
	encryptionFormToProto,
	encryptionProtoToForm,
	wifiFormToProto,
	wifiProtoToForm
} from './forms/types';
import {
	ACTION_REGISTRY,
	FORM_KEYS,
	formKeyFromActionType,
	formKeyFromString,
	type FormKey
} from './registry';

describe('ACTION_REGISTRY', () => {
	it('has an entry for every FormKey', () => {
		for (const key of FORM_KEYS) {
			expect(ACTION_REGISTRY[key]).toBeDefined();
			expect(ACTION_REGISTRY[key].key).toBe(key);
		}
	});

	it('every entry exposes the required functions and metadata', () => {
		for (const key of FORM_KEYS) {
			const adapter = ACTION_REGISTRY[key];
			expect(typeof adapter.defaultForm).toBe('function');
			expect(typeof adapter.formToProto).toBe('function');
			expect(typeof adapter.protoToForm).toBe('function');
			expect(adapter.schema).toBeDefined();
			expect(typeof adapter.paramsCase).toBe('string');
			expect(typeof adapter.supportsAbsent).toBe('boolean');
			expect(typeof adapter.actionType).toBe('number');
		}
	});

	it('defaultForm returns a non-null value for every FormKey', () => {
		for (const key of FORM_KEYS) {
			const v = ACTION_REGISTRY[key].defaultForm();
			expect(v).toBeDefined();
			expect(v).not.toBeNull();
		}
	});

	it('SHELL and COMPLIANCE_CHECK share the SHELL params bucket', () => {
		const shell = ACTION_REGISTRY.SHELL;
		const compliance = ACTION_REGISTRY.COMPLIANCE_CHECK;
		expect(shell.paramsCase).toBe('shell');
		expect(compliance.paramsCase).toBe('shell');
		expect(shell.actionType).toBe(ActionType.SHELL);
		expect(compliance.actionType).toBe(ActionType.SHELL);
	});

	it('encodes which action types skip the desired-state ABSENT toggle', () => {
		// Single source of truth — orchestrators read this instead of
		// hand-coding the SHELL/UPDATE/AGENT_UPDATE/COMPLIANCE_CHECK list.
		expect(ACTION_REGISTRY.SHELL.supportsAbsent).toBe(false);
		expect(ACTION_REGISTRY.UPDATE.supportsAbsent).toBe(false);
		expect(ACTION_REGISTRY.AGENT_UPDATE.supportsAbsent).toBe(false);
		expect(ACTION_REGISTRY.COMPLIANCE_CHECK.supportsAbsent).toBe(false);
		// Spot-check a few that do.
		expect(ACTION_REGISTRY.PACKAGE.supportsAbsent).toBe(true);
		expect(ACTION_REGISTRY.FILE.supportsAbsent).toBe(true);
		expect(ACTION_REGISTRY.FLATPAK.supportsAbsent).toBe(true);
	});
});

describe('formKeyFromActionType', () => {
	it('maps APP_IMAGE / DEB / RPM all to the shared APP form', () => {
		expect(formKeyFromActionType(ActionType.APP_IMAGE)).toBe('APP');
		expect(formKeyFromActionType(ActionType.DEB)).toBe('APP');
		expect(formKeyFromActionType(ActionType.RPM)).toBe('APP');
	});

	it('maps PACKAGE -> PACKAGE', () => {
		expect(formKeyFromActionType(ActionType.PACKAGE)).toBe('PACKAGE');
	});

	it('returns null for action types without a params form', () => {
		// REBOOT/SYNC/SCRIPT_RUN have no editable params shape.
		expect(formKeyFromActionType(ActionType.REBOOT)).toBeNull();
		expect(formKeyFromActionType(ActionType.SYNC)).toBeNull();
		expect(formKeyFromActionType(ActionType.SCRIPT_RUN)).toBeNull();
	});
});

describe('formKeyFromString', () => {
	it('round-trips every concrete FormKey', () => {
		for (const key of FORM_KEYS) {
			expect(formKeyFromString(key)).toBe(key);
		}
	});

	it('coalesces APP_IMAGE / DEB / RPM into the APP form key', () => {
		expect(formKeyFromString('APP_IMAGE')).toBe('APP');
		expect(formKeyFromString('DEB')).toBe('APP');
		expect(formKeyFromString('RPM')).toBe('APP');
	});

	it('returns null for unknown strings', () => {
		expect(formKeyFromString('NOT_A_TYPE')).toBeNull();
		expect(formKeyFromString('')).toBeNull();
	});
});

// Smoke-test that defaultForm -> formToProto -> protoToForm round-trips
// without throwing. This catches type-shape drift between the form-state
// interfaces and the proto schemas.
describe('default-form proto round-trip', () => {
	const skip = new Set<FormKey>([
		// COMPLIANCE_CHECK and SHELL share the SHELL converters; tested under SHELL.
		'COMPLIANCE_CHECK'
	]);
	for (const key of FORM_KEYS) {
		if (skip.has(key)) continue;
		it(`${key}: default -> proto -> form does not throw`, () => {
			const adapter = ACTION_REGISTRY[key];
			const def = adapter.defaultForm();
			const proto = adapter.formToProto(def);
			expect(proto).toBeDefined();
			const back = adapter.protoToForm(proto);
			expect(back).toBeDefined();
		});
	}
});

describe('write-only action credentials', () => {
	it('hydrates configured encryption metadata without reading the key back', () => {
		const form = encryptionProtoToForm(
			create(ManagedEncryptionParamsSchema, {
				presharedKeyConfigured: true,
				rotationIntervalDays: 30,
				minWords: 5
			})
		);
		expect(form.presharedKey).toBe('');
		expect(form.presharedKeyConfigured).toBe(true);
		expect(encryptionParamsSchema.safeParse(form).success).toBe(true);
		expect(encryptionFormToProto(form).presharedKey).toBeUndefined();
	});

	it('omits configured WiFi credentials while preserving the selected mode', () => {
		const form = wifiProtoToForm(
			create(ManagedWifiParamsSchema, {
				ssid: 'office',
				authType: 1,
				pskConfigured: true,
				autoConnect: true
			})
		);
		expect(form.psk).toBe('');
		expect(form.pskConfigured).toBe(true);
		expect(wifiParamsSchema.safeParse(form).success).toBe(true);
		expect(wifiFormToProto(form).psk).toBeUndefined();
	});
});
