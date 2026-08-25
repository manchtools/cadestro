

import type { ZodSchema } from 'zod';
import { ActionType } from '$contract/cadestro/v1/actions_pb';

import {
	defaultPackageForm,
	defaultShellForm,
	defaultServiceForm,
	defaultFileForm,
	defaultAppForm,
	defaultFlatpakForm,
	defaultUpdateForm,
	defaultRepositoryForm,
	defaultDirectoryForm,
	defaultUserForm,
	defaultSshForm,
	defaultSshdForm,
	defaultAdminPolicyForm,
	defaultLpsForm,
	defaultEncryptionForm,
	defaultGroupForm,
	defaultWifiForm,
	defaultAgentUpdateForm,
	type FormState,
	type FormStateByKey,
	packageFormToProto,
	shellFormToProto,
	serviceFormToProto,
	fileFormToProto,
	appFormToProto,
	flatpakFormToProto,
	updateFormToProto,
	repositoryFormToProto,
	directoryFormToProto,
	userFormToProto,
	sshFormToProto,
	sshdFormToProto,
	adminPolicyFormToProto,
	lpsFormToProto,
	encryptionFormToProto,
	groupFormToProto,
	wifiFormToProto,
	agentUpdateFormToProto,
	packageProtoToForm,
	shellProtoToForm,
	serviceProtoToForm,
	fileProtoToForm,
	appProtoToForm,
	flatpakProtoToForm,
	updateProtoToForm,
	repositoryProtoToForm,
	directoryProtoToForm,
	userProtoToForm,
	sshProtoToForm,
	sshdProtoToForm,
	adminPolicyProtoToForm,
	lpsProtoToForm,
	encryptionProtoToForm,
	groupProtoToForm,
	wifiProtoToForm,
	agentUpdateProtoToForm
} from './forms/types';

import {
	packageParamsSchema,
	shellParamsSchema,
	complianceShellParamsSchema,
	serviceParamsSchema,
	fileParamsSchema,
	appParamsSchema,
	flatpakParamsSchema,
	updateParamsSchema,
	repositoryParamsSchema,
	directoryParamsSchema,
	userParamsSchema,
	sshParamsSchema,
	sshdParamsSchema,
	adminPolicyParamsSchema,
	lpsParamsSchema,
	encryptionParamsSchema,
	groupParamsSchema,
	wifiParamsSchema,
	agentUpdateParamsSchema
} from '$lib/forms/schemas/actions';

export const FORM_KEYS = [
	'PACKAGE',
	'SHELL',
	'COMPLIANCE_CHECK',
	'SERVICE',
	'FILE',
	'APP',
	'FLATPAK',
	'UPDATE',
	'REPOSITORY',
	'DIRECTORY',
	'USER',
	'SSH',
	'SSHD',
	'ADMIN_POLICY',
	'LPS',
	'ENCRYPTION',
	'GROUP',
	'WIFI',
	'AGENT_UPDATE'
] as const satisfies readonly (keyof FormStateByKey)[];

export type FormKey = keyof FormStateByKey;

export type ParamsCase =
	| 'package'
	| 'shell'
	| 'service'
	| 'file'
	| 'app'
	| 'flatpak'
	| 'update'
	| 'repository'
	| 'directory'
	| 'user'
	| 'ssh'
	| 'sshd'
	| 'adminPolicy'
	| 'lps'
	| 'encryption'
	| 'group'
	| 'wifi'
	| 'agentUpdate';

export interface ActionTypeAdapter<F = FormState, P = unknown> {

	key: FormKey;

	actionType: ActionType;

	paramsCase: ParamsCase;

	supportsAbsent: boolean;

	defaultForm: () => F;

	schema: ZodSchema;

	formToProto: (form: F) => P;

	protoToForm: (proto: P) => F;
}

