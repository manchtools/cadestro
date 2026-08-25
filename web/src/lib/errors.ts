import * as m from '$lib/paraglide/messages';
import { ErrorCode } from '$contract/cadestro/v1/common_pb';
import { getErrorCode, getRequestId } from '$contractClient/client';

const errorMessages: Partial<Record<ErrorCode, () => string>> = {
	[ErrorCode.NOT_AUTHENTICATED]: m.error_not_authenticated,
	[ErrorCode.TOKEN_EXPIRED]: m.error_token_expired,
	[ErrorCode.PERMISSION_DENIED]: m.error_permission_denied,
	[ErrorCode.RATE_LIMITED]: m.error_rate_limited,
	[ErrorCode.USER_NOT_FOUND]: m.error_user_not_found,
	[ErrorCode.DEVICE_NOT_FOUND]: m.error_device_not_found,
	[ErrorCode.ACTION_NOT_FOUND]: m.error_action_not_found,
	[ErrorCode.ACTION_SET_NOT_FOUND]: m.error_action_set_not_found,
	[ErrorCode.DEFINITION_NOT_FOUND]: m.error_definition_not_found,
	[ErrorCode.DEVICE_GROUP_NOT_FOUND]: m.error_device_group_not_found,
	[ErrorCode.USER_GROUP_NOT_FOUND]: m.error_user_group_not_found,
	[ErrorCode.ROLE_NOT_FOUND]: m.error_role_not_found,
	[ErrorCode.PROVIDER_NOT_FOUND]: m.error_provider_not_found,
	[ErrorCode.IDENTITY_LINK_NOT_FOUND]: m.error_identity_link_not_found,
	[ErrorCode.TOKEN_NOT_FOUND]: m.error_token_not_found,
	[ErrorCode.ASSIGNMENT_NOT_FOUND]: m.error_assignment_not_found,
	[ErrorCode.EMAIL_ALREADY_EXISTS]: m.error_email_already_exists,
	[ErrorCode.USER_GROUP_NAME_EXISTS]: m.error_user_group_name_exists,
	[ErrorCode.DEVICE_GROUP_NAME_EXISTS]: m.error_device_group_name_exists,
	[ErrorCode.ROLE_NAME_EXISTS]: m.error_role_name_exists,
	[ErrorCode.PROVIDER_SLUG_EXISTS]: m.error_provider_slug_exists,
	[ErrorCode.USER_ALREADY_HAS_ROLE]: m.error_user_already_has_role,
	[ErrorCode.GROUP_ALREADY_HAS_ROLE]: m.error_group_already_has_role,
	[ErrorCode.USER_ALREADY_IN_GROUP]: m.error_user_already_in_group,
	[ErrorCode.DEVICE_ALREADY_IN_GROUP]: m.error_device_already_in_group,
	[ErrorCode.PROVIDER_DISABLED]: m.error_provider_disabled,
	[ErrorCode.GROUP_NOT_DYNAMIC]: m.error_group_not_dynamic,
	[ErrorCode.DYNAMIC_GROUP_MANUAL_MODIFY]: m.error_dynamic_group_manual_modify,
	[ErrorCode.CANNOT_DELETE_SYSTEM_ROLE]: m.error_cannot_delete_system_role,
	[ErrorCode.CANNOT_RENAME_SYSTEM_ROLE]: m.error_cannot_rename_system_role,
	[ErrorCode.CANNOT_MODIFY_SYSTEM_ACTION]: m.error_cannot_modify_system_action,
	[ErrorCode.ROLE_IN_USE]: m.error_role_in_use,
	[ErrorCode.SCIM_ALREADY_ENABLED]: m.error_scim_already_enabled,
	[ErrorCode.SCIM_NOT_ENABLED]: m.error_scim_not_enabled,
	[ErrorCode.SCIM_MANAGED_RESOURCE]: m.error_scim_managed_resource,
	[ErrorCode.SSO_STATE_EXPIRED]: m.error_sso_state_expired,
	[ErrorCode.NO_ASSIGNMENT_FOUND]: m.error_no_assignment_found,
	[ErrorCode.DEVICE_NOT_CONNECTED]: m.error_device_not_connected,
	[ErrorCode.CANNOT_UNLINK_OTHER_USER]: m.error_cannot_unlink_other_user,
	[ErrorCode.LAST_AUTH_METHOD]: m.error_last_auth_method,
	[ErrorCode.VALIDATION_FAILED]: m.error_validation_failed,
	[ErrorCode.INVALID_PAGE_TOKEN]: m.error_invalid_page_token,
	[ErrorCode.INVALID_QUERY]: m.error_invalid_query,
	[ErrorCode.INTERNAL_ERROR]: m.error_internal,
	[ErrorCode.UNIMPLEMENTED]: m.error_unimplemented,
	[ErrorCode.COMPLIANCE_POLICY_NOT_FOUND]: m.error_compliance_policy_not_found,
	[ErrorCode.COMPLIANCE_POLICY_NAME_EXISTS]: m.error_compliance_policy_name_exists,
	[ErrorCode.ACTION_NOT_COMPLIANCE]: m.error_action_not_compliance,
	[ErrorCode.TERMINAL_NOT_CONFIGURED]: m.error_terminal_not_configured,
	[ErrorCode.TERMINAL_LINUX_USERNAME_NOT_SET]: m.error_terminal_linux_username_not_set,
	[ErrorCode.QUERY_RESULT_NOT_FOUND]: m.error_query_result_not_found
};

const userFacingCodes = new Set<ErrorCode>([
	ErrorCode.NOT_AUTHENTICATED,
	ErrorCode.TOKEN_EXPIRED,
	ErrorCode.PERMISSION_DENIED,
	ErrorCode.RATE_LIMITED,
	ErrorCode.VALIDATION_FAILED
]);

export function getLocalizedError(error: unknown): string {
	const code = getErrorCode(error);
	let msg: string;
	const localized = code !== undefined ? errorMessages[code] : undefined;
	if (localized) {
		msg = localized();
	} else if (error instanceof Error) {
		msg = error.message;
	} else {
		msg = m.error_internal();
	}

	if (code === undefined || !userFacingCodes.has(code)) {
		const requestId = getRequestId(error);
		if (requestId) {
			msg += ` (Request ID: ${requestId})`;
		}
	}

	return msg;
}

export { getErrorCode };
export * from '$contractClient/errors';
