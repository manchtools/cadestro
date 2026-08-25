import { ActionType } from '$contract/cadestro/v1/actions_pb';
import {
	Package,
	Terminal,
	Play,
	FileText,
	Settings2,
	Zap,
	RotateCw,
	Database,
	Folder,
	UserCog,
	KeyRound,
	Shield,
	ShieldCheck,
	Lock,
	HardDrive,
	Users,
	Wifi,
	Download,
	type IconNode
} from '@lucide/svelte';
import * as m from '$lib/paraglide/messages';
import {
	getActionTypeEnum,
	actionTypeToString,
	ACTION_TYPE_OPTIONS
} from '$contractClient/action-types';

export interface ActionTypeInfo {
	label: string;
	icon: typeof Package;
	description: string;
}

export function getActionTypeInfo(type: ActionType): ActionTypeInfo {
	switch (type) {
		case ActionType.PACKAGE:
			return {
				label: m.actions_type_package(),
				icon: Package,
				description: m.actions_type_package_description()
			};
		case ActionType.UPDATE:
			return {
				label: m.actions_type_update(),
				icon: RotateCw,
				description: m.actions_type_update_description()
			};
		case ActionType.APP_IMAGE:
			return {
				label: m.actions_type_app_image(),
				icon: Package,
				description: m.actions_type_app_image_description()
			};
		case ActionType.DEB:
			return {
				label: m.actions_type_deb(),
				icon: Package,
				description: m.actions_type_deb_description()
			};
		case ActionType.RPM:
			return {
				label: m.actions_type_rpm(),
				icon: Package,
				description: m.actions_type_rpm_description()
			};
		case ActionType.FLATPAK:
			return {
				label: m.actions_type_flatpak(),
				icon: Package,
				description: m.actions_type_flatpak_description()
			};
		case ActionType.SHELL:
			return {
				label: m.actions_type_shell(),
				icon: Terminal,
				description: m.actions_type_shell_description()
			};
		case ActionType.SERVICE:
			return {
				label: m.actions_type_systemd(),
				icon: Settings2,
				description: m.actions_type_systemd_description()
			};
		case ActionType.FILE:
			return {
				label: m.actions_type_file(),
				icon: FileText,
				description: m.actions_type_file_description()
			};
		case ActionType.REPOSITORY:
			return {
				label: m.actions_type_repository(),
				icon: Database,
				description: m.actions_type_repository_description()
			};
		case ActionType.DIRECTORY:
			return {
				label: m.actions_type_directory(),
				icon: Folder,
				description: m.actions_type_directory_description()
			};
		case ActionType.USER:
			return {
				label: m.actions_type_user(),
				icon: UserCog,
				description: m.actions_type_user_description()
			};
		case ActionType.SSH:
			return {
				label: m.actions_type_ssh(),
				icon: KeyRound,
				description: m.actions_type_ssh_description()
			};
		case ActionType.SSHD:
			return {
				label: m.actions_type_sshd(),
				icon: Shield,
				description: m.actions_type_sshd_description()
			};
		case ActionType.ADMIN_POLICY:
			return {
				label: m.actions_type_sudo(),
				icon: ShieldCheck,
				description: m.actions_type_sudo_description()
			};
		case ActionType.LPS:
			return {
				label: m.actions_type_lps(),
				icon: Lock,
				description: m.actions_type_lps_description()
			};
		case ActionType.GROUP:
			return {
				label: m.actions_type_group(),
				icon: Users,
				description: m.actions_type_group_description()
			};
		case ActionType.ENCRYPTION:
			return {
				label: m.actions_type_luks(),
				icon: HardDrive,
				description: m.actions_type_luks_description()
			};
		case ActionType.SCRIPT_RUN:
			return {
				label: m.actions_type_script_run(),
				icon: Play,
				description: m.actions_type_script_run_description()
			};
		case ActionType.WIFI:
			return {
				label: m.actions_type_wifi(),
				icon: Wifi,
				description: m.actions_type_wifi_description()
			};
		case ActionType.AGENT_UPDATE:
			return {
				label: m.actions_type_agent_update(),
				icon: Download,
				description: m.actions_type_agent_update_description()
			};
		default:
			return {
				label: m.actions_type_unknown(),
				icon: Zap,
				description: m.actions_type_unknown_description()
			};
	}
}

export function getActionTypeInfoByValue(value: string): ActionTypeInfo {
	if (value === 'COMPLIANCE_CHECK') {
		return {
			label: m.actions_type_compliance_check(),
			icon: ShieldCheck,
			description: m.actions_type_compliance_check_description()
		};
	}
	return getActionTypeInfo(getActionTypeEnum(value));
}

export function getActionTypeLabel(type: ActionType): string {
	return getActionTypeInfo(type).label;
}

export function getActionTypeIcon(type: ActionType) {
	return getActionTypeInfo(type).icon;
}

