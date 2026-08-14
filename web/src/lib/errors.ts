import * as m from '$lib/paraglide/messages';
import { getErrorCode, getRequestId } from '$contractClient/client';

const errorMessages: Record<string, () => string> = {
	not_authenticated: m.error_not_authenticated,
	token_expired: m.error_token_expired,
	permission_denied: m.error_permission_denied,
	rate_limited: m.error_rate_limited,
	user_not_found: m.error_user_not_found,
	device_not_found: m.error_device_not_found,
	action_not_found: m.error_action_not_found,
	action_set_not_found: m.error_action_set_not_found,
	definition_not_found: m.error_definition_not_found,
	device_group_not_found: m.error_device_group_not_found,
	user_group_not_found: m.error_user_group_not_found,
	role_not_found: m.error_role_not_found,
	provider_not_found: m.error_provider_not_found,
	identity_link_not_found: m.error_identity_link_not_found,
	token_not_found: m.error_token_not_found,
	execution_not_found: m.error_execution_not_found,
	assignment_not_found: m.error_assignment_not_found,
	email_already_exists: m.error_email_already_exists,
	user_group_name_exists: m.error_user_group_name_exists,
	device_group_name_exists: m.error_device_group_name_exists,
	role_name_exists: m.error_role_name_exists,
	provider_slug_exists: m.error_provider_slug_exists,
	user_already_has_role: m.error_user_already_has_role,
	group_already_has_role: m.error_group_already_has_role,
	user_already_in_group: m.error_user_already_in_group,
	device_already_in_group: m.error_device_already_in_group,
	provider_disabled: m.error_provider_disabled,
	group_not_dynamic: m.error_group_not_dynamic,
	dynamic_group_manual_modify: m.error_dynamic_group_manual_modify,
	cannot_delete_system_role: m.error_cannot_delete_system_role,
	cannot_rename_system_role: m.error_cannot_rename_system_role,
	cannot_modify_system_action: m.error_cannot_modify_system_action,
	role_in_use: m.error_role_in_use,
	scim_already_enabled: m.error_scim_already_enabled,
	scim_not_enabled: m.error_scim_not_enabled,
	scim_managed_resource: m.error_scim_managed_resource,
	sso_state_expired: m.error_sso_state_expired,
	no_assignment_found: m.error_no_assignment_found,
	device_not_connected: m.error_device_not_connected,
	cannot_unlink_other_user: m.error_cannot_unlink_other_user,
	last_auth_method: m.error_last_auth_method,
	validation_failed: m.error_validation_failed,
	invalid_page_token: m.error_invalid_page_token,
	invalid_query: m.error_invalid_query,
	internal_error: m.error_internal,
	unimplemented: m.error_unimplemented,
	compliance_policy_not_found: m.error_compliance_policy_not_found,
	compliance_policy_name_exists: m.error_compliance_policy_name_exists,
	action_not_compliance: m.error_action_not_compliance,
	terminal_not_configured: m.error_terminal_not_configured,
	terminal_linux_username_not_set: m.error_terminal_linux_username_not_set,
	query_result_not_found: m.error_query_result_not_found
};

/**
 * Get a localized error message for a ConnectError.
 * Falls back to the raw error message if no i18n mapping exists.
 */
/** Error codes where the cause is obvious — no request ID needed in the message. */
const userFacingCodes = new Set([
	'not_authenticated',
	'token_expired',
	'permission_denied',
	'rate_limited',
	'validation_failed'
]);

export function getLocalizedError(error: unknown): string {
	const code = getErrorCode(error);
	let msg: string;
	if (code && errorMessages[code]) {
		msg = errorMessages[code]();
	} else if (error instanceof Error) {
		msg = error.message;
	} else {
		msg = m.error_internal();
	}

	// Append request ID for non-obvious errors so users can report them
	if (!code || !userFacingCodes.has(code)) {
		const requestId = getRequestId(error);
		if (requestId) {
			msg += ` (Request ID: ${requestId})`;
		}
	}

	return msg;
}

export { getErrorCode };
export * from '$contractClient/errors';
