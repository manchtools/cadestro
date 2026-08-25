import { create } from '@bufbuild/protobuf';
import {
	PackageParamsSchema,
	ShellParamsSchema,
	ServiceParamsSchema,
	FileParamsSchema,
	AppInstallParamsSchema,
	FlatpakParamsSchema,
	UpdateParamsSchema,
	RepositoryParamsSchema,
	DirectoryParamsSchema,
	UserParamsSchema,
	AptRepositorySchema,
	DnfRepositorySchema,
	PacmanRepositorySchema,
	ZypperRepositorySchema,
	ActionScheduleSchema,
	type PackageParams,
	type ShellParams,
	type ServiceParams,
	type FileParams,
	type AppInstallParams,
	type FlatpakParams,
	type UpdateParams,
	type RepositoryParams,
	type DirectoryParams,
	type UserParams,
	SshParamsSchema,
	type SshParams,
	SshdParamsSchema,
	SshdDirectiveSchema,
	type SshdParams,
	AdminPolicyParamsSchema,
	type AdminPolicyParams,
	AdminAccessLevel,
	PrivilegeBackend,
	LpsParamsSchema,
	type LpsParams,
	LpsPasswordComplexity,
	GroupParamsSchema,
	type GroupParams,
	ZypperRepositoryType,
	EncryptionDeviceBoundKeyType,
	WifiAuthType,
	AgentUpdateParamsSchema,
	AgentUpdateArchSchema,
	type AgentUpdateParams
} from '$contract/cadestro/v1/actions_pb';
import {
	EncryptionAuthoringParamsSchema,
	type EncryptionAuthoringParams,
	type ManagedEncryptionParams,
	WifiAuthoringParamsSchema,
	type WifiAuthoringParams,
	type ManagedWifiParams
} from '$contract/cadestro/v1/control_pb';

export interface PackageFormState {
	name: string;
	version: string;
	allowDowngrade: boolean;
	pin: boolean;
	aptName: string;
	dnfName: string;
	pacmanName: string;
	zypperName: string;
}

export interface ShellFormState {
	script: string;
	interpreter: string;
	runAsRoot: boolean;
	detectionScript: string;
	isCompliance: boolean;
}

export interface ServiceFormState {
	unitName: string;
	desiredState: string;
	enable: boolean;
}

export interface FileFormState {
	path: string;
	content: string;
	owner: string;
	group: string;
	mode: string;
	managedBlock: boolean;
}

export interface AppFormState {
	url: string;
	checksumSha256: string;
	installPath: string;
}

export interface FlatpakFormState {
	appId: string;
	remote: string;
	systemWide: boolean;
	pin: boolean;
}

export interface UpdateFormState {
	securityOnly: boolean;
	autoremove: boolean;
	rebootIfRequired: boolean;
}

export interface DirectoryFormState {
	path: string;
	owner: string;
	group: string;
	mode: string;
	recursive: boolean;
}

export interface UserFormState {
	username: string;
	uid: number;
	gid: number;
	homeDir: string;
	shell: string;
	sshAuthorizedKeys: string[];
	comment: string;
	systemUser: boolean;
	createHome: boolean;
	disabled: boolean;
	primaryGroup: string;
	hidden: boolean;
}

export interface GroupFormState {
	name: string;
	members: string[];
	gid: number;
	systemGroup: boolean;
}

export interface AptFormState {
	url: string;
	distribution: string;
	components: string;
	gpgKeyUrl: string;
	gpgKey: string;
	trusted: boolean;
	arch: string;
	disabled: boolean;
}

export interface DnfFormState {
	baseurl: string;
	description: string;
	enabled: boolean;
	gpgcheck: boolean;
	gpgkey: string;
	moduleHotfixes: boolean;
	disabled: boolean;
}

export interface PacmanFormState {
	server: string;
	sigLevel: string;
	disabled: boolean;
}

export interface ZypperFormState {
	url: string;
	description: string;
	enabled: boolean;
	autorefresh: boolean;
	gpgcheck: boolean;
	gpgkey: string;
	type: string;
	disabled: boolean;
}