export const ACTION_REGISTRY: Record<FormKey, ActionTypeAdapter> = {
	PACKAGE: {
		key: 'PACKAGE',
		actionType: ActionType.PACKAGE,
		paramsCase: 'package',
		supportsAbsent: true,
		defaultForm: defaultPackageForm,
		schema: packageParamsSchema,
		formToProto: packageFormToProto as never,
		protoToForm: packageProtoToForm as never
	} as ActionTypeAdapter,
	SHELL: {
		key: 'SHELL',
		actionType: ActionType.SHELL,
		paramsCase: 'shell',
		supportsAbsent: false,
		defaultForm: defaultShellForm,
		schema: shellParamsSchema,
		formToProto: shellFormToProto as never,
		protoToForm: shellProtoToForm as never
	} as ActionTypeAdapter,
	COMPLIANCE_CHECK: {
		key: 'COMPLIANCE_CHECK',
		actionType: ActionType.SHELL,
		paramsCase: 'shell',
		supportsAbsent: false,
		defaultForm: defaultShellForm,
		schema: complianceShellParamsSchema,
		formToProto: shellFormToProto as never,
		protoToForm: shellProtoToForm as never
	} as ActionTypeAdapter,
	SERVICE: {
		key: 'SERVICE',
		actionType: ActionType.SERVICE,
		paramsCase: 'service',
		supportsAbsent: true,
		defaultForm: defaultServiceForm,
		schema: serviceParamsSchema,
		formToProto: serviceFormToProto as never,
		protoToForm: serviceProtoToForm as never
	} as ActionTypeAdapter,
	FILE: {
		key: 'FILE',
		actionType: ActionType.FILE,
		paramsCase: 'file',
		supportsAbsent: true,
		defaultForm: defaultFileForm,
		schema: fileParamsSchema,
		formToProto: fileFormToProto as never,
		protoToForm: fileProtoToForm as never
	} as ActionTypeAdapter,
	APP: {

		key: 'APP',
		actionType: ActionType.APP_IMAGE,
		paramsCase: 'app',
		supportsAbsent: true,
		defaultForm: defaultAppForm,
		schema: appParamsSchema,
		formToProto: appFormToProto as never,
		protoToForm: appProtoToForm as never
	} as ActionTypeAdapter,
	FLATPAK: {
		key: 'FLATPAK',
		actionType: ActionType.FLATPAK,
		paramsCase: 'flatpak',
		supportsAbsent: true,
		defaultForm: defaultFlatpakForm,
		schema: flatpakParamsSchema,
		formToProto: flatpakFormToProto as never,
		protoToForm: flatpakProtoToForm as never
	} as ActionTypeAdapter,
	UPDATE: {
		key: 'UPDATE',
		actionType: ActionType.UPDATE,
		paramsCase: 'update',
		supportsAbsent: false,
		defaultForm: defaultUpdateForm,
		schema: updateParamsSchema,
		formToProto: updateFormToProto as never,
		protoToForm: updateProtoToForm as never
	} as ActionTypeAdapter,
	REPOSITORY: {
		key: 'REPOSITORY',
		actionType: ActionType.REPOSITORY,
		paramsCase: 'repository',
		supportsAbsent: true,
		defaultForm: defaultRepositoryForm,
		schema: repositoryParamsSchema,
		formToProto: repositoryFormToProto as never,
		protoToForm: repositoryProtoToForm as never
	} as ActionTypeAdapter,
	DIRECTORY: {
		key: 'DIRECTORY',
		actionType: ActionType.DIRECTORY,
		paramsCase: 'directory',
		supportsAbsent: true,
		defaultForm: defaultDirectoryForm,
		schema: directoryParamsSchema,
		formToProto: directoryFormToProto as never,
		protoToForm: directoryProtoToForm as never
	} as ActionTypeAdapter,
	USER: {
		key: 'USER',
		actionType: ActionType.USER,
		paramsCase: 'user',
		supportsAbsent: true,
		defaultForm: defaultUserForm,
		schema: userParamsSchema,
		formToProto: userFormToProto as never,
		protoToForm: userProtoToForm as never
	} as ActionTypeAdapter,
	SSH: {
		key: 'SSH',
		actionType: ActionType.SSH,
		paramsCase: 'ssh',
		supportsAbsent: true,
		defaultForm: defaultSshForm,
		schema: sshParamsSchema,
		formToProto: sshFormToProto as never,
		protoToForm: sshProtoToForm as never
	} as ActionTypeAdapter,
	SSHD: {
		key: 'SSHD',
		actionType: ActionType.SSHD,
		paramsCase: 'sshd',
		supportsAbsent: true,
		defaultForm: defaultSshdForm,
		schema: sshdParamsSchema,
		formToProto: sshdFormToProto as never,
		protoToForm: sshdProtoToForm as never
	} as ActionTypeAdapter,
	ADMIN_POLICY: {
		key: 'ADMIN_POLICY',
		actionType: ActionType.ADMIN_POLICY,
		paramsCase: 'adminPolicy',
		supportsAbsent: true,
		defaultForm: defaultAdminPolicyForm,
		schema: adminPolicyParamsSchema,
		formToProto: adminPolicyFormToProto as never,
		protoToForm: adminPolicyProtoToForm as never
	} as ActionTypeAdapter,
	LPS: {
		key: 'LPS',
		actionType: ActionType.LPS,
		paramsCase: 'lps',
		supportsAbsent: true,
		defaultForm: defaultLpsForm,
		schema: lpsParamsSchema,
		formToProto: lpsFormToProto as never,
		protoToForm: lpsProtoToForm as never
	} as ActionTypeAdapter,
	ENCRYPTION: {
		key: 'ENCRYPTION',
		actionType: ActionType.ENCRYPTION,
		paramsCase: 'encryption',
		supportsAbsent: true,
		defaultForm: defaultEncryptionForm,
		schema: encryptionParamsSchema,
		formToProto: encryptionFormToProto as never,
		protoToForm: encryptionProtoToForm as never
	} as ActionTypeAdapter,
	GROUP: {
		key: 'GROUP',
		actionType: ActionType.GROUP,
		paramsCase: 'group',
		supportsAbsent: true,
		defaultForm: defaultGroupForm,
		schema: groupParamsSchema,
		formToProto: groupFormToProto as never,
		protoToForm: groupProtoToForm as never
	} as ActionTypeAdapter,
	WIFI: {
		key: 'WIFI',
		actionType: ActionType.WIFI,
		paramsCase: 'wifi',
		supportsAbsent: true,
		defaultForm: defaultWifiForm,
		schema: wifiParamsSchema,
		formToProto: wifiFormToProto as never,
		protoToForm: wifiProtoToForm as never
	} as ActionTypeAdapter,
	AGENT_UPDATE: {
		key: 'AGENT_UPDATE',
		actionType: ActionType.AGENT_UPDATE,
		paramsCase: 'agentUpdate',
		supportsAbsent: false,
		defaultForm: defaultAgentUpdateForm,
		schema: agentUpdateParamsSchema,
		formToProto: agentUpdateFormToProto as never,
		protoToForm: agentUpdateProtoToForm as never
	} as ActionTypeAdapter
};