export function getActionTypeOptions() {
	return [
		{ value: 'PACKAGE', label: m.actions_type_option_package(), type: ActionType.PACKAGE },
		{ value: 'REPOSITORY', label: m.actions_type_option_repository(), type: ActionType.REPOSITORY },
		{ value: 'UPDATE', label: m.actions_type_option_update(), type: ActionType.UPDATE },
		{ value: 'SHELL', label: m.actions_type_option_shell(), type: ActionType.SHELL },
		{ value: 'SERVICE', label: m.actions_type_option_systemd(), type: ActionType.SERVICE },
		{ value: 'FILE', label: m.actions_type_option_file(), type: ActionType.FILE },
		{ value: 'DIRECTORY', label: m.actions_type_option_directory(), type: ActionType.DIRECTORY },
		{ value: 'APP_IMAGE', label: m.actions_type_option_app_image(), type: ActionType.APP_IMAGE },
		{ value: 'DEB', label: m.actions_type_option_deb(), type: ActionType.DEB },
		{ value: 'RPM', label: m.actions_type_option_rpm(), type: ActionType.RPM },
		{ value: 'FLATPAK', label: m.actions_type_option_flatpak(), type: ActionType.FLATPAK },
		{ value: 'USER', label: m.actions_type_option_user(), type: ActionType.USER },
		{ value: 'SSH', label: m.actions_type_option_ssh(), type: ActionType.SSH },
		{ value: 'SSHD', label: m.actions_type_option_sshd(), type: ActionType.SSHD },
		{ value: 'ADMIN_POLICY', label: m.actions_type_option_sudo(), type: ActionType.ADMIN_POLICY },
		{ value: 'LPS', label: m.actions_type_option_lps(), type: ActionType.LPS },
		{ value: 'GROUP', label: m.actions_type_option_group(), type: ActionType.GROUP },
		{ value: 'ENCRYPTION', label: m.actions_type_option_luks(), type: ActionType.ENCRYPTION },
		{ value: 'COMPLIANCE_CHECK', label: m.actions_type_option_compliance_check(), type: ActionType.SHELL },
		{ value: 'AGENT_UPDATE', label: m.actions_type_option_agent_update(), type: ActionType.AGENT_UPDATE }
	];
}

export interface ActionTypeGroup {
	id: string;
	label: string;
	description: string;
	types: Array<{ value: string; type: ActionType }>;
}

export function getGroupedActionTypeOptions(): ActionTypeGroup[] {
	return [
		{
			id: 'packages',
			label: m.actions_group_packages(),
			description: m.actions_group_packages_description(),
			types: [
				{ value: 'PACKAGE', type: ActionType.PACKAGE },
				{ value: 'REPOSITORY', type: ActionType.REPOSITORY },
				{ value: 'UPDATE', type: ActionType.UPDATE },
				{ value: 'APP_IMAGE', type: ActionType.APP_IMAGE },
				{ value: 'DEB', type: ActionType.DEB },
				{ value: 'RPM', type: ActionType.RPM },
				{ value: 'FLATPAK', type: ActionType.FLATPAK }
			]
		},
		{
			id: 'system',
			label: m.actions_group_system(),
			description: m.actions_group_system_description(),
			types: [
				{ value: 'SERVICE', type: ActionType.SERVICE },
				{ value: 'USER', type: ActionType.USER },
				{ value: 'SSH', type: ActionType.SSH },
				{ value: 'SSHD', type: ActionType.SSHD },
				{ value: 'ADMIN_POLICY', type: ActionType.ADMIN_POLICY },
				{ value: 'LPS', type: ActionType.LPS },
				{ value: 'ENCRYPTION', type: ActionType.ENCRYPTION },
				{ value: 'GROUP', type: ActionType.GROUP },
				{ value: 'AGENT_UPDATE', type: ActionType.AGENT_UPDATE }
			]
		},
		{
			id: 'files',
			label: m.actions_group_files(),
			description: m.actions_group_files_description(),
			types: [
				{ value: 'FILE', type: ActionType.FILE },
				{ value: 'DIRECTORY', type: ActionType.DIRECTORY }
			]
		},
		{
			id: 'scripts',
			label: m.actions_group_scripts(),
			description: m.actions_group_scripts_description(),
			types: [
				{ value: 'SHELL', type: ActionType.SHELL },
				{ value: 'COMPLIANCE_CHECK', type: ActionType.SHELL }
			]
		},
		{
			id: 'network',
			label: m.actions_group_network(),
			description: m.actions_group_network_description(),
			types: [{ value: 'WIFI', type: ActionType.WIFI }]
		}
	];
}

export { getActionTypeEnum, actionTypeToString, ACTION_TYPE_OPTIONS };

export const MARKETPLACE_SAFE_ACTION_TYPES: ReadonlySet<string> = new Set([
	'PACKAGE',
	'REPOSITORY',
	'SHELL',
	'SERVICE',
	'FILE',
	'DIRECTORY',
	'APP_IMAGE',
	'DEB',
	'RPM',
	'FLATPAK',
	'SSHD',
	'ADMIN_POLICY'
]);

export function getMarketplaceGroupedActionTypeOptions(): ActionTypeGroup[] {
	return getGroupedActionTypeOptions()
		.map((group) => ({
			...group,
			types: group.types.filter((t) => MARKETPLACE_SAFE_ACTION_TYPES.has(t.value))
		}))
		.filter((group) => group.types.length > 0);
}