export interface RepositoryFormState {
	name: string;
	apt: AptFormState;
	dnf: DnfFormState;
	pacman: PacmanFormState;
	zypper: ZypperFormState;
}

export interface ScheduleFormState {
	cron: string;
	intervalHours: number;
	runOnAssign: boolean;
	skipIfUnchanged: boolean;
}

export interface SshFormState {
	users: string[];
	allowPubkey: boolean;
	allowPassword: boolean;
}

export interface SshdDirectiveFormState {
	key: string;
	value: string;
}

export interface SshdFormState {
	directives: SshdDirectiveFormState[];
}

export interface AdminPolicyFormState {
	accessLevel: string;
	users: string[];
	customConfig: string;
	backend: string;
}

export interface LpsFormState {
	usernames: string[];
	passwordLength: number;
	complexity: string;
	rotationIntervalDays: number;
	gracePeriodHours: number;
}

export interface EncryptionFormState {
	presharedKey: string;
	presharedKeyConfigured: boolean;
	rotationIntervalDays: number;
	minWords: number;
	deviceBoundKeyType: string;
	userPassphraseMinLength: number;
	userPassphraseComplexity: string;
}

export interface WifiFormState {
	ssid: string;
	authType: string;
	psk: string;
	pskConfigured: boolean;
	caCert: string;
	clientCert: string;
	clientKey: string;
	clientKeyConfigured: boolean;
	identity: string;
	autoConnect: boolean;
	hidden: boolean;
	priority: number;
}

export interface AgentUpdateFormState {
	amd64BinaryUrl: string;
	amd64ChecksumUrl: string;
	arm64BinaryUrl: string;
	arm64ChecksumUrl: string;
	allowRedirect: boolean;
}

export interface FormStateByKey {
	PACKAGE: PackageFormState;
	SHELL: ShellFormState;
	COMPLIANCE_CHECK: ShellFormState;
	SERVICE: ServiceFormState;
	FILE: FileFormState;
	APP: AppFormState;
	FLATPAK: FlatpakFormState;
	UPDATE: UpdateFormState;
	REPOSITORY: RepositoryFormState;
	DIRECTORY: DirectoryFormState;
	USER: UserFormState;
	SSH: SshFormState;
	SSHD: SshdFormState;
	ADMIN_POLICY: AdminPolicyFormState;
	LPS: LpsFormState;
	ENCRYPTION: EncryptionFormState;
	GROUP: GroupFormState;
	WIFI: WifiFormState;
	AGENT_UPDATE: AgentUpdateFormState;
}

export type FormState = FormStateByKey[keyof FormStateByKey];

export function defaultPackageForm(): PackageFormState {
	return {
		name: '',
		version: '',
		allowDowngrade: false,
		pin: false,
		aptName: '',
		dnfName: '',
		pacmanName: '',
		zypperName: ''
	};
}

export function defaultShellForm(): ShellFormState {
	return { script: '', interpreter: '/bin/bash', runAsRoot: false, detectionScript: '', isCompliance: false };
}

export function defaultServiceForm(): ServiceFormState {
	return { unitName: '', desiredState: 'RUNNING', enable: true };
}

export function defaultFileForm(): FileFormState {
	return { path: '', content: '', owner: '', group: '', mode: '0644', managedBlock: false };
}

export function defaultAppForm(): AppFormState {
	return { url: '', checksumSha256: '', installPath: '' };
}

export function defaultFlatpakForm(): FlatpakFormState {
	return { appId: '', remote: '', systemWide: true, pin: false };
}

export function defaultUpdateForm(): UpdateFormState {
	return { securityOnly: false, autoremove: false, rebootIfRequired: false };
}

export function defaultDirectoryForm(): DirectoryFormState {
	return { path: '', owner: '', group: '', mode: '0755', recursive: true };
}

