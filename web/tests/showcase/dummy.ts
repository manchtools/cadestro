

export const REFERENCE_NOW_MS = Date.UTC(2026, 5, 20, 12, 0, 0);
const now = Math.floor(REFERENCE_NOW_MS / 1000);
const min = 60;
const hour = 60 * min;
const day = 24 * hour;

export function wrappedID(value: string): { value: string } {
	return { value };
}

export const DUMMY_DEVICES = [
	{
		id: '01J6XYZSHOWCASEDEVICE0001',
		hostname: 'edge-01.berlin',
		os_name: 'Fedora Linux',
		os_version: '40 (Workstation Edition)',
		os_arch: 'x86_64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 12,
		registered_at: now - 90 * day,
		compliance_status: 2,
		labels: 'env=production,role=edge',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0002',
		hostname: 'edge-02.berlin',
		os_name: 'Fedora Linux',
		os_version: '40 (Workstation Edition)',
		os_arch: 'x86_64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 28,
		registered_at: now - 90 * day,
		compliance_status: 2,
		labels: 'env=production,role=edge',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0003',
		hostname: 'kiosk-warehouse-04',
		os_name: 'Ubuntu',
		os_version: '24.04.2 LTS',
		os_arch: 'aarch64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 4,
		registered_at: now - 38 * day,
		compliance_status: 1,
		labels: 'env=production,role=kiosk',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0004',
		hostname: 'lab-debian-09',
		os_name: 'Debian GNU/Linux',
		os_version: '12 (bookworm)',
		os_arch: 'x86_64',
		agent_version: 'v2026.06.2',
		last_seen_at: now - 3 * hour,
		registered_at: now - 210 * day,
		compliance_status: 2,
		labels: 'env=lab,role=workstation',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0005',
		hostname: 'ci-runner-arch-01',
		os_name: 'Arch Linux',
		os_version: 'rolling',
		os_arch: 'x86_64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 38,
		registered_at: now - 12 * day,
		compliance_status: 2,
		labels: 'env=ci,role=runner',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0006',
		hostname: 'office-laptop-22',
		os_name: 'openSUSE Tumbleweed',
		os_version: '20260601',
		os_arch: 'x86_64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 47,
		registered_at: now - 5 * day,
		compliance_status: 3,
		labels: 'env=corporate,role=workstation',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0007',
		hostname: 'edge-03.frankfurt',
		os_name: 'Fedora Linux',
		os_version: '40 (Workstation Edition)',
		os_arch: 'x86_64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 21,
		registered_at: now - 60 * day,
		compliance_status: 2,
		labels: 'env=production,role=edge',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0008',
		hostname: 'kiosk-warehouse-09',
		os_name: 'Ubuntu',
		os_version: '24.04.2 LTS',
		os_arch: 'aarch64',
		agent_version: 'v2026.06.2',
		last_seen_at: now - 9 * day,
		registered_at: now - 40 * day,
		compliance_status: 1,
		labels: 'env=production,role=kiosk',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0009',
		hostname: 'meeting-room-fedora-12',
		os_name: 'Fedora Linux',
		os_version: '41',
		os_arch: 'x86_64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 6,
		registered_at: now - 18 * day,
		compliance_status: 2,
		labels: 'env=corporate,role=conference-room',
	},
	{
		id: '01J6XYZSHOWCASEDEVICE0010',
		hostname: 'lab-rhel-04',
		os_name: 'Red Hat Enterprise Linux',
		os_version: '9.4 (Plow)',
		os_arch: 'x86_64',
		agent_version: 'v2026.07.0',
		last_seen_at: now - 1 * hour,
		registered_at: now - 540 * day,
		compliance_status: 2,
		labels: 'env=lab,role=server',
	},
];

