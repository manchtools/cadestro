import { create, fromJson, toJson } from '@bufbuild/protobuf';
import { PackageActionParamsSchema, UpdateActionParamsSchema, ShellActionParamsSchema } from '$contract/cadestro/v1/actions_pb';
import { DesiredState } from '$contract/cadestro/v1/common_pb';
import { CreateActionRequestSchema, type CreateActionRequest, type ManagedAction } from '$contract/cadestro/v1/control_pb';
export type ActionDraft = CreateActionRequest;
export function emptyDraft(): ActionDraft { return create(CreateActionRequestSchema, { desiredState: DesiredState.PRESENT, timeoutSeconds: 300, schedule: { intervalHours: 24 } }); }
export function draftForType(value: string): ActionDraft {
 const draft = emptyDraft();
 if (value === 'PACKAGE') return create(CreateActionRequestSchema, { ...draft, params: { case: 'package', value: create(PackageActionParamsSchema) } });
 if (value === 'UPDATE') return create(CreateActionRequestSchema, { ...draft, params: { case: 'update', value: create(UpdateActionParamsSchema) } });
 return create(CreateActionRequestSchema, { ...draft, params: { case: 'shell', value: create(ShellActionParamsSchema, { interpreter: '/bin/bash', isCompliance: value === 'COMPLIANCE_CHECK' }) } });
}
export function draftFromAction(action: ManagedAction): ActionDraft {
 return create(CreateActionRequestSchema, { name: action.name, description: action.description, desiredState: action.desiredState, timeoutSeconds: action.timeoutSeconds, schedule: action.schedule, params: action.params });
}
export function hydrate(raw: unknown): ActionDraft | null {
 if (typeof raw !== 'string') return null;
 try { return fromJson(CreateActionRequestSchema, JSON.parse(raw)); } catch { return null; }
}
export function serialize(draft: ActionDraft): string { return JSON.stringify(toJson(CreateActionRequestSchema, draft)); }
export function draftErrors(draft: ActionDraft): Record<string, string> {
 const errors: Record<string, string> = {};
 if (!draft.name.trim()) errors.name = 'Enter an action name';
 if (draft.timeoutSeconds < 0 || draft.timeoutSeconds > 3600) errors.timeoutSeconds = 'Timeout must be between 0 and 3600 seconds';
 if (!draft.params.case) errors.params = 'Choose an action type';
 if (draft.params.case === 'package' && !draft.params.value.name.trim()) errors.packageName = 'Enter a package name';
 if (draft.params.case === 'shell') {
  if (!draft.params.value.script.trim() && !draft.params.value.detectionScript.trim()) errors.script = 'Enter a script or detection script';
  if (draft.params.value.isCompliance && !draft.params.value.detectionScript.trim()) errors.detectionScript = 'Enter a compliance detection script';
 }
 return errors;
}