export function defaultUserForm(): UserFormState {
	return {
		username: '',
		uid: 0,
		gid: 0,
		homeDir: '',
		shell: '',
		sshAuthorizedKeys: [],
		comment: '',
		systemUser: false,
		createHome: true,
		disabled: false,
		primaryGroup: '',
		hidden: false
	};
}

export function defaultGroupForm(): GroupFormState {
	return { name: '', members: [], gid: 0, systemGroup: false };
}

export function defaultAptForm(): AptFormState {
	return {
		url: '',
		distribution: '',
		components: '',
		gpgKeyUrl: '',
		gpgKey: '',
		trusted: false,
		arch: '',
		disabled: true
	};
}

export function defaultDnfForm(): DnfFormState {
	return {
		baseurl: '',
		description: '',
		enabled: true,
		gpgcheck: true,
		gpgkey: '',
		moduleHotfixes: false,
		disabled: true
	};
}

export function defaultPacmanForm(): PacmanFormState {
	return { server: '', sigLevel: '', disabled: true };
}

export function defaultZypperForm(): ZypperFormState {
	return {
		url: '',
		description: '',
		enabled: true,
		autorefresh: true,
		gpgcheck: true,
		gpgkey: '',
		type: '',
		disabled: true
	};
}

export function defaultRepositoryForm(): RepositoryFormState {
	return {
		name: '',
		apt: defaultAptForm(),
		dnf: defaultDnfForm(),
		pacman: defaultPacmanForm(),
		zypper: defaultZypperForm()
	};
}

export function defaultScheduleForm(): ScheduleFormState {
	return { cron: '', intervalHours: 8, runOnAssign: true, skipIfUnchanged: true };
}

export function defaultSshForm(): SshFormState {
	return { users: [], allowPubkey: true, allowPassword: false };
}

export function defaultSshdForm(): SshdFormState {
	return { directives: [] };
}

export function defaultAdminPolicyForm(): AdminPolicyFormState {
	return { accessLevel: 'FULL', users: [], customConfig: '', backend: 'SUDO' };
}

export function defaultLpsForm(): LpsFormState {
	return {
		usernames: [],
		passwordLength: 24,
		complexity: 'COMPLEX',
		rotationIntervalDays: 30,
		gracePeriodHours: 24
	};
}

export function defaultEncryptionForm(): EncryptionFormState {
	return {
		presharedKey: '',
		presharedKeyConfigured: false,
		rotationIntervalDays: 30,
		minWords: 5,
		deviceBoundKeyType: 'NONE',
		userPassphraseMinLength: 16,
		userPassphraseComplexity: 'COMPLEX'
	};
}

export function defaultWifiForm(): WifiFormState {
	return {
		ssid: '',
		authType: 'PSK',
		psk: '',
		pskConfigured: false,
		caCert: '',
		clientCert: '',
		clientKey: '',
		clientKeyConfigured: false,
		identity: '',
		autoConnect: true,
		hidden: false,
		priority: 0
	};
}

export function defaultAgentUpdateForm(): AgentUpdateFormState {
	return {
		amd64BinaryUrl: 'https://github.com/manchtools/cadestro/releases/latest/download/cadestrod-linux-amd64',
		amd64ChecksumUrl: 'https://github.com/manchtools/cadestro/releases/latest/download/SHA256SUMS',
		arm64BinaryUrl: 'https://github.com/manchtools/cadestro/releases/latest/download/cadestrod-linux-arm64',
		arm64ChecksumUrl: 'https://github.com/manchtools/cadestro/releases/latest/download/SHA256SUMS',

		allowRedirect: true
	};
}

function serviceStateToEnum(state: string): number {
	switch (state) {
		case 'RUNNING':
			return 1;
		case 'STOPPED':
			return 2;
		case 'RESTARTED':
			return 3;
		default:
			return 0;
	}
}

export function packageFormToProto(form: PackageFormState) {
	return create(PackageParamsSchema, { ...form });
}