export function formKeyFromActionType(type: ActionType): FormKey | null {
	switch (type) {
		case ActionType.PACKAGE:
			return 'PACKAGE';
		case ActionType.SHELL:
			return 'SHELL';
		case ActionType.SERVICE:
			return 'SERVICE';
		case ActionType.FILE:
			return 'FILE';
		case ActionType.APP_IMAGE:
		case ActionType.DEB:
		case ActionType.RPM:
			return 'APP';
		case ActionType.FLATPAK:
			return 'FLATPAK';
		case ActionType.UPDATE:
			return 'UPDATE';
		case ActionType.REPOSITORY:
			return 'REPOSITORY';
		case ActionType.DIRECTORY:
			return 'DIRECTORY';
		case ActionType.USER:
			return 'USER';
		case ActionType.SSH:
			return 'SSH';
		case ActionType.SSHD:
			return 'SSHD';
		case ActionType.ADMIN_POLICY:
			return 'ADMIN_POLICY';
		case ActionType.LPS:
			return 'LPS';
		case ActionType.ENCRYPTION:
			return 'ENCRYPTION';
		case ActionType.GROUP:
			return 'GROUP';
		case ActionType.WIFI:
			return 'WIFI';
		case ActionType.AGENT_UPDATE:
			return 'AGENT_UPDATE';
		default:
			return null;
	}
}

export function formKeyFromString(value: string): FormKey | null {
	if ((FORM_KEYS as readonly string[]).includes(value)) {
		return value as FormKey;
	}
	if (value === 'APP_IMAGE' || value === 'DEB' || value === 'RPM') {
		return 'APP';
	}
	return null;
}
