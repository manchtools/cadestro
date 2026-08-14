<script lang="ts">
	// THE EXECUTION WATCH WINDOW — movement B/F's corner watch slot, scoped to
	// what the wire actually carries.
	//
	// One execution, followed by POLLING GetExecution: the contract has no
	// execution stream, so the window asks again on an interval and stops the
	// moment the run can no longer change. Everything it shows is a field of
	// `ActionExecution` that the execution detail route already shows — this
	// window invents no state, and in particular there is no progress bar,
	// because nothing on the wire says how far along a run is.
	//
	// It is a BACKGROUND surface: a failed poll writes an inline note, never a
	// toast, and three consecutive failures end the watch instead of hammering a
	// server that is evidently not answering.
	import { base } from '$app/paths';
	import {
		apiClient,
		formatTimestampDateTime,
		formatDuration,
		type ActionExecution
	} from '$lib/sdk';
	import { ExecutionStatus } from '$contract/cadestro/v1/common_pb';
	import { getActionTypeLabel } from '$lib/components/actions/action-type';
	import { getExecutionStatusLabel } from '$lib/execution-status';
	import { getLocalizedError } from '$lib/errors';
	import { Chip } from '$lib/components/fleet';
	import type { FleetTone } from '$lib/components/fleet';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { ExternalLink, RotateCw } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	let { executionId, pollMs = 5000 }: { executionId: string; pollMs?: number } = $props();

	/** The only statuses that can still change. Everything else — the terminal
	 *  set, and any status this build does not know — ends the watch, so an
	 *  unrecognised value can never become an endless poll. */
	const LIVE: ReadonlySet<number> = new Set([
		ExecutionStatus.PENDING,
		ExecutionStatus.RUNNING,
		ExecutionStatus.SCHEDULED
	]);

	/** Consecutive failed reads that end the watch. */
	const MAX_FAILURES = 3;
	/** Output is a tail, not a log viewer: the window is ~330px wide. */
	const TAIL_LINES = 10;

	// The same buckets the operations feed paints its effect rows with
	// (./operation-feed's statusBucket: failed / queued / ok / skipped / …).
	function tone(status: number): FleetTone {
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

	let execution = $state<ActionExecution | null>(null);
	/** The read answered, with nothing — a deleted or never-existing run. */
	let missing = $state(false);
	let error = $state('');
	/** The watch ended on a read that kept failing (not on a terminal status). */
	let gaveUp = $state(false);

	let failures = 0;
	let timer: ReturnType<typeof setTimeout> | undefined;
	// Generation stamp: the effect's teardown bumps it, so an in-flight read and
	// any pending timer belonging to an older run are ignored. That is what makes
	// "destroy stops polling" true even mid-request.
	let generation = 0;

	async function poll(run: number) {
		if (run !== generation) return;
		try {
			const next = await apiClient.getExecution(executionId);
			if (run !== generation) return;
			execution = next ?? null;
			missing = next === undefined;
			error = '';
			failures = 0;
			if (!next || !LIVE.has(next.status)) return;
		} catch (cause) {
			if (run !== generation) return;
			// Background surface: the note is in the window, not in a toast.
			error = getLocalizedError(cause);
			console.error('execution watch: GetExecution failed', cause);
			failures += 1;
			if (failures >= MAX_FAILURES) {
				gaveUp = true;
				return;
			}
		}
		timer = setTimeout(() => void poll(run), pollMs);
	}

	function watch() {
		generation += 1;
		clearTimeout(timer);
		timer = undefined;
		failures = 0;
		error = '';
		gaveUp = false;
		missing = false;
		void poll(generation);
	}

	$effect(() => {
		void executionId;
		watch();
		return () => {
			generation += 1;
			clearTimeout(timer);
			timer = undefined;
		};
	});

	const title = $derived(
		execution ? execution.actionName || getActionTypeLabel(execution.type) : ''
	);
	const running = $derived(execution?.status === ExecutionStatus.RUNNING);
	/** `live_output` is what the agent has sent so far; the final `output`
	 *  replaces it on completion. */
	const stream = $derived(execution?.liveOutput ?? execution?.output);
	const tail = $derived(
		[stream?.stdout ?? '', stream?.stderr ?? '']
			.join('\n')
			.split('\n')
			.filter((line) => line.trim() !== '')
			.slice(-TAIL_LINES)
			.join('\n')
	);
	// `changed` only means something once the run reached an outcome — exactly
	// the gate the executions feed and the detail route use.
	const settled = $derived(
		execution?.status === ExecutionStatus.SUCCESS || execution?.status === ExecutionStatus.FAILED
	);
	// `compliant` is the detection script's answer, so it is only an answer when
	// a detection actually ran.
	const detected = $derived(execution?.detectionOutput !== undefined);
</script>

<div class="space-y-3 text-sm" data-testid="execution-window">
	{#if missing}
		<p class="text-xs text-muted-foreground" data-testid="execution-window-missing">
			{m.execution_detail_not_found()}
		</p>
	{:else if !execution && !error}
		<div class="space-y-2" aria-label={m.common_loading()}>
			<Skeleton class="h-4 w-32" />
			<Skeleton class="h-4 w-full" />
		</div>
	{:else if execution}
		<div class="flex items-start justify-between gap-2">
			<span class="min-w-0 flex-1 font-medium">{title}</span>
			<span data-testid="execution-window-status" class="shrink-0">
				<Chip tone={tone(execution.status)} label={getExecutionStatusLabel(execution.status)} />
			</span>
		</div>

		<dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs">
			<!-- ActionExecution carries no hostname (only the search document does),
			     so the device reads as its id; the window title carries the label
			     the operator opened it from. -->
			<dt class="text-muted-foreground">{m.executions_table_device()}</dt>
			<dd class="truncate text-right font-mono" data-testid="execution-window-device">
				{execution.deviceId}
			</dd>

			<dt class="text-muted-foreground">{m.executions_table_created()}</dt>
			<dd class="text-right">{formatTimestampDateTime(execution.createdAt)}</dd>

			{#if execution.completedAt}
				<dt class="text-muted-foreground">{m.execution_detail_completed()}</dt>
				<dd class="text-right">{formatTimestampDateTime(execution.completedAt)}</dd>
			{/if}

			{#if execution.durationMs}
				<dt class="text-muted-foreground">{m.executions_table_duration()}</dt>
				<dd class="text-right">{formatDuration(execution.durationMs)}</dd>
			{/if}
		</dl>

		{#if settled || detected}
			<div class="flex flex-wrap gap-1.5">
				{#if settled}
					<Chip
						tone={execution.changed ? 'info' : 'idle'}
						label={execution.changed ? m.executions_changed() : m.executions_unchanged()}
					/>
				{/if}
				{#if detected}
					<Chip
						tone={execution.compliant ? 'ok' : 'warn'}
						label={execution.compliant
							? m.compliance_status_compliant()
							: m.compliance_status_non_compliant()}
					/>
				{/if}
			</div>
		{/if}

		{#if execution.error}
			<p class="rounded-md border border-crit/30 bg-crit-soft px-2.5 py-1.5 text-xs text-crit">
				{execution.error}
			</p>
		{/if}

		{#if running && tail}
			<div>
				<p class="mb-1 flex items-baseline justify-between gap-2 text-[11px] uppercase tracking-wide text-faint">
					<span>{m.execution_detail_output()}</span>
					<!-- The block is a TAIL, not the log: say how much of it this is,
					     so a missing earlier line is never read as a missing line. -->
					<span class="normal-case" data-testid="execution-window-tail-note">
						{m.executions_watch_tail({ count: TAIL_LINES })}
					</span>
				</p>
				<pre
					data-testid="execution-window-output"
					class="max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-md bg-sunken p-2 font-mono text-[11px] leading-snug">{tail}</pre>
			</div>
		{/if}
	{/if}

	{#if error}
		<!-- A failed read is a note in the window, never a toast: this surface
		     runs in the background while the operator works elsewhere. -->
		<div
			data-testid="execution-window-error"
			class="space-y-2 rounded-md border border-crit/30 bg-crit-soft px-2.5 py-2 text-xs text-crit"
		>
			<p>{m.execution_detail_load_failed()}</p>
			<p class="text-crit/80">{error}</p>
			{#if gaveUp}
				<!-- The watch has given up after MAX_FAILURES: it SAYS so, and asking
				     again becomes the operator's move. A control that quietly appears
				     leaves "is this still updating?" to be guessed; the sentence names
				     the number of failed reads that ended the watch. -->
				<div data-testid="execution-window-stopped" class="space-y-2">
					<p>{m.executions_watch_stopped({ count: MAX_FAILURES })}</p>
					<Button size="sm" variant="outline" onclick={watch}>
						<RotateCw class="h-3.5 w-3.5" />
						{m.common_refresh()}
					</Button>
				</div>
			{/if}
		</div>
	{/if}

	<div class="border-t pt-2">
		<Button size="sm" variant="outline" href="{base}/executions/{executionId}">
			<ExternalLink class="h-3.5 w-3.5" />
			{m.executions_open_details()}
		</Button>
	</div>
</div>