export function shellFormToProto(form: ShellFormState) {
	return create(ShellParamsSchema, {
		script: form.script,
		interpreter: form.interpreter,
		runAsRoot: form.runAsRoot,
		detectionScript: form.detectionScript,
		isCompliance: form.isCompliance
	});
}

export function serviceFormToProto(form: ServiceFormState) {
	return create(ServiceParamsSchema, {
		unitName: form.unitName,
		desiredState: serviceStateToEnum(form.desiredState),
		enable: form.enable
	});
}

export function fileFormToProto(form: FileFormState) {
	return create(FileParamsSchema, { ...form });
}

export function appFormToProto(form: AppFormState) {
	return create(AppInstallParamsSchema, { ...form });
}

export function flatpakFormToProto(form: FlatpakFormState) {
	return create(FlatpakParamsSchema, { ...form, appId: { value: form.appId } });
}

export function updateFormToProto(form: UpdateFormState) {
	return create(UpdateParamsSchema, { ...form });
}

export function directoryFormToProto(form: DirectoryFormState) {
	return create(DirectoryParamsSchema, { ...form });
}

export function userFormToProto(form: UserFormState) {
	return create(UserParamsSchema, {
		username: form.username,
		uid: form.uid || undefined,
		gid: form.gid || undefined,
		homeDir: form.homeDir || undefined,
		shell: form.shell || undefined,
		comment: form.comment || undefined,
		systemUser: form.systemUser,
		createHome: form.createHome,
		disabled: form.disabled,
		primaryGroup: form.primaryGroup || undefined,
		sshAuthorizedKeys: form.sshAuthorizedKeys.filter((k) => k.trim()),
		hidden: form.hidden
	});
}

export function groupFormToProto(form: GroupFormState) {
	return create(GroupParamsSchema, {
		name: form.name,
		members: form.members.map((u) => u.trim()).filter(Boolean),
		gid: form.gid || undefined,
		systemGroup: form.systemGroup
	});
}

export function repositoryFormToProto(form: RepositoryFormState) {
	return create(RepositoryParamsSchema, {
		name: form.name,
		apt: form.apt.disabled
			? undefined
			: create(AptRepositorySchema, {
					url: form.apt.url,
					distribution: form.apt.distribution,
					components: form.apt.components.split(/\s+/).filter(Boolean),
					gpgKeyUrl: form.apt.gpgKeyUrl,
					gpgKey: form.apt.gpgKey,
					trusted: form.apt.trusted,
					arch: form.apt.arch,
					disabled: form.apt.disabled
				}),
		dnf: form.dnf.disabled
			? undefined
			: create(DnfRepositorySchema, {
					baseurl: form.dnf.baseurl,
					description: form.dnf.description,
					enabled: form.dnf.enabled,
					gpgcheck: form.dnf.gpgcheck,
					gpgkey: form.dnf.gpgkey,
					moduleHotfixes: form.dnf.moduleHotfixes,
					disabled: form.dnf.disabled
				}),
		pacman: form.pacman.disabled
			? undefined
			: create(PacmanRepositorySchema, {
					server: form.pacman.server,
					sigLevel: form.pacman.sigLevel,
					disabled: form.pacman.disabled
				}),
		zypper: form.zypper.disabled
			? undefined
			: create(ZypperRepositorySchema, {
					url: form.zypper.url,
					description: form.zypper.description,
					enabled: form.zypper.enabled,
					autorefresh: form.zypper.autorefresh,
					gpgcheck: form.zypper.gpgcheck,
					gpgkey: form.zypper.gpgkey,
					type: zypperTypeToEnum(form.zypper.type),
					disabled: form.zypper.disabled
				})
	});
}

export function scheduleFormToProto(form: ScheduleFormState) {
	return create(ActionScheduleSchema, {
		cron: form.cron || undefined,
		intervalHours: form.intervalHours,
		runOnAssign: form.runOnAssign,
		skipIfUnchanged: form.skipIfUnchanged
	});
}

