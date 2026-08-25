import { describe, expect, it } from 'vitest';
import { ActionType } from '../gen/ts/cadestro/v1/actions_pb';
import { getActionTypeEnum, actionTypeToString, ACTION_TYPE_OPTIONS } from './action-types';

const enumMembers = Object.entries(ActionType).filter(
	(e): e is [string, ActionType] => typeof e[1] === 'number' && e[1] !== ActionType.UNSPECIFIED
);

describe('action type mappings cover the generated enum', () => {
	it('discovers a non-empty enum', () => {
		expect(enumMembers.length).toBeGreaterThan(0);
	});

	it.each(enumMembers)('%s round-trips through toString/fromString', (name, value) => {
		const s = actionTypeToString(value);
		expect(s).toBe(name);
		expect(getActionTypeEnum(s)).toBe(value);
	});

	it('ACTION_TYPE_OPTIONS lists every enum member exactly once', () => {
		const optionTypes = ACTION_TYPE_OPTIONS.map((o) => o.type);
		expect(new Set(optionTypes).size).toBe(optionTypes.length);
		for (const [, value] of enumMembers) {
			expect(optionTypes).toContain(value);
		}
	});
});
