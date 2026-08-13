// The one execution-status label map.
//
// It lived page-locally on the executions list while that page was its only
// caller; the execution watch window is a second surface that must read the
// same words, and a second switch would be a place for the two to drift apart.
// The i18n keys it names are the ones ./execution-status-labels.test.ts pins
// against the SDK enum, so a new ExecutionStatus still forces a label decision.
import { ExecutionStatus } from '$sdk/powermanage/v1/common_pb';
import type { FleetTone } from '$lib/components/fleet/tone';
import * as m from '$lib/paraglide/messages';

/**
 * Status tone for a run, in the fleet vocabulary — the same buckets the
 * execution watch window and the operations feed paint with (ok / crit for
 * terminal outcomes, info while the run is still in flight, warn for a run that
 * decided not to act). Chips carry status; a tone-less badge never does.
 */
export function getExecutionStatusTone(status: number): FleetTone {
	switch (status) {
		case ExecutionStatus.SUCCESS:
			return 'ok';
		case ExecutionStatus.FAILED:
		case ExecutionStatus.TIMEOUT:
		case ExecutionStatus.INDETERMINATE:
			return 'crit';
		case ExecutionStatus.PENDING:
		case ExecutionStatus.RUNNING:
		case ExecutionStatus.SCHEDULED:
			return 'info';
		case ExecutionStatus.SKIPPED:
		case ExecutionStatus.NOT_APPLICABLE:
			return 'warn';
		default:
			return 'idle';
	}
}

export function getExecutionStatusLabel(status: number): string {
	switch (status) {
		case ExecutionStatus.PENDING:
			return m.executions_status_pending();
		case ExecutionStatus.RUNNING:
			return m.executions_status_running();
		case ExecutionStatus.SUCCESS:
			return m.executions_status_success();
		case ExecutionStatus.FAILED:
			return m.executions_status_failed();
		case ExecutionStatus.INDETERMINATE:
			return m.executions_status_indeterminate();
		case ExecutionStatus.SKIPPED:
			return m.executions_status_skipped();
		case ExecutionStatus.NOT_APPLICABLE:
			return m.executions_status_not_applicable();
		case ExecutionStatus.TIMEOUT:
			return m.executions_status_timeout();
		case ExecutionStatus.SCHEDULED:
			return m.execution_status_scheduled();
		case ExecutionStatus.CANCELLED:
			return m.execution_status_cancelled();
		default:
			return m.executions_status_unknown();
	}
}