export function sshFormToProto(form: SshFormState) {
	return create(SshParamsSchema, {
		users: form.users.map((u) => u.trim()).filter(Boolean),
		allowPubkey: form.allowPubkey,
		allowPassword: form.allowPassword
	});
}

export function sshdFormToProto(form: SshdFormState) {
	return create(SshdParamsSchema, {
		directives: form.directives.map((d) =>
			create(SshdDirectiveSchema, { key: d.key, value: d.value })
		)
	});
}

function adminAccessLevelToEnum(level: string): AdminAccessLevel {
	switch (level) {
		case 'FULL':
			return AdminAccessLevel.FULL;
		case 'LIMITED':
			return AdminAccessLevel.LIMITED;
		case 'CUSTOM':
			return AdminAccessLevel.CUSTOM;
		default:
			return AdminAccessLevel.UNSPECIFIED;
	}
}

function zypperTypeToEnum(type: string): ZypperRepositoryType {
	switch (type) {
		case 'rpm-md':
			return ZypperRepositoryType.RPM_MD;
		case 'yast2':
			return ZypperRepositoryType.YAST2;
		case 'plaindir':
			return ZypperRepositoryType.PLAINDIR;
		default:
			return ZypperRepositoryType.UNSPECIFIED;
	}
}

function zypperTypeFromEnum(type: ZypperRepositoryType): string {
	switch (type) {
		case ZypperRepositoryType.RPM_MD:
			return 'rpm-md';
		case ZypperRepositoryType.YAST2:
			return 'yast2';
		case ZypperRepositoryType.PLAINDIR:
			return 'plaindir';
		default:
			return '';
	}
}

function adminAccessLevelFromEnum(level: AdminAccessLevel): string {
	switch (level) {
		case AdminAccessLevel.FULL:
			return 'FULL';
		case AdminAccessLevel.LIMITED:
			return 'LIMITED';
		case AdminAccessLevel.CUSTOM:
			return 'CUSTOM';
		default:
			return 'FULL';
	}
}

function privilegeBackendToEnum(backend: string): PrivilegeBackend {
	switch (backend) {
		case 'DOAS':
			return PrivilegeBackend.DOAS;
		case 'SUDO':
		default:
			return PrivilegeBackend.SUDO;
	}
}

function privilegeBackendFromEnum(backend: PrivilegeBackend): string {
	switch (backend) {
		case PrivilegeBackend.DOAS:
			return 'DOAS';
		default:
			return 'SUDO';
	}
}

export function adminPolicyFormToProto(form: AdminPolicyFormState) {
	return create(AdminPolicyParamsSchema, {
		accessLevel: adminAccessLevelToEnum(form.accessLevel),
		users: form.users.map((u) => u.trim()).filter(Boolean),
		customConfig: form.accessLevel === 'CUSTOM' ? form.customConfig : undefined,
		backend: privilegeBackendToEnum(form.backend)
	});
}

function lpsComplexityToEnum(complexity: string): LpsPasswordComplexity {
	switch (complexity) {
		case 'ALPHANUMERIC':
			return LpsPasswordComplexity.ALPHANUMERIC;
		case 'COMPLEX':
			return LpsPasswordComplexity.COMPLEX;
		default:
			return LpsPasswordComplexity.COMPLEX;
	}
}

function lpsComplexityFromEnum(complexity: LpsPasswordComplexity): string {
	switch (complexity) {
		case LpsPasswordComplexity.ALPHANUMERIC:
			return 'ALPHANUMERIC';
		case LpsPasswordComplexity.COMPLEX:
			return 'COMPLEX';
		default:
			return 'COMPLEX';
	}
}

export function lpsFormToProto(form: LpsFormState) {
	return create(LpsParamsSchema, {
		usernames: form.usernames.map((u) => u.trim()).filter(Boolean),
		passwordLength: form.passwordLength,
		complexity: lpsComplexityToEnum(form.complexity),
		rotationIntervalDays: form.rotationIntervalDays,
		gracePeriodHours: form.gracePeriodHours
	});
}

