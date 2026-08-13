// The chip ⇄ query mapping. These assert the STRING the RPC receives, because
// that is the contract the server validates — the chips are only a drawing of
// it. Anything the chips cannot draw must parse to null (raw text survives)
// rather than round-trip into a different query.
import { describe, it, expect } from 'vitest';
import {
	LABEL_CUSTOM,
	compileCond,
	compileQuery,
	emptyCond,
	emptyModel,
	isComplete,
	modelComplete,
	modelIsEmpty,
	parseCondition,
	parseQuery,
	type QueryModel
} from './query-model';

function cond(property: string, operator: string, value = '', labelKey = '') {
	return { property, labelKey, operator, value };
}

describe('compileCond', () => {
	it('quotes a plain value with double quotes', () => {
		expect(compileCond(cond('device.os', 'equals', 'ubuntu'))).toBe('device.os equals "ubuntu"');
	});

	it('falls back to single quotes when the value already contains a double quote', () => {
		expect(compileCond(cond('device.name', 'equals', 'a"b'))).toBe(`device.name equals 'a"b'`);
	});

	it('emits no value for a unary operator', () => {
		expect(compileCond(cond('device.labels.env', 'exists'))).toBe('device.labels.env exists');
	});

	it('passes an in-list through verbatim instead of quoting it', () => {
		expect(compileCond(cond('device.os', 'in', '("ubuntu", "debian")'))).toBe(
			'device.os in ("ubuntu", "debian")'
		);
	});

	it('expands the custom-label sentinel to device.labels.<key>', () => {
		expect(compileCond(cond(LABEL_CUSTOM, 'equals', 'production', 'env'))).toBe(
			'device.labels.env equals "production"'
		);
	});

	it('compiles nothing while the property is unset', () => {
		expect(compileCond(cond('', 'equals', 'x'))).toBe('');
	});
});

describe('compileQuery', () => {
	it('joins conditions with the chosen conjunctions', () => {
		const model: QueryModel = {
			nodes: [
				{ kind: 'cond', cond: cond('device.os', 'equals', 'ubuntu') },
				{ kind: 'cond', cond: cond('device.labels.env', 'notEquals', 'staging') }
			],
			joins: ['AND']
		};
		expect(compileQuery(model)).toBe(
			'device.os equals "ubuntu" AND device.labels.env notEquals "staging"'
		);
	});

	it('wraps a group in parentheses with its own conjunction', () => {
		const model: QueryModel = {
			nodes: [
				{ kind: 'cond', cond: cond('device.labels.env', 'equals', 'production') },
				{
					kind: 'group',
					join: 'OR',
					conds: [cond('device.os', 'equals', 'ubuntu'), cond('device.os', 'equals', 'debian')]
				},
				{ kind: 'cond', cond: cond('device.labels.role', 'notEquals', 'bastion') }
			],
			joins: ['AND', 'AND']
		};
		expect(compileQuery(model)).toBe(
			'device.labels.env equals "production" AND (device.os equals "ubuntu" OR device.os equals "debian") AND device.labels.role notEquals "bastion"'
		);
	});

	it('drops an incomplete condition and does not leave a dangling conjunction', () => {
		const model: QueryModel = {
			nodes: [
				{ kind: 'cond', cond: cond('device.os', 'equals', 'ubuntu') },
				{ kind: 'cond', cond: cond('', 'equals', '') }
			],
			joins: ['OR']
		};
		expect(compileQuery(model)).toBe('device.os equals "ubuntu"');
		expect(modelComplete(model)).toBe(false);
	});

	// An empty query is a LEGAL rule — the server parses '' as the always-true
	// tree and both group pages advertise exactly that ("matches all"). The
	// untouched builder must therefore be complete and serialize to ''.
	it('treats the pristine empty model as complete: "" means match-all', () => {
		expect(modelIsEmpty(emptyModel())).toBe(true);
		expect(modelComplete(emptyModel())).toBe(true);
		// The string the RPC receives for the pristine model is exactly ''.
		expect(compileQuery(emptyModel())).toBe('');
	});

	it('keeps a partially filled condition incomplete — the pristine pass must not loosen the gate', () => {
		// Property chosen, value still empty on a binary operator: half-typed.
		const propertyOnly: QueryModel = {
			nodes: [{ kind: 'cond', cond: cond('device.os', 'equals', '') }],
			joins: []
		};
		expect(modelIsEmpty(propertyOnly)).toBe(false);
		expect(modelComplete(propertyOnly)).toBe(false);

		// Operator moved off the seed while everything else is empty: touched, not pristine.
		const operatorOnly: QueryModel = {
			nodes: [{ kind: 'cond', cond: cond('', 'contains', '') }],
			joins: []
		};
		expect(modelIsEmpty(operatorOnly)).toBe(false);
		expect(modelComplete(operatorOnly)).toBe(false);

		// A second node beside the pristine seed: the empty pass applies to exactly
		// the one-pristine-cond state, never to a model that still holds real work.
		const pristineBeside: QueryModel = {
			nodes: [
				{ kind: 'cond', cond: cond('device.os', 'equals', 'ubuntu') },
				{ kind: 'cond', cond: emptyCond() }
			],
			joins: ['AND']
		};
		expect(modelIsEmpty(pristineBeside)).toBe(false);
		expect(modelComplete(pristineBeside)).toBe(false);

		// A pristine cond wrapped in a group is not the seeded state either.
		const grouped: QueryModel = {
			nodes: [{ kind: 'group', join: 'OR', conds: [emptyCond(), emptyCond()] }],
			joins: []
		};
		expect(modelIsEmpty(grouped)).toBe(false);
		expect(modelComplete(grouped)).toBe(false);
	});

	it('treats a unary condition as complete without a value', () => {
		expect(isComplete(cond('device.labels.env', 'exists'))).toBe(true);
		expect(isComplete(cond('device.labels.env', 'equals'))).toBe(false);
	});
});