export function devicesAsSearchResults() {
	return {
		results: DUMMY_DEVICES.map((d) => ({
			id: wrappedID(d.id),
			name: d.hostname,
			description: '',
			scope: 5,
			memberCount: 0,
			fields: {
				hostname: d.hostname,
				agent_version: d.agent_version,
				last_seen_at: String(d.last_seen_at),
				registered_at: String(d.registered_at),
				compliance_status: String(d.compliance_status),
				os_name: d.os_name,
				os_version: d.os_version,
				os_arch: d.os_arch,
				labels: d.labels,
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_DEVICES.length,
	};
}

function isoFromSec(sec: number): string {
	return new Date(sec * 1000).toISOString();
}

export function getDeviceByIdResponse(id: string) {
	const d = DUMMY_DEVICES.find((x) => x.id === id) ?? DUMMY_DEVICES[0];
	const labelsObj: Record<string, string> = {};
	for (const pair of d.labels.split(',')) {
		const [k, v] = pair.split('=');
		if (k) labelsObj[k] = v ?? '';
	}

	return {
		device: {
			id: wrappedID(d.id),
			hostname: d.hostname,
			agentVersion: d.agent_version,
			status: now - d.last_seen_at < 5 * 60 ? 'DEVICE_STATUS_ONLINE' : 'DEVICE_STATUS_OFFLINE',
			registeredAt: isoFromSec(d.registered_at),
			lastSeenAt: isoFromSec(d.last_seen_at),
			certExpiresAt: isoFromSec(now + 200 * day),
			labels: labelsObj,
			assignedUserIds: [wrappedID('01J6XYZSHOWCASEADMINUSR01')],
			assignedGroupIds: [wrappedID('01J6XYZSHOWCASEGROUP0001')],
		},
	};
}

export function getDeviceInventoryResponse() {
	return {
		tables: [
			{
				tableName: 'os_version',
				rows: [
					{
						columns: {
							name: 'Fedora Linux',
							version: '40 (Workstation Edition)',
							platform: 'fedora',
							platform_like: 'rhel',
							arch: 'x86_64',
						},
					},
				],
				collectedAt: isoFromSec(now - 60),
			},
			{
				tableName: 'system_info',
				rows: [
					{
						columns: {
							hostname: 'edge-01.berlin',
							cpu_brand: 'Intel(R) Xeon(R) E-2356G',
							cpu_logical_cores: '12',
							physical_memory: '34359738368',
							uuid: 'B6F4F3A0-7C1E-4A12-9D5C-7B9F22A1A7E1',
						},
					},
				],
				collectedAt: isoFromSec(now - 60),
			},
		],
	};
}

export const DUMMY_ACTIONS = [
	{
		id: '01J6XYZSHOWCASEACTION001',
		name: 'Install nginx',
		description: 'Add the upstream NGINX repo and install the package',
		type: 1,
		desired_state: 1,
		created_at: now - 60 * day,
		updated_at: now - 12 * day,
		is_compliance: 'false',
	},
	{
		id: '01J6XYZSHOWCASEACTION002',
		name: 'Apply security updates',
		description: 'Run unattended-upgrades, security pocket only',
		type: 2,
		desired_state: 1,
		created_at: now - 30 * day,
		updated_at: now - 2 * day,
		is_compliance: 'true',
	},
	{
		id: '01J6XYZSHOWCASEACTION003',
		name: 'Disable telemetry service',
		description: 'systemctl disable + mask vendor-telemetry.service',
		type: 700,
		desired_state: 2,
		created_at: now - 90 * day,
		updated_at: now - 30 * day,
		is_compliance: 'true',
	},
	{
		id: '01J6XYZSHOWCASEACTION004',
		name: 'Flatpak: Firefox ESR',
		description: 'org.mozilla.firefox from flathub, system-wide',
		type: 103,
		desired_state: 1,
		created_at: now - 14 * day,
		updated_at: now - 14 * day,
		is_compliance: 'false',
	},
	{
		id: '01J6XYZSHOWCASEACTION005',
		name: 'Deploy /etc/issue.net banner',
		description: 'Compliance banner per group security policy',
		type: 400,
		desired_state: 1,
		created_at: now - 200 * day,
		updated_at: now - 5 * day,
		is_compliance: 'true',
	},
	{
		id: '01J6XYZSHOWCASEACTION006',
		name: 'Rotate LUKS device key',
		description: 'Rotate slot-0 key on encrypted root',
		type: 800,
		desired_state: 1,
		created_at: now - 7 * day,
		updated_at: now - 7 * day,
		is_compliance: 'false',
	},
	{
		id: '01J6XYZSHOWCASEACTION007',
		name: 'Provision lab user "intern"',
		description: 'Local account in operators group, no password',
		type: 600,
		desired_state: 1,
		created_at: now - 18 * day,
		updated_at: now - 18 * day,
		is_compliance: 'false',
	},
	{
		id: '01J6XYZSHOWCASEACTION008',
		name: 'Run kernel-lockdown audit',
		description: 'osquery compliance check for kernel.lockdown',
		type: 200,
		desired_state: 1,
		created_at: now - 45 * day,
		updated_at: now - 1 * day,
		is_compliance: 'true',
	},
];

export function actionsAsSearchResults() {
	return {
		results: DUMMY_ACTIONS.map((a) => ({
			id: wrappedID(a.id),
			name: a.name,
			description: a.description,
			scope: 1,
			memberCount: 0,
			fields: {
				name: a.name,
				description: a.description,
				type: String(a.type),
				desired_state: String(a.desired_state),
				created_at: String(a.created_at),
				updated_at: String(a.updated_at),
				is_compliance: a.is_compliance,
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_ACTIONS.length,
	};
}

export const DUMMY_USERS = [
	{
		id: '01J6XYZSHOWCASEADMINUSR01',
		email: 'admin@cadestro.example',
		display_name: 'Sam Reiter',
		created_at: now - 320 * day,
		last_login_at: now - 1 * hour,
		disabled: 'false',
		roles: 'Administrator',
	},
	{
		id: '01J6XYZSHOWCASEUSER0002',
		email: 'lina.hartmann@cadestro.example',
		display_name: 'Lina Hartmann',
		created_at: now - 180 * day,
		last_login_at: now - 6 * hour,
		disabled: 'false',
		roles: 'Fleet Operator',
	},
	{
		id: '01J6XYZSHOWCASEUSER0003',
		email: 'kai.bauer@cadestro.example',
		display_name: 'Kai Bauer',
		created_at: now - 90 * day,
		last_login_at: now - 2 * day,
		disabled: 'false',
		roles: 'Compliance Reviewer',
	},
	{
		id: '01J6XYZSHOWCASEUSER0004',
		email: 'sven.koch@cadestro.example',
		display_name: 'Sven Koch',
		created_at: now - 540 * day,
		last_login_at: now - 12 * day,
		disabled: 'false',
		roles: 'Fleet Operator',
	},
	{
		id: '01J6XYZSHOWCASEUSER0005',
		email: 'rebecca.weber@cadestro.example',
		display_name: 'Rebecca Weber',
		created_at: now - 60 * day,
		last_login_at: now - 38 * day,
		disabled: 'true',
		roles: 'Read-only Auditor',
	},
];

export function usersAsSearchResults() {
	return {
		results: DUMMY_USERS.map((u) => ({
			id: wrappedID(u.id),
			name: u.display_name,
			description: u.email,
			scope: 6,
			memberCount: 0,
			fields: {
				email: u.email,
				display_name: u.display_name,
				created_at: String(u.created_at),
				last_login_at: String(u.last_login_at),
				disabled: u.disabled,
				role: u.roles,
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_USERS.length,
	};
}

export const ALL_PERMISSIONS = [
	'GetCurrentUser','GetUser','GetUser:self','ListUsers',
	'EraseJITUser','UpdateUserEmail','UpdateUserEmail:self','SetUserDisabled',
	'UpdateUserProfile','UpdateUserProfile:self','UpdateUserSshSettings','UpdateUserSshSettings:self',
	'UpdateUserLinuxUsername','AddUserSshKey','AddUserSshKey:self','RemoveUserSshKey',
	'RemoveUserSshKey:self','ListDevices','ListDevices:assigned','GetDevice',
	'GetDevice:assigned','SetDeviceLabel','RemoveDeviceLabel','AssignDevice',
	'UnassignDevice','ListDeviceAssignees','SetDeviceSyncInterval','SetDeviceInventoryInterval',
	'DeleteDevice','CreateToken','ListTokens',
	'RenameToken','SetTokenDisabled','DeleteToken','CreateAction',
	'GetAction','ListActions','RenameAction','UpdateActionDescription',
	'UpdateActionParams','DeleteAction','CreateActionSet','GetActionSet',
	'ListActionSets','RenameActionSet','UpdateActionSetDescription','UpdateActionSetSchedule',
	'DeleteActionSet','AddActionToSet','RemoveActionFromSet','ReorderActionInSet',
	'CreateDefinition','GetDefinition','ListDefinitions','RenameDefinition',
	'UpdateDefinitionDescription','UpdateDefinitionSchedule','DeleteDefinition','AddActionSetToDefinition',
	'RemoveActionSetFromDefinition','ReorderActionSetInDefinition','CreateStaticDeviceGroup','CreateDynamicDeviceGroup',
	'GetDeviceGroup','ListDeviceGroups','ListDeviceGroupsForDevice','RenameDeviceGroup',
	'UpdateDeviceGroupDescription','UpdateDynamicDeviceGroupQuery','DeleteDeviceGroup','AddDeviceToGroup',
	'RemoveDeviceFromGroup','ValidateDynamicQuery','EvaluateDynamicGroup','SetDeviceGroupSyncInterval',
	'SetDeviceGroupInventoryInterval','SetDeviceGroupMaintenanceWindow','CreateAssignment','DeleteAssignment',
	'ListAssignments','GetDeviceAssignments','GetUserAssignments','SetUserSelection',
	'ListAvailableActions','RebootDevice','SyncDevice','DispatchOSQuery',
	'GetOSQueryResult','GetDeviceInventory','RefreshDeviceInventory','QueryDeviceLogs',
	'GetDeviceLogResult','GetDeviceCompliance','GetDeviceCompliance:assigned','CreateCompliancePolicy',
	'GetCompliancePolicy','ListCompliancePolicies','RenameCompliancePolicy','UpdateCompliancePolicyDescription',
	'DeleteCompliancePolicy','AddCompliancePolicyRule','RemoveCompliancePolicyRule','UpdateCompliancePolicyRule',
	'GetDeviceCompliancePolicyStatus','GetDeviceCompliancePolicyStatus:assigned','ListAuditEvents','ListLpsPasswords',
	'RevealLpsPassword','ListLuksKeys','RevealLuksKey','CreateLuksToken',
	'RevokeLuksDeviceKey','CreateRole','GetRole','ListRoles',
	'UpdateRole','DeleteRole','AssignRoleToUser','RevokeRoleFromUser',
	'AssignRoleScope','ListPermissions','CreateStaticUserGroup','CreateDynamicUserGroup',
	'GetUserGroup','ListUserGroups','UpdateUserGroup','DeleteUserGroup',
	'AddUserToGroup','RemoveUserFromGroup','AssignRoleToUserGroup','RevokeRoleFromUserGroup',
	'ListUserGroupsForUser','UpdateDynamicUserGroupQuery','ValidateUserGroupQuery','EvaluateDynamicUserGroup',
	'SetUserGroupMaintenanceWindow','CreateIdentityProvider','GetIdentityProvider','ListIdentityProviders',
	'UpdateIdentityProvider','DeleteIdentityProvider','EnableSCIM','DisableSCIM',
	'RotateSCIMToken','ListIdentityLinks','UnlinkIdentity','Search',
	'RebuildSearchIndex','GetServerSettings','UpdateServerSettings','SetUserProvisioningEnabled',
	'StartTerminal','StopTerminal','ListActiveTerminalSessions','TerminateTerminalSession',
	'TerminalAdminLimited','TerminalAdminFull',
];

export function listRolesResponse() {
	return {
		roles: [
			{
				id: wrappedID('00000000000000000000000001'),
				name: 'Administrator',
				description: 'Full platform control',
				permissions: ALL_PERMISSIONS,
				isSystem: true,
			},
			{
				id: wrappedID('01J6XYZSHOWCASEROLE0002'),
				name: 'Fleet Operator',
				description: 'Run actions, assign actions, view audit',
				permissions: [
					'ListDevices', 'AssignDevice', 'ListActions', 'CreateAction',
					'ListActionSets', 'CreateActionSet',
					'ListDeviceGroups', 'CreateDeviceGroup', 'Search',
				],
				isSystem: false,
			},
			{
				id: wrappedID('01J6XYZSHOWCASEROLE0003'),
				name: 'Compliance Reviewer',
				description: 'Read-only access to audit + compliance policies',
				permissions: [
					'ListDevices', 'ListCompliancePolicies',
					'CreateCompliancePolicy', 'ViewAudit', 'Search',
				],
				isSystem: false,
			},
			{
				id: wrappedID('01J6XYZSHOWCASEROLE0004'),
				name: 'Read-only Auditor',
				description: 'View-only access; surfaces nothing destructive',
				permissions: ['ListDevices', 'ListActions', 'ViewAudit', 'Search'],
				isSystem: false,
			},
		],
		nextPageToken: '',
		totalCount: 4,
	};
}

export const DUMMY_USER_GROUPS = [
	{
		id: '01J6XYZSHOWCASEGROUP0001',
		name: 'Berlin Operators',
		description: 'Fleet operators with edge access in the Berlin region',
		kind: 'static',
		member_count: '4',
		role_count: '1',
		created_at: now - 200 * day,
	},
	{
		id: '01J6XYZSHOWCASEGROUP0002',
		name: 'Compliance team',
		description: 'Auditors with read-only fleet access',
		kind: 'static',
		member_count: '2',
		role_count: '1',
		created_at: now - 110 * day,
	},
	{
		id: '01J6XYZSHOWCASEGROUP0003',
		name: 'Auto: active operators',
		description: 'Dynamic: every enabled operator account',
		kind: 'dynamic',
		member_count: '3',
		role_count: '2',
		created_at: now - 14 * day,
	},
];

export const DUMMY_DEVICE_GROUPS = [
	{
		id: '01J6XYZSHOWCASEDEVGRP0001',
		name: 'Berlin Edge Nodes',
		description: 'All production edge devices in the Berlin region',
		is_dynamic: true,
		dynamic_query: '(device.labels.env equals "production") and (device.labels.role equals "edge") and (device.hostname matches "berlin")',
		member_count: 6,
		sync_interval_minutes: 15,
		created_at: now - 80 * day,
		created_by: 'sam.reiter',
	},
	{
		id: '01J6XYZSHOWCASEDEVGRP0002',
		name: 'Workstations',
		description: 'Office laptops + meeting-room machines (corporate env)',
		is_dynamic: true,
		dynamic_query: '(device.labels.env equals "corporate") and (device.labels.role in ["workstation","conference-room"])',
		member_count: 22,
		sync_interval_minutes: 60,
		created_at: now - 220 * day,
		created_by: 'sam.reiter',
	},
	{
		id: '01J6XYZSHOWCASEDEVGRP0003',
		name: 'Lab fleet',
		description: 'Static membership: the lab-* hosts in DE-FRA-1',
		is_dynamic: false,
		dynamic_query: '',
		member_count: 8,
		sync_interval_minutes: 0,
		created_at: now - 420 * day,
		created_by: 'lina.hartmann',
	},
];

export function deviceGroupsAsSearchResults() {
	return {
		results: DUMMY_DEVICE_GROUPS.map((g) => ({
			id: wrappedID(g.id),
			name: g.name,
			description: g.description,
			scope: 7,
			memberCount: g.member_count,
			fields: {
				name: g.name,
				description: g.description,
				is_dynamic: String(g.is_dynamic),
				member_count: String(g.member_count),
				sync_interval_minutes: String(g.sync_interval_minutes),
				created_at: String(g.created_at),
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_DEVICE_GROUPS.length,
	};
}

export function getDeviceGroupResponse(id: string) {
	const g = DUMMY_DEVICE_GROUPS.find((x) => x.id === id) ?? DUMMY_DEVICE_GROUPS[0];
	return {
		group: {
			id: wrappedID(g.id),
			name: g.name,
			description: g.description,
			memberCount: g.member_count,
			createdAt: isoFromSec(g.created_at),
			createdBy: g.created_by,
			isDynamic: g.is_dynamic,
			dynamicQuery: g.dynamic_query,
			syncIntervalMinutes: g.sync_interval_minutes,
		},
	};
}

export function listDevicesResponse() {
	const subset = DUMMY_DEVICES.slice(0, 6);
	return {
		devices: subset.map((d) => ({
			id: wrappedID(d.id),
			hostname: d.hostname,
			agentVersion: d.agent_version,
			status: now - d.last_seen_at < 5 * 60 ? 'DEVICE_STATUS_ONLINE' : 'DEVICE_STATUS_OFFLINE',
			registeredAt: isoFromSec(d.registered_at),
			lastSeenAt: isoFromSec(d.last_seen_at),
		})),
		nextPageToken: '',
		totalCount: subset.length,
	};
}

export function getDeviceCompliancePolicyStatusResponse() {
	return {
		overallStatus: 'COMPLIANCE_STATUS_NON_COMPLIANT',
		policies: [
			{
				policyId: wrappedID('01J6XYZSHOWCASEPOLICY001'),
				policyName: 'CIS Linux Baseline',
				status: 'COMPLIANCE_STATUS_COMPLIANT',
				rules: [
					{
						actionId: wrappedID('01J6XYZSHOWCASECHECK0001'),
						actionName: 'cis-1.1.1: Disable unused filesystems',
						status: 'COMPLIANCE_STATUS_COMPLIANT',
						compliant: true,
						checkedAt: isoFromSec(now - 12 * hour),
						gracePeriodHours: 0,
					},
					{
						actionId: wrappedID('01J6XYZSHOWCASECHECK0002'),
						actionName: 'cis-1.4.1: Bootloader password set',
						status: 'COMPLIANCE_STATUS_COMPLIANT',
						compliant: true,
						checkedAt: isoFromSec(now - 12 * hour),
						gracePeriodHours: 0,
					},
					{
						actionId: wrappedID('01J6XYZSHOWCASECHECK0003'),
						actionName: 'cis-5.2.1: SSH protocol = 2',
						status: 'COMPLIANCE_STATUS_COMPLIANT',
						compliant: true,
						checkedAt: isoFromSec(now - 12 * hour),
						gracePeriodHours: 0,
					},
				],
			},
			{
				policyId: wrappedID('01J6XYZSHOWCASEPOLICY002'),
				policyName: 'Endpoint hardening — identity & disk encryption',
				status: 'COMPLIANCE_STATUS_NON_COMPLIANT',
				rules: [
					{
						actionId: wrappedID('01J6XYZSHOWCASECHECK0010'),
						actionName: 'Operator accounts are centrally managed',
						status: 'COMPLIANCE_STATUS_COMPLIANT',
						compliant: true,
						checkedAt: isoFromSec(now - 2 * hour),
						gracePeriodHours: 0,
					},
					{
						actionId: wrappedID('01J6XYZSHOWCASECHECK0011'),
						actionName: 'Root filesystem encrypted with LUKS',
						status: 'COMPLIANCE_STATUS_COMPLIANT',
						compliant: true,
						checkedAt: isoFromSec(now - 2 * hour),
						gracePeriodHours: 0,
					},
					{
						actionId: wrappedID('01J6XYZSHOWCASECHECK0012'),
						actionName: 'Idle screen lock <= 5 minutes',
						status: 'COMPLIANCE_STATUS_NON_COMPLIANT',
						compliant: false,
						checkedAt: isoFromSec(now - 2 * hour),
						firstFailedAt: isoFromSec(now - 36 * hour),
						gracePeriodHours: 24,
						graceExpiresAt: isoFromSec(now - 12 * hour),
					},
				],
			},
		],
	};
}

export function userGroupsAsSearchResults() {
	return {
		results: DUMMY_USER_GROUPS.map((g) => ({
			id: wrappedID(g.id),
			name: g.name,
			description: g.description,
			scope: 8,
			memberCount: parseInt(g.member_count, 10),
			fields: {
				name: g.name,
				description: g.description,
				kind: g.kind,
				member_count: g.member_count,
				role_count: g.role_count,
				created_at: String(g.created_at),
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_USER_GROUPS.length,
	};
}

export const DUMMY_ACTION_SETS = [
	{
		id: '01J6XYZSHOWCASESET00001',
		name: 'Baseline hardening',
		description: 'Banner, telemetry-off, security updates — applied fleet-wide',
		member_count: 3,
		created_at: now - 120 * day,
		updated_at: now - 9 * day,
	},
	{
		id: '01J6XYZSHOWCASESET00002',
		name: 'Workstation apps',
		description: 'Firefox ESR + the standard flatpak bundle',
		member_count: 2,
		created_at: now - 40 * day,
		updated_at: now - 3 * day,
	},
	{
		id: '01J6XYZSHOWCASESET00003',
		name: 'Empty set (no actions yet)',
		description: 'Placeholder set kept to exercise the "no actions" filter',
		member_count: 0,
		created_at: now - 5 * day,
		updated_at: now - 5 * day,
	},
];

export function actionSetsAsSearchResults() {
	return {
		results: DUMMY_ACTION_SETS.map((s) => ({
			id: wrappedID(s.id),
			name: s.name,
			description: s.description,
			scope: 2,
			memberCount: s.member_count,
			fields: {
				name: s.name,
				description: s.description,
				member_count: String(s.member_count),
				created_at: String(s.created_at),
				updated_at: String(s.updated_at),
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_ACTION_SETS.length,
	};
}

export function getActionSetResponse(id: string) {
	const s = DUMMY_ACTION_SETS.find((x) => x.id === id) ?? DUMMY_ACTION_SETS[0];
	const members = DUMMY_ACTIONS.slice(0, s.member_count).map((a, i) => ({
		actionId: wrappedID(a.id),
		sortOrder: i,
		actionName: a.name,
		actionType: a.type,
	}));
	return {
		set: {
			id: wrappedID(s.id),
			name: s.name,
			description: s.description,
			memberCount: s.member_count,
			createdAt: isoFromSec(s.created_at),
			createdBy: 'sam.reiter',
			updatedAt: isoFromSec(s.updated_at),
		},
		members,
	};
}

export function listActionSetsResponse() {
	return {
		sets: DUMMY_ACTION_SETS.map((s) => ({
			id: wrappedID(s.id),
			name: s.name,
			description: s.description,
			memberCount: s.member_count,
			createdAt: isoFromSec(s.created_at),
			updatedAt: isoFromSec(s.updated_at),
		})),
		nextPageToken: '',
		totalCount: DUMMY_ACTION_SETS.length,
	};
}

export const DUMMY_DEFINITIONS = [
	{
		id: '01J6XYZSHOWCASEDEF000001',
		name: 'Nightly maintenance',
		description: 'Baseline hardening + workstation apps, scheduled 02:00',
		member_count: 2,
		created_at: now - 95 * day,
		updated_at: now - 6 * day,
	},
	{
		id: '01J6XYZSHOWCASEDEF000002',
		name: 'Onboarding bootstrap',
		description: 'Everything a freshly-enrolled edge node needs',
		member_count: 1,
		created_at: now - 30 * day,
		updated_at: now - 30 * day,
	},
];

export function definitionsAsSearchResults() {
	return {
		results: DUMMY_DEFINITIONS.map((d) => ({
			id: wrappedID(d.id),
			name: d.name,
			description: d.description,
			scope: 3,
			memberCount: d.member_count,
			fields: {
				name: d.name,
				description: d.description,
				member_count: String(d.member_count),
				created_at: String(d.created_at),
				updated_at: String(d.updated_at),
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_DEFINITIONS.length,
	};
}

export function getDefinitionResponse(id: string) {
	const d = DUMMY_DEFINITIONS.find((x) => x.id === id) ?? DUMMY_DEFINITIONS[0];
	const members = DUMMY_ACTION_SETS.slice(0, d.member_count).map((s, i) => ({
		actionSetId: wrappedID(s.id),
		sortOrder: i,
		actionSetName: s.name,
	}));
	return {
		definition: {
			id: wrappedID(d.id),
			name: d.name,
			description: d.description,
			memberCount: d.member_count,
			createdAt: isoFromSec(d.created_at),
			createdBy: 'sam.reiter',
			updatedAt: isoFromSec(d.updated_at),
		},
		members,
	};
}

export const DUMMY_COMPLIANCE_POLICIES = [
	{
		id: '01J6XYZSHOWCASEPOLICY001',
		name: 'CIS Linux Baseline',
		description: 'Subset of CIS Distribution Independent Linux benchmark',
		rule_count: 3,
		created_at: now - 150 * day,
	},
	{
		id: '01J6XYZSHOWCASEPOLICY002',
		name: 'Endpoint hardening — identity & disk encryption',
		description: 'Central identity + LUKS root + idle screen lock',
		rule_count: 3,
		created_at: now - 60 * day,
	},
	{
		id: '01J6XYZSHOWCASEPOLICY003',
		name: 'Draft policy (no rules)',
		description: 'Exercises the "no rules" empty-relation filter',
		rule_count: 0,
		created_at: now - 2 * day,
	},
];

export function compliancePoliciesAsSearchResults() {
	return {
		results: DUMMY_COMPLIANCE_POLICIES.map((p) => ({
			id: wrappedID(p.id),
			name: p.name,
			description: p.description,
			scope: 4,
			memberCount: p.rule_count,
			fields: {
				name: p.name,
				description: p.description,
				rule_count: String(p.rule_count),
				created_at: String(p.created_at),
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_COMPLIANCE_POLICIES.length,
	};
}

export function getCompliancePolicyResponse(id: string) {
	const p = DUMMY_COMPLIANCE_POLICIES.find((x) => x.id === id) ?? DUMMY_COMPLIANCE_POLICIES[0];
	const rules = DUMMY_ACTIONS.filter((a) => a.is_compliance === 'true')
		.slice(0, p.rule_count)
		.map((a) => ({ actionId: wrappedID(a.id), actionName: a.name, gracePeriodHours: 24 }));
	return {
		policy: {
			id: wrappedID(p.id),
			name: p.name,
			description: p.description,
			rules,
			ruleCount: p.rule_count,
			createdAt: isoFromSec(p.created_at),
			createdBy: 'sam.reiter',
		},
	};
}

export const DUMMY_AUDIT_EVENTS = [
	{ id: '01J6XYZSHOWCASEAUDIT0001', stream_type: 'device', stream_id: '01J6XYZSHOWCASEDEVICE0001', event_type: 'DeviceAssigned', actor_type: 'user', actor_id: '01J6XYZSHOWCASEADMINUSR01', occurred_at: now - 1 * hour },
	{ id: '01J6XYZSHOWCASEAUDIT0002', stream_type: 'action', stream_id: '01J6XYZSHOWCASEACTION002', event_type: 'ActionCreated', actor_type: 'user', actor_id: '01J6XYZSHOWCASEUSER0002', occurred_at: now - 3 * hour },
	{ id: '01J6XYZSHOWCASEAUDIT0003', stream_type: 'user', stream_id: '01J6XYZSHOWCASEUSER0005', event_type: 'UserDisabled', actor_type: 'user', actor_id: '01J6XYZSHOWCASEADMINUSR01', occurred_at: now - 8 * hour },
	{ id: '01J6XYZSHOWCASEAUDIT0004', stream_type: 'role', stream_id: '01J6XYZSHOWCASEROLE0002', event_type: 'RoleAssignedToUser', actor_type: 'user', actor_id: '01J6XYZSHOWCASEADMINUSR01', occurred_at: now - 26 * hour },
	{ id: '01J6XYZSHOWCASEAUDIT0005', stream_type: 'token', stream_id: '01J6XYZSHOWCASETOKEN0001', event_type: 'TokenCreated', actor_type: 'user', actor_id: '01J6XYZSHOWCASEUSER0004', occurred_at: now - 2 * day },
	{ id: '01J6XYZSHOWCASEAUDIT0006', stream_type: 'compliance_policy', stream_id: '01J6XYZSHOWCASEPOLICY001', event_type: 'CompliancePolicyCreated', actor_type: 'user', actor_id: '01J6XYZSHOWCASEUSER0003', occurred_at: now - 5 * day },
];

export function auditEventsAsSearchResults() {
	return {
		results: DUMMY_AUDIT_EVENTS.map((e) => ({
			id: wrappedID(e.id),
			name: e.event_type,
			description: '',
			scope: 9,
			memberCount: 0,
			fields: {
				stream_type: e.stream_type,
				stream_id: e.stream_id,
				event_type: e.event_type,
				actor_type: e.actor_type,
				actor_id: e.actor_id,
				occurred_at: String(e.occurred_at),
			},
		})),
		nextPageToken: '',
		totalCount: DUMMY_AUDIT_EVENTS.length,
	};
}

export function getActionResponse(id: string) {
	const a = DUMMY_ACTIONS.find((x) => x.id === id) ?? DUMMY_ACTIONS[0];
	return {
		action: {
			id: wrappedID(a.id),
			name: a.name,
			description: a.description,
			type: a.type,
			desiredState: a.desired_state,
			timeoutSeconds: 300,
			createdAt: isoFromSec(a.created_at),
			updatedAt: isoFromSec(a.updated_at),
			createdBy: 'sam.reiter',
		},
	};
}

export function listActionsResponse() {
	return {
		actions: DUMMY_ACTIONS.map((a) => ({
			id: wrappedID(a.id),
			name: a.name,
			description: a.description,
			type: a.type,
			desiredState: a.desired_state,
			createdAt: isoFromSec(a.created_at),
			updatedAt: isoFromSec(a.updated_at),
		})),
		nextPageToken: '',
		totalCount: DUMMY_ACTIONS.length,
	};
}

function userProto(u: (typeof DUMMY_USERS)[number]) {
	return {
		id: wrappedID(u.id),
		email: u.email,
		displayName: u.display_name,
		givenName: u.display_name.split(' ')[0],
		familyName: u.display_name.split(' ').slice(1).join(' '),
		createdAt: isoFromSec(u.created_at),
		lastLoginAt: isoFromSec(u.last_login_at),
		disabled: u.disabled === 'true',
		linuxUsername: u.email.split('@')[0].replace('.', '-'),
		sshAccessEnabled: true,
		roleGrants: [{
			role: { id: wrappedID('01J6XYZSHOWCASEROLE0002'), name: u.roles, permissions: [], isSystem: false },
			scopeKind: 'ROLE_GRANT_SCOPE_KIND_UNSPECIFIED',
			scopeId: wrappedID(''),
			scopeName: ''
		}],
		identityLinks: [],
		inheritedRoles: [],
	};
}

export function getUserResponse(id: string) {
	const u = DUMMY_USERS.find((x) => x.id === id) ?? DUMMY_USERS[1];
	return { user: userProto(u) };
}

export function listUsersResponse() {
	return {
		users: DUMMY_USERS.map(userProto),
		nextPageToken: '',
		totalCount: DUMMY_USERS.length,
	};
}

export function getUserGroupResponse(id: string) {
	const g = DUMMY_USER_GROUPS.find((x) => x.id === id) ?? DUMMY_USER_GROUPS[0];
	const members = DUMMY_USERS.slice(0, parseInt(g.member_count, 10)).map((u) => ({
		userId: wrappedID(u.id),
		email: u.email,
		addedAt: isoFromSec(now - 30 * day),
	}));
	return {
		group: {
			id: wrappedID(g.id),
			name: g.name,
			description: g.description,
			memberCount: parseInt(g.member_count, 10),
			isDynamic: g.kind === 'dynamic',
			dynamicQuery: g.kind === 'dynamic' ? '(user.disabled equals "false")' : '',
			createdAt: isoFromSec(g.created_at),
			roleGrants: [{
				role: { id: wrappedID('01J6XYZSHOWCASEROLE0002'), name: 'Fleet Operator', permissions: [], isSystem: false },
				scopeKind: 'ROLE_GRANT_SCOPE_KIND_UNSPECIFIED',
				scopeId: wrappedID(''),
				scopeName: ''
			}],
		},
		members,
	};
}

export function getRoleResponse(id: string) {
	const r = listRolesResponse().roles.find((x) => x.id.value === id) ?? listRolesResponse().roles[1];
	return {
		role: {
			id: r.id,
			name: r.name,
			description: r.description,
			permissions: r.permissions,
			createdAt: isoFromSec(now - 200 * day),
			isSystem: r.isSystem === true,
		},
		userCount: 3,
	};
}

const PERMISSION_GROUPS: Array<[string, string[]]> = [
	['Devices', ['ListDevices', 'GetDevice', 'AssignDevice', 'UnassignDevice', 'DeleteDevice']],
	['Actions', ['ListActions', 'CreateAction', 'RenameAction', 'DeleteAction']],

	['Users', ['ListUsers', 'SetUserDisabled', 'UpdateUserProfile', 'EraseJITUser']],
	['Roles', ['ListRoles', 'CreateRole', 'UpdateRole', 'DeleteRole', 'AssignRoleToUser']],
	['Audit', ['ListAuditEvents']],
];

export function listPermissionsResponse() {
	const permissions: Array<{ key: string; group: string; description: string }> = [];
	for (const [group, keys] of PERMISSION_GROUPS) {
		for (const key of keys) {
			permissions.push({ key, group, description: `${group}: ${key}` });
		}
	}
	return { permissions };
}

export const DUMMY_TOKENS = [
	{ id: '01J6XYZSHOWCASETOKEN0001', name: 'Berlin rollout', max_uses: 0, current_uses: 2, expires_at: now + 60 * day, created_at: now - 40 * day, disabled: false },
	{ id: '01J6XYZSHOWCASETOKEN0002', name: 'Lab enrollment', max_uses: 1, current_uses: 1, expires_at: now + 7 * day, created_at: now - 2 * day, disabled: false },
	{ id: '01J6XYZSHOWCASETOKEN0003', name: 'Expired CI token', max_uses: 50, current_uses: 0, expires_at: now - 3 * day, created_at: now - 90 * day, disabled: true },
];

export function listTokensResponse() {
	return {
		tokens: DUMMY_TOKENS.map((t) => ({
			id: wrappedID(t.id),
			name: t.name,
			maxUses: t.max_uses,
			currentUses: t.current_uses,
			expiresAt: isoFromSec(t.expires_at),
			createdAt: isoFromSec(t.created_at),
			createdBy: 'sam.reiter',
			disabled: t.disabled,
		})),
		nextPageToken: '',
		totalCount: DUMMY_TOKENS.length,
	};
}

export const DUMMY_IDPS = [
	{ id: '01J6XYZSHOWCASEIDP00001', name: 'Okta (Staging)', slug: 'okta-staging', enabled: true, client_id: 'okta-client-staging', issuer_url: 'https://staging.okta.com', auto_create_users: true, auto_link_by_email: false, scim: true },
	{ id: '01J6XYZSHOWCASEIDP00002', name: 'Google Workspace', slug: 'google', enabled: false, client_id: 'google-oauth-client', issuer_url: 'https://accounts.google.com', auto_create_users: false, auto_link_by_email: false, scim: false },
];

function idpProto(p: (typeof DUMMY_IDPS)[number]) {
	return {
		id: wrappedID(p.id),
		name: p.name,
		slug: p.slug,
		providerType: 1,
		enabled: p.enabled,
		clientId: wrappedID(p.client_id),
		issuerUrl: p.issuer_url,
		scopes: ['openid', 'email', 'profile'],
		autoCreateUsers: p.auto_create_users,
		autoLinkByEmail: p.auto_link_by_email,
		groupClaim: 'groups',
		groupMapping: {},
		createdAt: isoFromSec(now - 120 * day),
		updatedAt: isoFromSec(now - 10 * day),
	};
}

export function listIdentityProvidersResponse() {
	return { providers: DUMMY_IDPS.map(idpProto), nextPageToken: '', totalCount: DUMMY_IDPS.length };
}

export function getIdentityProviderResponse(id: string) {
	const p = DUMMY_IDPS.find((x) => x.id === id) ?? DUMMY_IDPS[0];
	return { provider: idpProto(p) };
}

export function listIdentityLinksResponse() {
	return {
		links: [
			{
				id: wrappedID('01J6XYZSHOWCASELINK0001'),
				userId: wrappedID('01J6XYZSHOWCASEADMINUSR01'),
				providerId: wrappedID('01J6XYZSHOWCASEIDP00001'),
				providerName: 'Okta (Staging)',
				providerSlug: 'okta-staging',
				externalId: wrappedID('okta|00u123'),
				externalEmail: 'admin@cadestro.example',
				externalName: 'Sam Reiter',
				linkedAt: isoFromSec(now - 100 * day),
				lastLoginAt: isoFromSec(now - 1 * hour),
			},
		],
	};
}

export function listAuthMethodsResponse() {
	return {
		providers: DUMMY_IDPS.filter((p) => p.enabled).map((p) => ({
			slug: p.slug,
			name: p.name,
			providerType: 1,
		})),
	};
}

export function getServerSettingsResponse() {
	return { settings: { userProvisioningEnabled: true, sshAccessForAll: false } };
}

export function listActiveTerminalSessionsResponse() {
	return {
		sessions: [
			{
				sessionId: wrappedID('01J6XYZSHOWCASETERM0001'),
				userId: wrappedID('01J6XYZSHOWCASEADMINUSR01'),
				userEmail: 'admin@cadestro.example',
				deviceId: wrappedID('01J6XYZSHOWCASEDEVICE0001'),
				deviceHostname: 'edge-01.berlin',
				ttyUser: 'root',
				startedAt: isoFromSec(now - 18 * min),
				lastActivityAt: isoFromSec(now - 30),
			},
		],
		nextPageToken: '',
		totalCount: 1,
	};
}

export function listAvailableActionsResponse() {
	return {
		items: DUMMY_ACTIONS.slice(0, 4).map((a) => ({
			actionId: wrappedID(a.id),
			actionName: a.name,
			actionType: a.type,
			sourceType: 'ASSIGNMENT_SOURCE_TYPE_DIRECT',
			sourceId: wrappedID('01J6XYZSHOWCASEADMINUSR01'),
			sourceName: 'Sam Reiter',
		})),
	};
}
