<script lang="ts" generics="R extends FeedRow">
	// Movement F: one card per operation. Counts up front, effect rows on demand,
	// partial success rendered as a shape rather than an error.
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { formatTimestampDateTime, formatDuration } from '$lib/sdk';
	import { ExecutionStatus, DesiredState } from '$contract/cadestro/v1/common_pb';
	import { getActionTypeLabel } from '$lib/components/actions/action-type';
	import { Chip } from '$lib/components/fleet';
	import type { FleetTone } from '$lib/components/fleet';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { openPanel } from '$lib/shell/shell.svelte';
	import {
		BUCKET_ORDER,
		RETRY_MAX_DEVICES,
		statusBucket,
		type EffectBucket,
		type FeedRow,
		type Operation
	} from './operation-feed';
	import {
		AppWindow,
		ChevronRight,
		Eye,
		ExternalLink,
		MoreHorizontal,
		RotateCcw,
		XCircle
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	let {
		operation,
		open = false,
		tour,
		statusLabel,
		onretry,
		oncancel
	}: {
		operation: Operation<R>;
		open?: boolean;
		/** data-tour marker; only the first card carries one. */
		tour?: string;
		statusLabel: (status: number) => string;
		onretry: (operation: Operation<R>) => void;
		oncancel: (row: R) => void;
	} = $props();

	const TONE: Record<EffectBucket, FleetTone> = {
		failed: 'crit',
		queued: 'info',
		ok: 'ok',
		skipped: 'warn',
		cancelled: 'idle',
		unknown: 'idle'
	};

	function countLabel(bucket: EffectBucket, count: number): string {
		switch (bucket) {
			case 'failed':
				return m.executions_op_count_failed({ count });
			case 'queued':
				return m.executions_op_count_queued({ count });
			case 'ok':
				return m.executions_op_count_ok({ count });
			case 'skipped':
				return m.executions_op_count_skipped({ count });
			case 'cancelled':
				return m.executions_op_count_cancelled({ count });
			default:
				return m.executions_op_count_unknown({ count });
		}
	}

	function goalLabel(desiredState: number): string {
		return desiredState === DesiredState.ABSENT
			? m.executions_goal_absent()
			: m.executions_goal_present();
	}

	function deviceLabel(row: FeedRow): string {
		// Falls back to a short device id when the index is stale or the field is
		// missing — the hostname only exists on the raw search document.
		return row.deviceHostname || row.deviceId.slice(0, 8) + '...';
	}

	const title = $derived(operation.actionName || getActionTypeLabel(operation.type));

	/** Window title for a watched effect — the stage rail and the panel header
	 *  both render it in one line, so it is cut to a headline length here. */
	function watchTitle(row: FeedRow): string {
		const label = `${title} · ${deviceLabel(row)}`;
		return label.length > 44 ? label.slice(0, 43) + '…' : label;
	}

	const shownCounts = $derived(
		BUCKET_ORDER.filter((bucket) => operation.counts[bucket] > 0).map((bucket) => ({
			bucket,
			count: operation.counts[bucket]
		}))
	);
</script>

<details
	class="group overflow-hidden rounded-xl border border-hair bg-surface shadow-plate"
	data-testid="operation-card"
	data-tour={tour}
	data-operation-key={operation.key}
	{open}
>
	<summary
		class="flex cursor-pointer list-none flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 hover:bg-sunken focus-visible:outline-2 focus-visible:outline-ring [&::-webkit-details-marker]:hidden"
	>
		<ChevronRight
			class="h-4 w-4 shrink-0 text-faint transition-transform group-open:rotate-90"
			aria-hidden="true"
		/>
		<span class="font-medium">
			{m.executions_op_headline({ action: title, count: operation.deviceIds.length })}
		</span>
		<span class="flex flex-wrap items-center gap-1.5" data-testid="operation-counts">
			{#each shownCounts as entry (entry.bucket)}
				<Chip tone={TONE[entry.bucket]} label={countLabel(entry.bucket, entry.count)} />
			{/each}
		</span>
		<span class="ml-auto whitespace-nowrap font-mono text-xs text-muted-foreground">
			{#if operation.actor}{operation.actor} ·
			{/if}{formatTimestampDateTime(operation.startedAt)}
		</span>
	</summary>

	<div class="border-t border-hair px-4 pb-3">
		<ul class="divide-y divide-hair" data-testid="operation-effects">
			{#each operation.effects as effect (effect.id)}
				{@const bucket = statusBucket(effect.status)}
				<li class="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm">
					<span class="min-w-0">
						<a
							href="{base}/devices/{effect.deviceId}"
							class="font-mono hover:underline"
							data-testid="effect-device"
						>
							{deviceLabel(effect)}
						</a>
						<a
							href="{base}/executions/{effect.id}"
							class="block truncate font-mono text-xs text-faint hover:underline"
						>
							{effect.id}
						</a>
					</span>
					<span class="font-mono text-xs text-muted-foreground">
						{goalLabel(effect.desiredState ?? 0)}
						· {formatDuration(effect.durationMs)}
						{#if (effect.status === ExecutionStatus.SUCCESS || effect.status === ExecutionStatus.FAILED) && typeof effect.changed === 'boolean'}
							· {effect.changed ? m.executions_changed() : m.executions_unchanged()}
						{/if}
					</span>
					<span class="ml-auto flex items-center gap-2">
						<Chip tone={TONE[bucket]} label={statusLabel(effect.status)} />
						<DropdownMenu.Root>
							<DropdownMenu.Trigger>
								{#snippet child({ props })}
									<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
										<MoreHorizontal class="h-4 w-4" />
									</Button>
								{/snippet}
							</DropdownMenu.Trigger>
							<DropdownMenu.Content align="end">
								<DropdownMenu.Item onclick={() => goto(`/executions/${effect.id}`)}>
									<ExternalLink class="mr-2 h-4 w-4" />
									{m.executions_open_details()}
								</DropdownMenu.Item>
								<!-- Two windows, both scoped to ONE effect row: the run
								     itself (polled until it settles) and the device it
								     landed on. An operation is a derived cluster with no
								     server id, so it can never become a window. -->
								<DropdownMenu.Item
									onclick={() => openPanel('execution', effect.id, watchTitle(effect))}
								>
									<Eye class="mr-2 h-4 w-4" />
									{m.executions_watch_window()}
								</DropdownMenu.Item>
								<DropdownMenu.Item
									onclick={() => openPanel('device', effect.deviceId, deviceLabel(effect))}
								>
									<AppWindow class="mr-2 h-4 w-4" />
									{m.common_open_window()}
								</DropdownMenu.Item>
								{#if effect.status === ExecutionStatus.SCHEDULED}
									<DropdownMenu.Separator />
									<DropdownMenu.Item onclick={() => oncancel(effect)} class="text-destructive">
										<XCircle class="mr-2 h-4 w-4" />
										{m.execution_cancel()}
									</DropdownMenu.Item>
								{/if}
							</DropdownMenu.Content>
						</DropdownMenu.Root>
					</span>
				</li>
			{/each}
		</ul>

		{#if operation.counts.failed > 0}
			<div class="flex flex-wrap items-center gap-3 border-t border-hair pt-3">
				{#if operation.retryable}
					<Button
						variant="outline"
						size="sm"
						data-testid="operation-retry"
						onclick={() => onretry(operation)}
					>
						<RotateCcw class="mr-2 h-3.5 w-3.5" />
						{m.executions_op_retry_failed({ count: operation.retryDeviceIds.length })}
					</Button>
				{:else}
					<p class="text-xs text-muted-foreground" data-testid="operation-retry-unavailable">
						{operation.actionId
							? m.executions_op_retry_too_many({ limit: RETRY_MAX_DEVICES })
							: m.executions_op_retry_inline()}
					</p>
				{/if}
			</div>
		{/if}
	</div>
</details>