function encryptionDeviceBoundKeyToEnum(type: string): EncryptionDeviceBoundKeyType {
	switch (type) {
		case 'NONE':
			return EncryptionDeviceBoundKeyType.NONE;
		case 'TPM':
			return EncryptionDeviceBoundKeyType.TPM;
		case 'USER_PASSPHRASE':
			return EncryptionDeviceBoundKeyType.USER_PASSPHRASE;
		default:
			return EncryptionDeviceBoundKeyType.NONE;
	}
}

function encryptionDeviceBoundKeyFromEnum(type: EncryptionDeviceBoundKeyType): string {
	switch (type) {
		case EncryptionDeviceBoundKeyType.TPM:
			return 'TPM';
		case EncryptionDeviceBoundKeyType.USER_PASSPHRASE:
			return 'USER_PASSPHRASE';
		default:
			return 'NONE';
	}
}

export function encryptionFormToProto(form: EncryptionFormState) {
	return create(EncryptionAuthoringParamsSchema, {
		presharedKey: form.presharedKey || undefined,
		rotationIntervalDays: form.rotationIntervalDays,
		minWords: form.minWords,
		deviceBoundKeyType: encryptionDeviceBoundKeyToEnum(form.deviceBoundKeyType),
		userPassphraseMinLength: form.deviceBoundKeyType === 'USER_PASSPHRASE' ? form.userPassphraseMinLength : 0,
		userPassphraseComplexity: form.deviceBoundKeyType === 'USER_PASSPHRASE' ? lpsComplexityToEnum(form.userPassphraseComplexity) : 0
	});
}

function wifiAuthTypeToEnum(type: string): WifiAuthType {
	switch (type) {
		case 'PSK':
			return WifiAuthType.PSK;
		case 'EAP_TLS':
			return WifiAuthType.EAP_TLS;
		default:
			return WifiAuthType.UNSPECIFIED;
	}
}

function wifiAuthTypeFromEnum(type: WifiAuthType): string {
	switch (type) {
		case WifiAuthType.PSK:
			return 'PSK';
		case WifiAuthType.EAP_TLS:
			return 'EAP_TLS';
		default:
			return 'PSK';
	}
}

export function wifiFormToProto(form: WifiFormState) {
	return create(WifiAuthoringParamsSchema, {
		ssid: form.ssid,
		authType: wifiAuthTypeToEnum(form.authType),
		psk: form.authType === 'PSK' && form.psk ? form.psk : undefined,
		caCert: form.authType === 'EAP_TLS' ? form.caCert : undefined,
		clientCert: form.authType === 'EAP_TLS' ? form.clientCert : undefined,
		clientKey: form.authType === 'EAP_TLS' && form.clientKey ? form.clientKey : undefined,
		identity: form.authType === 'EAP_TLS' ? form.identity : undefined,
		autoConnect: form.autoConnect,
		hidden: form.hidden,
		priority: form.priority
	});
}

export function agentUpdateFormToProto(form: AgentUpdateFormState) {
	return create(AgentUpdateParamsSchema, {
		amd64: form.amd64BinaryUrl ? create(AgentUpdateArchSchema, {
			binaryUrl: form.amd64BinaryUrl,
			checksumUrl: form.amd64ChecksumUrl
		}) : undefined,
		arm64: form.arm64BinaryUrl ? create(AgentUpdateArchSchema, {
			binaryUrl: form.arm64BinaryUrl,
			checksumUrl: form.arm64ChecksumUrl
		}) : undefined,
		allowRedirect: form.allowRedirect
	});
}

export function packageProtoToForm(proto: PackageParams): PackageFormState {
	return {
		name: proto.name,
		version: proto.version || '',
		allowDowngrade: proto.allowDowngrade || false,
		pin: proto.pin || false,
		aptName: proto.aptName || '',
		dnfName: proto.dnfName || '',
		pacmanName: proto.pacmanName || '',
		zypperName: proto.zypperName || ''
	};
}

