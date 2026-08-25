

import * as m from '$lib/paraglide/messages';

export type AuditSummaryEvent = {
	eventType: string;
	data: string;
};

export type AuditSummaryContext = {

	deviceName: (id: string) => string;
};

export const SUMMARIZED_EVENT_TYPES = [
	'OSQueryDispatched',
	'DeviceLogsQueried',
	'DeviceInventoryRefreshRequested',
	'LuksTokenCreated',
	'LpsPasswordsViewed',
	'LuksKeysViewed',
	'LpsPasswordsViewDenied',
	'LuksKeysViewDenied',
	'UserLoggedOut',
	'UserSessionRefreshed',
	'TerminalSessionStarted',
	'TerminalSessionStopped',
	'TerminalSessionTerminated'
] as const;

function str(v: unknown): string {
	return typeof v === 'string' ? v : '';
}

function count(v: unknown): number {
	return Array.isArray(v) ? v.length : 0;
}

export function auditEventSummary(event: AuditSummaryEvent, ctx: AuditSummaryContext): string | null {
	let data: Record<string, unknown>;
	try {
		const parsed: unknown = JSON.parse(event.data || '{}');
		if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return null;
		data = parsed as Record<string, unknown>;
	} catch (err) {
		console.debug('audit summary: payload parse failed', err);
		return null;
	}

	const device = () => ctx.deviceName(str(data.device_id));

	switch (event.eventType) {
		case 'OSQueryDispatched':
			return m.audit_summary_osquery_dispatched({ table: str(data.table_name), device: device() });
		case 'DeviceLogsQueried':
			return str(data.unit)
				? m.audit_summary_device_logs_queried_unit({ unit: str(data.unit), device: device() })
				: m.audit_summary_device_logs_queried({ device: device() });
		case 'DeviceInventoryRefreshRequested':
			return m.audit_summary_inventory_refresh_requested({ device: device() });
		case 'LuksTokenCreated':
			return m.audit_summary_luks_token_created({ device: device() });
		case 'LpsPasswordsViewed':
			return m.audit_summary_lps_viewed({ count: String(count(data.entries)), device: device() });
		case 'LuksKeysViewed':
			return m.audit_summary_luks_viewed({ count: String(count(data.entries)), device: device() });
		case 'LpsPasswordsViewDenied':
			return m.audit_summary_lps_view_denied({ device: device(), reason: str(data.reason) });
		case 'LuksKeysViewDenied':
			return m.audit_summary_luks_view_denied({ device: device(), reason: str(data.reason) });
		case 'UserLoggedOut':
			return m.audit_summary_user_logged_out();
		case 'UserSessionRefreshed':
			return m.audit_summary_session_refreshed();
		case 'TerminalSessionStarted':
			return m.audit_summary_terminal_started({ ttyUser: str(data.tty_user) });
		case 'TerminalSessionStopped':
			return m.audit_summary_terminal_stopped({ reason: str(data.reason) });
		case 'TerminalSessionTerminated':
			return m.audit_summary_terminal_terminated({ reason: str(data.reason) });
	}
	return null;
}

export function auditEventOutcome(eventType: string): 'success' | 'denied' {
	return eventType.endsWith('Denied') ? 'denied' : 'success';
}
