// The library overview's derivations. What is load-bearing here:
//
//   1. an action lands in the bucket the page's own type FILTER can name, and a
//      compliance check is its own bucket rather than a shell script;
//   2. a type the filter menu has no slug for is still counted, but is marked
//      unfilterable instead of pretending a click could narrow to it;
//   3. install + remove === total is an invariant of the summary strip.
import { describe, it, expect } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { ManagedActionSchema } from '$sdk/powermanage/v1/control_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import { DesiredState } from '$sdk/powermanage/v1/common_pb';
import {
	bucketOf,
	buildBubbles,
	isCompliance,
	summarize,
	COMPLIANCE_BUCKET,
	UNFILTERABLE_PREFIX
} from './library-model';

/** The same map the page builds from getActionTypeOptions(): the exact inverse
 *  of the slug → ActionType map its `filterToTags` reads. WIFI is deliberately
 *  absent, because the page's type menu really does omit it. */
const SLUG_BY_TYPE = new Map<number, string>([
	[ActionType.SHELL, 'shell'],
	[ActionType.PACKAGE, 'package'],
	[ActionType.SERVICE, 'service']
]);

function action(o: {
	id: string;
	name?: string;
	type?: ActionType;
	absent?: boolean;
	compliance?: boolean;
}) {
	const base = {
		id: o.id,
		name: o.name ?? o.id,
		type: o.type ?? ActionType.SHELL,
		desiredState: o.absent ? DesiredState.ABSENT : DesiredState.PRESENT
	};
	if (o.compliance === undefined) return create(ManagedActionSchema, base);
	return create(ManagedActionSchema, {
		...base,
		params: { case: 'shell' as const, value: { script: 'true', isCompliance: o.compliance } }
	});
}

describe('bucketOf — the grouping key is the page filter’s own id space', () => {
	it('names a plain typed action by its filter slug', () => {
		expect(bucketOf(action({ id: 'a', type: ActionType.PACKAGE }), SLUG_BY_TYPE)).toBe('package');
	});

	it('splits a compliance-flagged SHELL action out of the shell bucket', () => {
		const check = action({ id: 'c', type: ActionType.SHELL, compliance: true });
		expect(isCompliance(check)).toBe(true);
		expect(bucketOf(check, SLUG_BY_TYPE)).toBe(COMPLIANCE_BUCKET);

		const script = action({ id: 's', type: ActionType.SHELL, compliance: false });
		expect(isCompliance(script)).toBe(false);
		expect(bucketOf(script, SLUG_BY_TYPE)).toBe('shell');
	});

	it('marks a type the filter menu cannot name instead of guessing a slug', () => {
		expect(bucketOf(action({ id: 'w', type: ActionType.WIFI }), SLUG_BY_TYPE)).toBe(
			UNFILTERABLE_PREFIX + ActionType.WIFI
		);
	});
});

describe('buildBubbles — one bubble per bucket really present', () => {
	const library = [
		action({ id: 'p1', name: 'Install Firefox', type: ActionType.PACKAGE }),
		action({ id: 'p2', name: 'Drop telnet', type: ActionType.PACKAGE, absent: true }),
		action({ id: 'p3', name: 'Install curl', type: ActionType.PACKAGE }),
		action({ id: 's1', name: 'Rotate logs', type: ActionType.SHELL, compliance: false }),
		action({ id: 'c1', name: 'Check LUKS', type: ActionType.SHELL, compliance: true }),
		action({ id: 'c2', name: 'Check sshd', type: ActionType.SHELL, compliance: true }),
		action({ id: 'w1', name: 'Corp wifi', type: ActionType.WIFI })
	];

	it('counts every bucket off the snapshot, compliance apart from shell', () => {
		const bubbles = buildBubbles(library, SLUG_BY_TYPE);
		expect(bubbles.length).toBeGreaterThan(0); // matches-zero guard

		expect(
			bubbles.map((b) => [b.id, b.actions.length, b.remove, b.filterable])
		).toEqual([
			// filterable first, biggest first, then id
			['package', 3, 1, true],
			['compliance', 2, 0, true],
			['shell', 1, 0, true],
			// the bucket the type filter cannot name always trails
			[UNFILTERABLE_PREFIX + ActionType.WIFI, 1, 0, false]
		]);
	});

	it('accounts for every swept action exactly once', () => {
		const bubbles = buildBubbles(library, SLUG_BY_TYPE);
		expect(bubbles.length).toBeGreaterThan(0);

		const seen = bubbles.flatMap((b) => b.actions.map((a) => a.id));
		expect(seen.length).toBe(library.length);
		expect(new Set(seen).size).toBe(library.length);
	});

	it('orders a bubble removals-first, then by name', () => {
		const bubbles = buildBubbles(library, SLUG_BY_TYPE);
		const pkg = bubbles.find((b) => b.id === 'package');
		expect(pkg).toBeDefined();
		expect(pkg!.actions.map((a) => a.name)).toEqual([
			'Drop telnet', // ABSENT leads
			'Install curl',
			'Install Firefox'
		]);
		expect(pkg!.actions[0].absent).toBe(true);
	});

	it('returns no bubbles at all for an empty library', () => {
		expect(buildBubbles([], SLUG_BY_TYPE)).toEqual([]);
	});
});

describe('summarize — the strip cannot disagree with the tiles', () => {
	it('partitions the library into install and remove', () => {
		const s = summarize([
			action({ id: 'a' }),
			action({ id: 'b', absent: true }),
			action({ id: 'c' }),
			action({ id: 'd', absent: true }),
			action({ id: 'e' })
		]);

		expect(s).toEqual({ total: 5, install: 3, remove: 2 });
		expect(s.install + s.remove).toBe(s.total);
	});

	it('is all zeroes for an empty library', () => {
		expect(summarize([])).toEqual({ total: 0, install: 0, remove: 0 });
	});
});
