// Marketplace template → PM control-server import flow.
//
// Each template type's `content` payload has an opinionated JSON shape
// (the marketplace emits it, this code consumes it). When a shape
// cannot be parsed the importer throws a descriptive error rather
// than partially creating resources. On the multi-step flows (sets
// and definitions) we short-circuit on the first failure; any already-
// created children are NOT rolled back — the caller surfaces the
// error, the operator can delete the partial leftovers from the
// normal UI.

import { create } from '@bufbuild/protobuf';
import { OnFailure } from '$contract/cadestro/v1/agent_pb';

import { apiClient } from '$lib/sdk';
import { ActionScheduleSchema } from '$contract/cadestro/v1/actions_pb';

// Default container schedule applied when the marketplace template
// doesn't carry one explicitly. Uses the server's eight-hour default.
// The operator can adjust it
// later via the action set / definition edit form.
const defaultContainerSchedule = () =>
	create(ActionScheduleSchema, { intervalHours: 8 });

/**
 * Template payload arriving from the marketplace iframe via
 * postMessage. Shape defined by the `pm.marketplace.import` message
 * (see src/lib/marketplace/embed.ts).
 */
export type TemplateType = 'ACTION' | 'ACTION_SET' | 'DEFINITION' | 'COMPLIANCE_POLICY';

export interface Template {
	id: string;
	name: string;
	templateType: TemplateType;
	content: unknown;
}

export interface ImportResult {
	/** The route the caller should navigate to for inspecting the new object. */
	redirect: string;
	/** The server-assigned identifier of the top-level created object. */
	id: string;
	/** Human-readable name (for the success toast). */
	name: string;
}

export class ImportError extends Error {
	readonly phase: string;
	constructor(phase: string, message: string) {
		super(`${phase}: ${message}`);
		this.name = 'ImportError';
		this.phase = phase;
	}
}

/** Expected shapes for each template type's `content` payload. */
interface ActionContent {
	// Opaque — the marketplace-emitted shape matches the PM
	// CreateActionRequest JSON serialization. The apiClient.createAction
	// call validates it via proto schema; we let that do the work and
	// surface a descriptive error on mismatch.
	action: Record<string, unknown>;
}

interface ActionSetContent {
	set: { name: string; description?: string };
	actions?: Record<string, unknown>[]; // CreateAction-shaped children
}

interface DefinitionContent {
	definition: { name: string; description?: string };
	actionSets?: Array<{
		set: { name: string; description?: string };
		actions?: Record<string, unknown>[];
	}>;
}

interface CompliancePolicyContent {
	policy: { name: string; description?: string };
}

export async function importTemplate(template: Template): Promise<ImportResult> {
	switch (template.templateType) {
		case 'ACTION':
			return importAction(template);
		case 'ACTION_SET':
			return importActionSet(template);
		case 'DEFINITION':
			return importDefinition(template);
		case 'COMPLIANCE_POLICY':
			return importCompliancePolicy(template);
		default:
			throw new ImportError('validate', `unsupported template type: ${template.templateType satisfies TemplateType}`);
	}
}

function parse<T>(template: Template, guard: (v: unknown) => v is T): T {
	const content = template.content;
	if (!content || typeof content !== 'object') {
		throw new ImportError('parse', 'template content is empty or not an object');
	}
	if (!guard(content)) {
		throw new ImportError('parse', `content shape does not match ${template.templateType} expectations`);
	}
	return content;
}

// --- Per-type importers ----------------------------------------------------

async function importAction(template: Template): Promise<ImportResult> {
	const content = parse<ActionContent>(template, isActionContent);
	const action = await apiClient.createAction(content.action as Parameters<typeof apiClient.createAction>[0]);
	if (!action?.id) {
		throw new ImportError('createAction', 'server returned action without an id');
	}
	return { redirect: '/actions/' + (action.id?.value ?? ''), id: (action.id?.value ?? ''), name: action.name ?? template.name };
}

