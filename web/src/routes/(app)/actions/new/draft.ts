

import { ACTION_REGISTRY } from '$lib/components/actions/registry';
import { defaultPackageForm, defaultScheduleForm, type FormState, type ScheduleFormState, type ShellFormState } from '$lib/components/actions/forms/types';
import { COMPLIANCE_KEY, formKeyForTypeValue } from './type-tiles';

export type ActionDraft = {

	typeValue: string | null;
	name: string;
	description: string;
	timeoutSeconds: number;

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

export function draftForType(typeValue: string, over: Partial<ActionDraft> = {}): ActionDraft {
	const key = formKeyForTypeValue(typeValue);
	const params = key ? ACTION_REGISTRY[key].defaultForm() : defaultPackageForm();

	if (typeValue === COMPLIANCE_KEY) (params as ShellFormState).isCompliance = true;
	const schedule =
		typeValue === 'LPS' || typeValue === 'ENCRYPTION'
			? { ...defaultScheduleForm(), intervalHours: 1, runOnAssign: true, skipIfUnchanged: true }
			: defaultScheduleForm();
	return { ...emptyDraft(), typeValue, params, schedule, ...over };
}

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
