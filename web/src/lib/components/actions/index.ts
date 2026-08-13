// Type utilities
export {
	getActionTypeInfo,
	getActionTypeInfoByValue,
	getActionTypeLabel,
	getActionTypeIcon,
	getActionTypeEnum,
	getActionTypeOptions,
	getGroupedActionTypeOptions,
	actionTypeToString,
	ACTION_TYPE_OPTIONS,
	type ActionTypeInfo,
	type ActionTypeGroup
} from './action-type';

// Display components
export { default as ActionTypeBadge } from './ActionTypeBadge.svelte';
export { default as ActionParamsDisplay } from './ActionParamsDisplay.svelte';
export { default as ActionScheduleDisplay } from './ActionScheduleDisplay.svelte';
export { default as ActionCreateForm } from './action-create-form.svelte';
export { default as EditParamsDialog, supportsAbsent } from './edit-params-dialog.svelte';
export { default as ActionDetailSheet, openActionSheet } from './action-detail-sheet.svelte';

// The B1 pipeline builders and the inline params editor are deliberately NOT
// re-exported here. They pull in the draft-persistence hook and the shell's
// context pill, and this barrel is imported by every list page for nothing more
// than a type label — a re-export would make those pages load (and their tests
// mock) machinery they never render. Their three consumers import them by path.

// Form components
export {
	PackageParamsForm,
	ShellParamsForm,
	ServiceParamsForm,
	FileParamsForm,
	DirectoryParamsForm,
	AppParamsForm,
	FlatpakParamsForm,
	UpdateParamsForm,
	RepositoryParamsForm,
	UserParamsForm,
	ActionScheduleForm,
	SshParamsForm,
	SshdParamsForm,
	AdminPolicyParamsForm,
	LpsParamsForm,
	EncryptionParamsForm,
	GroupParamsForm,
	WifiParamsForm,
	AgentUpdateParamsForm
} from './forms';

// Form state types, defaults, and converters
export {
	defaultPackageForm,
	defaultShellForm,
	defaultServiceForm,
	defaultFileForm,
	defaultAppForm,
	defaultFlatpakForm,
	defaultUpdateForm,
	defaultDirectoryForm,
	defaultUserForm,
	defaultRepositoryForm,
	defaultScheduleForm,
	defaultSshForm,
	defaultSshdForm,
	defaultAdminPolicyForm,
	defaultLpsForm,
	defaultEncryptionForm,
	defaultGroupForm,
	defaultWifiForm,
	defaultAgentUpdateForm,
	packageFormToProto,
	shellFormToProto,
	serviceFormToProto,
	fileFormToProto,
	appFormToProto,
	flatpakFormToProto,
	updateFormToProto,
	directoryFormToProto,
	userFormToProto,
	repositoryFormToProto,
	scheduleFormToProto,
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
	directoryProtoToForm,
	userProtoToForm,
	repositoryProtoToForm,
	sshProtoToForm,
	sshdProtoToForm,
	adminPolicyProtoToForm,
	lpsProtoToForm,
	encryptionProtoToForm,
	groupProtoToForm,
	wifiProtoToForm,
	agentUpdateProtoToForm
} from './forms';