async function importActionSet(template: Template): Promise<ImportResult> {
	const content = parse<ActionSetContent>(template, isActionSetContent);
	const set = await apiClient.createActionSet({
		name: content.set.name,
		description: content.set.description ?? '',
		schedule: defaultContainerSchedule(),
		onFailure: OnFailure.CONTINUE
	});
	if (!set?.id) throw new ImportError('createActionSet', 'server returned set without an id');

	for (const [i, actionSpec] of (content.actions ?? []).entries()) {
		let action;
		try {
			action = await apiClient.createAction(actionSpec as Parameters<typeof apiClient.createAction>[0]);
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			throw new ImportError(`createAction[${i}]`, msg);
		}
		if (!action?.id) {
			throw new ImportError(`createAction[${i}]`, 'server returned action without an id');
		}
		try {
			await apiClient.addActionToSet((set.id?.value ?? ''), (action.id?.value ?? ''), i);
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			throw new ImportError(`addActionToSet[${i}]`, msg);
		}
	}
	return { redirect: '/action-sets/' + (set.id?.value ?? ''), id: (set.id?.value ?? ''), name: set.name ?? template.name };
}

async function importDefinition(template: Template): Promise<ImportResult> {
	const content = parse<DefinitionContent>(template, isDefinitionContent);
	const def = await apiClient.createDefinition({
		name: content.definition.name,
		description: content.definition.description ?? '',
		schedule: defaultContainerSchedule(),
	});
	if (!def?.id) throw new ImportError('createDefinition', 'server returned definition without an id');

	for (const [i, setSpec] of (content.actionSets ?? []).entries()) {
		const set = await apiClient.createActionSet({
			name: setSpec.set.name,
			description: setSpec.set.description ?? '',
			schedule: defaultContainerSchedule(),
			onFailure: OnFailure.CONTINUE
		});
		if (!set?.id) throw new ImportError(`createActionSet[${i}]`, 'server returned set without an id');

		for (const [j, actionSpec] of (setSpec.actions ?? []).entries()) {
			const action = await apiClient.createAction(actionSpec as Parameters<typeof apiClient.createAction>[0]);
			if (!action?.id) {
				throw new ImportError(`createAction[${i}.${j}]`, 'server returned action without an id');
			}
			await apiClient.addActionToSet((set.id?.value ?? ''), (action.id?.value ?? ''), j);
		}
		await apiClient.addActionSetToDefinition((def.id?.value ?? ''), (set.id?.value ?? ''), i);
	}
	return { redirect: '/definitions/' + (def.id?.value ?? ''), id: (def.id?.value ?? ''), name: def.name ?? template.name };
}

async function importCompliancePolicy(template: Template): Promise<ImportResult> {
	const content = parse<CompliancePolicyContent>(template, isCompliancePolicyContent);
	const policy = await apiClient.createCompliancePolicy(content.policy.name, content.policy.description ?? '');
	if (!policy?.id) throw new ImportError('createCompliancePolicy', 'server returned policy without an id');
	return { redirect: '/compliance-policies/' + (policy.id?.value ?? ''), id: (policy.id?.value ?? ''), name: policy.name ?? template.name };
}

// --- Type guards ----------------------------------------------------------

function isActionContent(v: unknown): v is ActionContent {
	return typeof v === 'object' && v !== null && 'action' in v && typeof (v as { action: unknown }).action === 'object';
}

function isActionSetContent(v: unknown): v is ActionSetContent {
	if (typeof v !== 'object' || v === null) return false;
	const c = v as ActionSetContent;
	return typeof c.set === 'object' && typeof c.set?.name === 'string';
}

function isDefinitionContent(v: unknown): v is DefinitionContent {
	if (typeof v !== 'object' || v === null) return false;
	const c = v as DefinitionContent;
	return typeof c.definition === 'object' && typeof c.definition?.name === 'string';
}

function isCompliancePolicyContent(v: unknown): v is CompliancePolicyContent {
	if (typeof v !== 'object' || v === null) return false;
	const c = v as CompliancePolicyContent;
	return typeof c.policy === 'object' && typeof c.policy?.name === 'string';
}
