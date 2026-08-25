// The /actions/new working buffer.
//
// One plain, JSON-shaped object so the SAME value can be (a) autosaved through
// useDraft — an IndexedDB put cannot clone a $state proxy — and (b) carried as
// the stage draft's payload when the operator stashes and walks away. Both paths
// hand it back to `hydrate`, which is the only place that decides what a
// half-trusted stored object is allowed to become.

import { ACTION_REGISTRY } from '$lib/components/actions/registry';
import { defaultPackageForm, defaultScheduleForm, type FormState, type ScheduleFormState, type ShellFormState } from '$lib/components/actions/forms/types';
import { COMPLIANCE_KEY, formKeyForTypeValue } from './type-tiles';

export type ActionDraft = {
	/** Orchestrator-level type string ('PACKAGE', 'DEB', 'COMPLIANCE_CHECK'), or
	 *  null while the operator is still on the type wall. */
	typeValue: string | null;
	name: string;
	description: string;
	timeoutSeconds: number;
	/** 0 = PRESENT, 1 = ABSENT. Forced to 0 for types with no ABSENT. */
	desiredState: number;
	params: FormState;
	schedule: ScheduleFormState;
};

export function emptyDraft(): ActionDraft {
	return {
		typeValue: null,
		name: '',
		description: '',
		timeoutSeconds: 300,
		desiredState: 0,
		params: defaultPackageForm(),
		schedule: defaultScheduleForm()
	};
}

/** A fresh buffer for a chosen type: registry defaults for the params, plus the
 *  two schedule presets the create form has always applied (LPS and LUKS check
 *  hourly and run on assign — rotation that only ran on a cron would leave a
 *  freshly assigned device unrotated). */
export function draftForType(typeValue: string, over: Partial<ActionDraft> = {}): ActionDraft {
	const key = formKeyForTypeValue(typeValue);
	const params = key ? ACTION_REGISTRY[key].defaultForm() : defaultPackageForm();
	// COMPLIANCE_CHECK shares the SHELL params shape; the flag is what makes the
	// stricter compliance schema the right one to validate against.
	if (typeValue === COMPLIANCE_KEY) (params as ShellFormState).isCompliance = true;
	const schedule =
		typeValue === 'LPS' || typeValue === 'ENCRYPTION'
			? { ...defaultScheduleForm(), intervalHours: 1, runOnAssign: true, skipIfUnchanged: true }
			: defaultScheduleForm();
	return { ...emptyDraft(), typeValue, params, schedule, ...over };
}

/** Rebuild a draft from a persisted autosave or a claimed stage payload. The
 *  stored object is plain JSON that may be a release old, so an unknown type
 *  string drops back to the type wall instead of feeding a mismatched params
 *  bucket to a form that cannot render it. */
export function hydrate(raw: unknown): ActionDraft | null {
	if (!raw || typeof raw !== 'object') return null;
	const d = raw as Partial<ActionDraft>;
	if (typeof d.typeValue !== 'string' || !formKeyForTypeValue(d.typeValue)) return null;
	const base = draftForType(d.typeValue);
	return {
		typeValue: d.typeValue,
		name: typeof d.name === 'string' ? d.name : '',
		description: typeof d.description === 'string' ? d.description : '',
		timeoutSeconds: typeof d.timeoutSeconds === 'number' ? d.timeoutSeconds : base.timeoutSeconds,
		desiredState: d.desiredState === 1 ? 1 : 0,
		params: d.params && typeof d.params === 'object' ? ({ ...base.params, ...d.params } as FormState) : base.params,
		schedule:
			d.schedule && typeof d.schedule === 'object'
				? { ...base.schedule, ...d.schedule }
				: base.schedule
	};
}
