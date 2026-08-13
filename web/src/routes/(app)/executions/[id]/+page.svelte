<script lang="ts">
	import { onMount } from 'svelte';
	import { getLocalizedError } from '$lib/errors';
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type ActionExecution, type Device, formatTimestampDateTime, formatDuration } from '$lib/sdk';
	import { ExecutionStatus, DesiredState } from '$sdk/powermanage/v1/common_pb';
	import { ActionType } from '$sdk/powermanage/v1/actions_pb';
	import { getActionTypeLabel } from '$lib/components/actions/action-type';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Chip } from '$lib/components/fleet';
	import { getExecutionStatusTone } from '$lib/execution-status';
	import { ArrowLeft, RefreshCw, Monitor, Zap, Clock, AlertCircle, CheckCircle2, XCircle, Timer, Info } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	let execution = $state<ActionExecution | null>(null);
	let device = $state<Device | null>(null);
	let loading = $state(true);

	const executionId = $derived(page.params.id ?? '');

	onMount(() => {
		if (executionId) {
			loadExecution();
		}
	});

	async function loadExecution() {
		if (!executionId) return;
		loading = true;
		try {
			execution = (await apiClient.getExecution(executionId)) ?? null;

			// Load related data
			if (execution) {
				await loadRelatedData();
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function loadRelatedData() {
		if (!execution) return;

		// Load device
		try {
			device = (await apiClient.getDevice(execution.deviceId)) ?? null;
		} catch (error) {
			console.error('Failed to load device', error);
		}
	}

	function getStatusLabel(status: ExecutionStatus): string {
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

	let cancelling = $state(false);
	// The list page uses the same AlertDialog + keys for this mutation.
	let cancelDialogOpen = $state(false);

	async function cancelScheduled() {
		if (!execution) return;
		cancelDialogOpen = false;
		cancelling = true;
		try {
			const updated = await apiClient.cancelExecution(execution.id);
			if (updated) execution = updated;
			toast.success(m.execution_cancelled_toast());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			cancelling = false;
		}
	}

	function getStatusIcon(status: ExecutionStatus) {
		switch (status) {
			case ExecutionStatus.SUCCESS:
				return CheckCircle2;
			case ExecutionStatus.FAILED:
			case ExecutionStatus.TIMEOUT:
			case ExecutionStatus.INDETERMINATE:
				return XCircle;
			case ExecutionStatus.RUNNING:
				return RefreshCw;
			case ExecutionStatus.PENDING:
				return Clock;
			case ExecutionStatus.NOT_APPLICABLE:
				return Info;
			default:
				return AlertCircle;
		}
	}


	function formatOutput(output: ActionExecution['output']): string {
		if (!output) return '';
		const parts: string[] = [];
		if (output.stdout) parts.push(`STDOUT:\n${output.stdout}`);
		if (output.stderr) parts.push(`STDERR:\n${output.stderr}`);
		if (output.exitCode !== undefined) parts.push(`Exit Code: ${output.exitCode}`);
		return parts.join('\n\n');
	}

	function isInstantAction(type: ActionType): boolean {
		return type === ActionType.REBOOT || type === ActionType.SYNC;
	}

	function getGoalLabel(desiredState: number, actionType: ActionType): string {
		// Instant actions use "Execute" instead of "Install"
		if (isInstantAction(actionType)) {
			return m.executions_goal_execute();
		}
		return desiredState === DesiredState.ABSENT
			? m.executions_goal_absent()
			: m.executions_goal_present();
	}

	function getChangedLabel(changed: boolean): string {
		return changed ? m.executions_changed() : m.executions_unchanged();
	}
</script>

{#snippet sectionLabel(text: string)}
	<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{text}</span>
{/snippet}

<div class="flex-1 overflow-auto p-4 md:p-6">
<div class="space-y-6">
	<div class="flex items-center gap-4">
		<Button variant="ghost" size="icon" onclick={() => history.back()}>
			<ArrowLeft class="h-4 w-4" />
		</Button>
		<div class="flex-1">
			<h1 class="text-2xl font-bold">{m.execution_detail_title()}</h1>
			<p class="text-muted-foreground text-sm font-mono">{executionId}</p>
		</div>
		{#if execution?.status === ExecutionStatus.SCHEDULED}
			<Button variant="destructive" onclick={() => (cancelDialogOpen = true)} disabled={cancelling}>
				<XCircle class="h-4 w-4 mr-2" />
				{m.execution_cancel()}
			</Button>
		{/if}
		<Button variant="outline" onclick={loadExecution} disabled={loading}>
			<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
				<RefreshCw class="h-4 w-4" />
			</span>
			{m.common_refresh()}
		</Button>
	</div>

	{#if loading && !execution}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if execution}
		{@const StatusIcon = getStatusIcon(execution.status)}
		<div class="grid gap-6 md:grid-cols-2">
			<!-- Status Card -->
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex items-center gap-2">
					<StatusIcon class="h-4 w-4 text-faint" />
					{@render sectionLabel(m.execution_detail_status())}
				</div>
				<div class="mt-3 space-y-4">
					<div class="flex items-center gap-3">
						<Chip
							tone={getExecutionStatusTone(execution.status)}
							label={getStatusLabel(execution.status)}
						/>
						{#if (execution.status === ExecutionStatus.SUCCESS || execution.status === ExecutionStatus.FAILED) && typeof execution.changed === 'boolean'}
							<span class="text-muted-foreground text-sm">
								({getChangedLabel(execution.changed)})
							</span>
						{/if}
						{#if execution.durationMs}
							<span class="text-muted-foreground text-sm flex items-center gap-1">
								<Timer class="h-4 w-4" />
								{formatDuration(execution.durationMs)}
							</span>
						{/if}
					</div>

					{#if execution.error}
						{#if execution.status === ExecutionStatus.NOT_APPLICABLE}
							<!-- Not-applicable is a non-error outcome: neutral styling, "Reason" header -->
							<div class="rounded-md border border-hair bg-sunken p-3">
								<div class="flex items-start gap-2">
									<Info class="mt-0.5 h-4 w-4 text-muted-foreground" />
									<div>
										<p class="text-sm font-medium">{m.execution_detail_reason()}</p>
										<p class="mt-1 text-sm text-muted-foreground">{execution.error}</p>
									</div>
								</div>
							</div>
						{:else}
							<div class="rounded-md border border-crit/40 bg-crit-soft p-3">
								<div class="flex items-start gap-2">
									<AlertCircle class="mt-0.5 h-4 w-4 text-crit" />
									<div>
										<p class="text-sm font-medium text-crit">{m.execution_detail_error()}</p>
										<p class="mt-1 text-sm text-crit/80">{execution.error}</p>
									</div>
								</div>
							</div>
						{/if}
					{/if}
				</div>
			</section>

			<!-- Action Card -->
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex items-center gap-2">
					<Zap class="h-4 w-4 text-faint" />
					{@render sectionLabel(m.execution_detail_action())}
				</div>
				<div class="mt-3 space-y-4">
					<div class="grid grid-cols-2 gap-4">
						{#if execution.actionId}
							<div>
								<Label class="text-muted-foreground">{m.execution_detail_name()}</Label>
								<p class="mt-1">
									<a href="{base}/actions/{execution.actionId}" class="font-medium hover:underline">
										{execution.actionName || execution.actionId.slice(0, 12) + '...'}
									</a>
								</p>
							</div>
						{/if}
						<div>
							<Label class="text-muted-foreground">{m.execution_detail_type()}</Label>
							<p class="mt-1 font-medium">{getActionTypeLabel(execution.type)}</p>
						</div>
						<div>
							<Label class="text-muted-foreground">{m.execution_detail_desired_state()}</Label>
							<p class="mt-1 font-medium">{getGoalLabel(execution.desiredState, execution.type)}</p>
						</div>
						<div>
							<Label class="text-muted-foreground">{m.execution_detail_created_by()}</Label>
							<p class="mt-1 text-sm">{execution.createdBy || m.common_unknown()}</p>
						</div>
					</div>
				</div>
			</section>

			<!-- Device Card -->
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex items-center gap-2">
					<Monitor class="h-4 w-4 text-faint" />
					{@render sectionLabel(m.execution_detail_device())}
				</div>
				<div class="mt-3 space-y-2">
					<div>
						<Label class="text-muted-foreground">{m.execution_detail_hostname()}</Label>
						<p class="mt-1">
							<a href="{base}/devices/{execution.deviceId}" class="font-medium hover:underline">
								{device?.hostname ?? execution.deviceId.slice(0, 12) + '...'}
							</a>
						</p>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.execution_detail_id()}</Label>
						<p class="mt-1 font-mono text-xs text-muted-foreground">{execution.deviceId}</p>
					</div>
				</div>
			</section>

			<!-- Timeline Card -->
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex items-center gap-2">
					<Clock class="h-4 w-4 text-faint" />
					{@render sectionLabel(m.execution_detail_timeline())}
				</div>
				<div class="mt-3 space-y-3">
					<div class="grid grid-cols-2 gap-4">
						<div>
							<Label class="text-muted-foreground">{m.executions_table_created()}</Label>
							<p class="mt-1 text-sm">{formatTimestampDateTime(execution.createdAt)}</p>
						</div>
						{#if execution.scheduledFor}
							<div>
								<Label class="text-muted-foreground">{m.execution_scheduled_for()}</Label>
								<p class="mt-1 text-sm">{formatTimestampDateTime(execution.scheduledFor)}</p>
							</div>
						{/if}
						{#if execution.dispatchedAt}
							<div>
								<Label class="text-muted-foreground">{m.execution_detail_dispatched()}</Label>
								<p class="mt-1 text-sm">{formatTimestampDateTime(execution.dispatchedAt)}</p>
							</div>
						{/if}
						{#if execution.completedAt}
							<div>
								<Label class="text-muted-foreground">{m.execution_detail_completed()}</Label>
								<p class="mt-1 text-sm">{formatTimestampDateTime(execution.completedAt)}</p>
							</div>
						{/if}
					</div>
				</div>
			</section>
		</div>

		<!-- Output Card (full width) -->
		{#if execution.output && (execution.output.stdout || execution.output.stderr)}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				{@render sectionLabel(m.execution_detail_output())}
				<pre class="mt-3 overflow-x-auto rounded-md bg-sunken p-4 font-mono text-sm whitespace-pre-wrap">{formatOutput(execution.output)}</pre>
			</section>
		{/if}
	{:else}
		<div
			class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface py-12 text-center shadow-plate"
		>
			<AlertCircle class="mb-4 h-12 w-12 text-muted-foreground" />
			<h3 class="font-semibold">{m.execution_detail_not_found()}</h3>
			<p class="text-muted-foreground">{m.execution_detail_not_found_hint()}</p>
			<Button variant="outline" class="mt-4" onclick={() => goto('/executions')}>
				{m.execution_detail_back()}
			</Button>
		</div>
	{/if}
</div>
</div>

<AlertDialog.Root bind:open={cancelDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.executions_cancel_dialog_title()}</AlertDialog.Title>
			<AlertDialog.Description>{m.execution_cancel_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={cancelScheduled} variant="destructive">
				{m.execution_cancel()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