export function shellProtoToForm(proto: ShellParams): ShellFormState {
	return {
		script: proto.script,
		interpreter: proto.interpreter || '/bin/bash',
		runAsRoot: proto.runAsRoot || false,
		detectionScript: proto.detectionScript || '',
		isCompliance: proto.isCompliance || false
	};
}

export function serviceProtoToForm(proto: ServiceParams): ServiceFormState {
	return {
		unitName: proto.unitName,
		desiredState:
			proto.desiredState === 2 ? 'STOPPED' : proto.desiredState === 3 ? 'RESTARTED' : 'RUNNING',
		enable: proto.enable ?? true
	};
}

export function fileProtoToForm(proto: FileParams): FileFormState {
	return {
		path: proto.path,
		content: proto.content || '',
		owner: proto.owner || '',
		group: proto.group || '',
		mode: proto.mode || '0644',
		managedBlock: proto.managedBlock || false
	};
}

export function appProtoToForm(proto: AppInstallParams): AppFormState {
	return {
		url: proto.url,
		checksumSha256: proto.checksumSha256 || '',
		installPath: proto.installPath || ''
	};
}

export function flatpakProtoToForm(proto: FlatpakParams): FlatpakFormState {
	return {
		appId: proto.appId?.value ?? '',
		remote: proto.remote || '',
		systemWide: proto.systemWide ?? true,
		pin: proto.pin || false
	};
}

export function updateProtoToForm(proto: UpdateParams): UpdateFormState {
	return {
		securityOnly: proto.securityOnly || false,
		autoremove: proto.autoremove || false,
		rebootIfRequired: proto.rebootIfRequired || false
	};
}

export function directoryProtoToForm(proto: DirectoryParams): DirectoryFormState {
	return {
		path: proto.path,
		owner: proto.owner || '',
		group: proto.group || '',
		mode: proto.mode || '0755',
		recursive: proto.recursive ?? true
	};
}

export function userProtoToForm(proto: UserParams): UserFormState {
	return {
		username: proto.username,
		uid: proto.uid || 0,
		gid: proto.gid || 0,
		homeDir: proto.homeDir || '',
		shell: proto.shell || '',
		sshAuthorizedKeys: [...(proto.sshAuthorizedKeys ?? [])],
		comment: proto.comment || '',
		systemUser: proto.systemUser || false,
		createHome: proto.createHome ?? true,
		disabled: proto.disabled || false,
		primaryGroup: proto.primaryGroup || '',
		hidden: proto.hidden || false
	};
}

export function groupProtoToForm(proto: GroupParams): GroupFormState {
	return {
		name: proto.name,
		members: [...(proto.members ?? [])],
		gid: proto.gid || 0,
		systemGroup: proto.systemGroup || false
	};
}

export function repositoryProtoToForm(proto: RepositoryParams): RepositoryFormState {
	return {
		name: proto.name,
		apt: proto.apt
			? {
					url: proto.apt.url || '',
					distribution: proto.apt.distribution || '',
					components: proto.apt.components?.join(' ') || '',
					gpgKeyUrl: proto.apt.gpgKeyUrl || '',
					gpgKey: proto.apt.gpgKey || '',
					trusted: proto.apt.trusted || false,
					arch: proto.apt.arch || '',
					disabled: proto.apt.disabled || false
				}
			: defaultAptForm(),
		dnf: proto.dnf
			? {
					baseurl: proto.dnf.baseurl || '',
					description: proto.dnf.description || '',
					enabled: proto.dnf.enabled ?? true,
					gpgcheck: proto.dnf.gpgcheck ?? true,
					gpgkey: proto.dnf.gpgkey || '',
					moduleHotfixes: proto.dnf.moduleHotfixes || false,
					disabled: proto.dnf.disabled || false
				}
			: defaultDnfForm(),
		pacman: proto.pacman
			? {
					server: proto.pacman.server || '',
					sigLevel: proto.pacman.sigLevel || '',
					disabled: proto.pacman.disabled || false
				}
			: defaultPacmanForm(),
		zypper: proto.zypper
			? {
					url: proto.zypper.url || '',
					description: proto.zypper.description || '',
					enabled: proto.zypper.enabled ?? true,
					autorefresh: proto.zypper.autorefresh ?? true,
					gpgcheck: proto.zypper.gpgcheck ?? true,
					gpgkey: proto.zypper.gpgkey || '',
					type: zypperTypeFromEnum(proto.zypper.type),
					disabled: proto.zypper.disabled || false
				}
			: defaultZypperForm()
	};
}

