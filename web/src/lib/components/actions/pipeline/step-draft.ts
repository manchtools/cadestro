

import { ActionType } from '$contract/cadestro/v1/actions_pb';
import { ACTION_REGISTRY, formKeyFromActionType, formKeyFromString, type FormKey } from '../registry';
import { defaultScheduleForm, type FormState, type ScheduleFormState, type ShellFormState } from '../forms/types';
import type { ManagedAction } from '$contract/cadestro/v1/control_pb';

export interface StepDraft {

	key: string;

	actionId: string;
	formKey: FormKey;

	actionType: ActionType;
	name: string;
	description: string;
	timeoutSeconds: number;
	desiredState: number;
	params: FormState;
	schedule: ScheduleFormState;

	isNew: boolean;

	originalIndex: number;
}

let seq = 0;
function nextKey(): string {
	return `step-${++seq}`;
}

export function resetStepKeys() {
	seq = 0;
}

function scheduleToForm(action: ManagedAction): ScheduleFormState {
	const base = defaultScheduleForm();
	const s = action.schedule;
	if (!s) return base;
	return {
		cron: s.cron ?? '',
		intervalHours: s.intervalHours || base.intervalHours,
		runOnAssign: s.runOnAssign,
		skipIfUnchanged: s.skipIfUnchanged
	};
}

export function stepFromAction(action: ManagedAction, sortOrder: number): StepDraft | null {
	let key = formKeyFromActionType(action.type);
	if (!key) return null;
	if (
		key === 'SHELL' &&
		action.params.case === 'shell' &&
		action.params.value.isCompliance
	) {
		key = 'COMPLIANCE_CHECK';
	}
	const adapter = ACTION_REGISTRY[key];

	const params =
		action.params.case === adapter.paramsCase
			? adapter.protoToForm(action.params.value)
			: adapter.defaultForm();
	return {
		key: nextKey(),
		actionId: (action.id?.value ?? ''),
		formKey: key,
		actionType: action.type,
		name: action.name,
		description: action.description,
		timeoutSeconds: action.timeoutSeconds || 300,
		desiredState: action.desiredState ?? 0,
		params,
		schedule: scheduleToForm(action),
		isNew: false,
		originalIndex: sortOrder
	};
}

export function stepFromPalette(
	typeValue: string,
	actionType: ActionType,
	name: string
): StepDraft | null {
	const key = typeValue === 'COMPLIANCE_CHECK' ? 'COMPLIANCE_CHECK' : formKeyFromString(typeValue);
	if (!key) return null;
	const adapter = ACTION_REGISTRY[key];
	const params = adapter.defaultForm();

	if (key === 'COMPLIANCE_CHECK') (params as ShellFormState).isCompliance = true;
	return {
		key: nextKey(),
		actionId: '',
		formKey: key,
		actionType,
		name,
		description: '',
		timeoutSeconds: 300,
		desiredState: 0,
		params,
		schedule: defaultScheduleForm(),
		isNew: true,
		originalIndex: -1
	};
}

export function stepFromJson(raw: unknown): StepDraft | null {
	if (!raw || typeof raw !== 'object') return null;
	const s = raw as Partial<StepDraft>;
	if (typeof s.formKey !== 'string' || !(s.formKey in ACTION_REGISTRY)) return null;
	return {
		key: typeof s.key === 'string' && s.key ? s.key : nextKey(),
		actionId: typeof s.actionId === 'string' ? s.actionId : '',
		formKey: s.formKey as FormKey,
		actionType: typeof s.actionType === 'number' ? s.actionType : ActionType.UNSPECIFIED,
		name: typeof s.name === 'string' ? s.name : '',
		description: typeof s.description === 'string' ? s.description : '',
		timeoutSeconds: typeof s.timeoutSeconds === 'number' ? s.timeoutSeconds : 300,
		desiredState: typeof s.desiredState === 'number' ? s.desiredState : 0,
		params: s.params ?? ACTION_REGISTRY[s.formKey as FormKey].defaultForm(),
		schedule: (s.schedule as ScheduleFormState) ?? defaultScheduleForm(),
		isNew: s.isNew !== false,
		originalIndex: typeof s.originalIndex === 'number' ? s.originalIndex : -1
	};
}

export interface StepIssues {

	fields: Record<string, string>;

	first: string | null;
}

export function validateStep(step: StepDraft, nameRequired: string): StepIssues {
	const fields: Record<string, string> = {};
	if (!step.name.trim()) fields.name = nameRequired;

	const result = ACTION_REGISTRY[step.formKey].schema.safeParse(step.params);
	if (!result.success) {
		for (const issue of result.error.issues) {
			const field = issue.path.length ? String(issue.path[0]) : '_';
			if (!(field in fields)) fields[field] = issue.message;
		}
	}
	const first = Object.values(fields)[0] ?? null;
	return { fields, first };
}