describe('parseQuery', () => {
	it('round-trips the concept query through chips unchanged', () => {
		const text =
			'device.labels.env equals "production" AND (device.os equals "ubuntu" OR device.os equals "debian") AND device.labels.role notEquals "bastion"';
		const model = parseQuery(text);
		expect(model).not.toBeNull();
		expect(model!.nodes).toHaveLength(3);
		expect(model!.nodes[1]).toMatchObject({ kind: 'group', join: 'OR' });
		expect(compileQuery(model!)).toBe(text);
	});

	it('round-trips an in-list without re-quoting it', () => {
		const text = 'device.os in ("ubuntu", "debian") AND device.labels.env exists';
		const model = parseQuery(text);
		expect(model).not.toBeNull();
		expect(compileQuery(model!)).toBe(text);
	});

	it('never splits on AND inside a quoted value', () => {
		const text = 'device.name equals "black AND white"';
		const model = parseQuery(text);
		expect(model!.nodes).toHaveLength(1);
		expect(compileQuery(model!)).toBe(text);
	});

	it('folds device.labels.<key> back into a label chip when labels are offered', () => {
		const model = parseQuery('device.labels.env equals "production"', { hasCustomLabels: true });
		expect(model!.nodes[0]).toMatchObject({
			kind: 'cond',
			cond: { property: LABEL_CUSTOM, labelKey: 'env' }
		});
		expect(compileQuery(model!)).toBe('device.labels.env equals "production"');
	});

	it('keeps the raw property when the palette offers no custom labels', () => {
		const model = parseQuery('user.email endsWith "@example.com"');
		expect(model!.nodes[0]).toMatchObject({ cond: { property: 'user.email' } });
	});

	it('normalises a lower-case conjunction to the canonical AND/OR', () => {
		const model = parseQuery('device.os equals "ubuntu" and device.labels.env exists');
		expect(model!.joins).toEqual(['AND']);
	});

	it('collapses a single-condition group rather than emitting stray parentheses', () => {
		const model = parseQuery('(device.os equals "ubuntu")');
		expect(model!.nodes[0].kind).toBe('cond');
		expect(compileQuery(model!)).toBe('device.os equals "ubuntu"');
	});

	it('returns an empty model for an empty query', () => {
		expect(parseQuery('   ')).toEqual(emptyModel());
	});

	describe('refuses what the chips cannot draw', () => {
		it.each([
			['NOT', 'NOT device.os equals "ubuntu"'],
			['a top-level not between conditions', 'device.os equals "ubuntu" not device.kernel exists'],
			['nested groups', '((a equals "1" OR b equals "2") AND c equals "3") OR d equals "4"'],
			['a mixed-conjunction group', '(a equals "1" OR b equals "2" AND c equals "3")'],
			['unbalanced parentheses', '(device.os equals "ubuntu"'],
			['an unparseable token', 'device.os ~~ ubuntu']
		])('%s', (_label, text) => {
			expect(parseQuery(text)).toBeNull();
		});
	});
});

describe('parseCondition', () => {
	it('accepts a single-quoted value', () => {
		expect(parseCondition(`device.name equals 'web 01'`)).toMatchObject({ value: 'web 01' });
	});

	it('canonicalises operator case for unary and list operators', () => {
		expect(parseCondition('device.labels.env EXISTS')).toMatchObject({ operator: 'exists' });
		expect(parseCondition('device.os IN ("a")')).toMatchObject({ operator: 'in' });
	});

	it('rejects an unquoted value', () => {
		expect(parseCondition('device.os equals ubuntu')).toBeNull();
	});
});