export function sshProtoToForm(proto: SshParams): SshFormState {
	return {
		users: [...(proto.users ?? [])],
		allowPubkey: proto.allowPubkey ?? true,
		allowPassword: proto.allowPassword || false
	};
}

export function sshdProtoToForm(proto: SshdParams): SshdFormState {
	return {
		directives: proto.directives.map((d) => ({ key: d.key, value: d.value }))
	};
}

export function adminPolicyProtoToForm(proto: AdminPolicyParams): AdminPolicyFormState {
	return {
		accessLevel: adminAccessLevelFromEnum(proto.accessLevel),
		users: [...(proto.users ?? [])],
		customConfig: proto.customConfig || '',
		backend: privilegeBackendFromEnum(proto.backend)
	};
}

export function lpsProtoToForm(proto: LpsParams): LpsFormState {
	return {
		usernames: [...(proto.usernames ?? [])],
		passwordLength: proto.passwordLength || 24,
		complexity: lpsComplexityFromEnum(proto.complexity),
		rotationIntervalDays: proto.rotationIntervalDays || 30,
		gracePeriodHours: proto.gracePeriodHours ?? 24
	};
}

export function encryptionProtoToForm(
	proto: ManagedEncryptionParams | EncryptionAuthoringParams
): EncryptionFormState {
	const authoredKey = 'presharedKey' in proto ? (proto.presharedKey ?? '') : '';
	return {
		presharedKey: authoredKey,
		presharedKeyConfigured:
			'presharedKeyConfigured' in proto ? proto.presharedKeyConfigured : authoredKey !== '',
		rotationIntervalDays: proto.rotationIntervalDays || 30,
		minWords: proto.minWords || 5,
		deviceBoundKeyType: encryptionDeviceBoundKeyFromEnum(proto.deviceBoundKeyType),
		userPassphraseMinLength: proto.userPassphraseMinLength || 16,
		userPassphraseComplexity: lpsComplexityFromEnum(proto.userPassphraseComplexity)
	};
}

export function wifiProtoToForm(proto: ManagedWifiParams | WifiAuthoringParams): WifiFormState {
	const authoredPsk = 'psk' in proto ? (proto.psk ?? '') : '';
	const authoredClientKey = 'clientKey' in proto ? (proto.clientKey ?? '') : '';
	return {
		ssid: proto.ssid || '',
		authType: wifiAuthTypeFromEnum(proto.authType),
		psk: authoredPsk,
		pskConfigured: 'pskConfigured' in proto ? proto.pskConfigured : authoredPsk !== '',
		caCert: proto.caCert || '',
		clientCert: proto.clientCert || '',
		clientKey: authoredClientKey,
		clientKeyConfigured:
			'clientKeyConfigured' in proto ? proto.clientKeyConfigured : authoredClientKey !== '',
		identity: proto.identity || '',
		autoConnect: proto.autoConnect ?? true,
		hidden: proto.hidden || false,
		priority: proto.priority || 0
	};
}

export function agentUpdateProtoToForm(proto: AgentUpdateParams): AgentUpdateFormState {
	return {
		amd64BinaryUrl: proto.amd64?.binaryUrl || '',
		amd64ChecksumUrl: proto.amd64?.checksumUrl || '',
		arm64BinaryUrl: proto.arm64?.binaryUrl || '',
		arm64ChecksumUrl: proto.arm64?.checksumUrl || '',
		allowRedirect: proto.allowRedirect || false
	};
}
