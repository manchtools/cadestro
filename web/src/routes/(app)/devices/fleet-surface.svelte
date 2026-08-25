<script lang="ts">

	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { toast } from 'svelte-sonner';
	import { Stat, TONE_FILL } from '$lib/components/fleet';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as m from '$lib/paraglide/messages';
	import PageShell from '$lib/components/page-shell.svelte';
	import { codecs, readURLParam, syncToURL, onPopstate } from '$lib/url-state';
	import { enterSelection, exitSelection } from '$lib/shell/shell.svelte';
	import { setCarried } from '$lib/shell/carried-selection.svelte';
	import {
		buildBubbles,
		selectionFacts,
		summarize,
		toFleetDevice,
		worstFirst,
		type FleetBubble,
		type FleetDevice
	} from './fleet-model';
	import { bulkReboot, bulkSetLabel, type BulkOutcome } from './fleet-bulk';
	import AddLabelDialog from './[id]/add-label-dialog.svelte';
	import type { FleetSnapshot } from './fleet-data';
	import {
		clearFleetSelection,
		fleetSelectionSurface,
		getFleetSelection,
		setFleetSelection
	} from './fleet-selection.svelte';
	import BubbleGroup from './bubble-group.svelte';
	import NearPane from './near-pane.svelte';
	import FleetEmpty from './fleet-empty.svelte';

	let {
		surfaceId,
		heading,
		snapshot,
		loading = false,
		error = null,
		nowMs = Date.now(),
		emptyTitle,
		emptyHint,
		openLabel,
		onOpenDevice,
		headerExtra,
		filters,
		deviceLevel,
		emptyExtra
	}: {

		surfaceId: string;

		heading: (count: number) => string;
		snapshot: FleetSnapshot | null;
		loading?: boolean;
		error?: string | null;

		nowMs?: number;
		emptyTitle: string;
		emptyHint: string;

		openLabel: string;
		onOpenDevice: (device: FleetDevice) => void;
		headerExtra?: Snippet;
		filters?: Snippet;

		deviceLevel?: Snippet;
		emptyExtra?: Snippet;
	} = $props();

	const ZOOMS = ['fleet', 'group', 'device'] as const;
	type Zoom = (typeof ZOOMS)[number];

	const ZOOM_LABEL: Record<Zoom, () => string> = {
		fleet: m.fleet_zoom_fleet,
		group: m.fleet_zoom_group,
		device: m.fleet_zoom_device
	};

	const ZOOM_CODEC = codecs.enum<Zoom>(ZOOMS, 'fleet');
	const GROUP_CODEC = codecs.string('');

	let zoom = $state(readURLParam(page.url, 'zoom', ZOOM_CODEC));
	let groupId = $state(readURLParam(page.url, 'group', GROUP_CODEC));

	onPopstate((u) => {
		zoom = readURLParam(u, 'zoom', ZOOM_CODEC);
		groupId = readURLParam(u, 'group', GROUP_CODEC);
	});

	function setZoom(next: Zoom, group: string = groupId) {
		zoom = next;
		groupId = next === 'fleet' ? group : group || (bubbles[0]?.id ?? '');
		syncToURL(
			[
				['zoom', zoom, ZOOM_CODEC],
				['group', groupId, GROUP_CODEC]
			],
			'push'
		);
	}

	const nowSec = $derived(Math.floor(nowMs / 1000));

	const groupMinutes = $derived.by(() => {
		const out = new Map<string, number[]>();
		if (!snapshot) return out;
		for (const g of snapshot.groups) {
			if (g.syncIntervalMinutes <= 0) continue;
			for (const id of snapshot.membership.get((g.id?.value ?? '')) ?? []) {
				const list = out.get(id);
				if (list) list.push(g.syncIntervalMinutes);
				else out.set(id, [g.syncIntervalMinutes]);
			}
		}
		return out;
	});

	const devices = $derived(
		(snapshot?.devices ?? []).map((d) => toFleetDevice(d, groupMinutes.get((d.id?.value ?? '')) ?? [], nowSec))
	);
	const summary = $derived(summarize(devices));
	const bubbles = $derived(
		buildBubbles(devices, (snapshot?.groups ?? []).map((group) => ({ ...group, id: group.id?.value ?? '' })), snapshot?.membership ?? new Map(), m.fleet_ungrouped())
	);
	const focused = $derived<FleetBubble | null>(
		bubbles.find((b) => b.id === groupId) ?? bubbles[0] ?? null
	);
	const isEmpty = $derived(!loading && snapshot !== null && devices.length === 0);

	const allWorstFirst = $derived(worstFirst(devices));

	const selectedIds = $derived(getFleetSelection(surfaceId));
	const selected = $derived(new Set(selectedIds));

	let anchor: { bubbleId: string; index: number } | null = null;

	function toggle(bubbleId: string, list: FleetDevice[], index: number, shift: boolean) {
		const target = list[index];
		if (!target) return;
		if (shift && anchor && anchor.bubbleId === bubbleId) {
			const from = Math.min(anchor.index, index);
			const to = Math.max(anchor.index, index);
			const add = list.slice(from, to + 1).map((d) => d.id);
			setFleetSelection(surfaceId, [...new Set([...selectedIds, ...add])]);
			return;
		}
		anchor = { bubbleId, index };
		setFleetSelection(
			surfaceId,
			selected.has(target.id)
				? selectedIds.filter((id) => id !== target.id)
				: [...selectedIds, target.id]
		);
	}

	const CONFIRM_HOSTS = 8;

	let rebootOpen = $state(false);
	let labelOpen = $state(false);
	let running = $state(false);

	const selectedDevices = $derived(allWorstFirst.filter((d) => selected.has(d.id)));
	const confirmHosts = $derived(selectedDevices.slice(0, CONFIRM_HOSTS));
	const confirmMore = $derived(Math.max(0, selectedDevices.length - CONFIRM_HOSTS));

	function report(
		outcomes: BulkOutcome[],
		success: (input: { count: number }) => string,
		partial: (input: { ok: number; failed: number }) => string
	) {
		const failed = outcomes.filter((o) => !o.ok);
		if (failed.length === 0) {
			toast.success(success({ count: outcomes.length }));
			return;
		}
		console.error('fleet bulk: per-device failures', failed);
		toast.error(partial({ ok: outcomes.length - failed.length, failed: failed.length }));
	}

	async function runReboot() {
		const ids = [...selectedIds];
		if (ids.length === 0 || running) return;
		running = true;
		try {
			report(await bulkReboot(ids), m.bulk_reboot_requested, m.bulk_reboot_partial);
		} finally {
			running = false;
			rebootOpen = false;
		}
	}

	async function runLabel(key: string, value: string) {
		const ids = [...selectedIds];
		if (ids.length === 0 || running) return;
		running = true;
		try {
			report(await bulkSetLabel(ids, key, value), m.bulk_label_applied, m.bulk_label_partial);
		} finally {
			running = false;
			labelOpen = false;
		}
	}

	$effect(() => {
		const ids = getFleetSelection(surfaceId);
		if (ids.length === 0) {

			rebootOpen = false;
			labelOpen = false;
			if (fleetSelectionSurface() === surfaceId) exitSelection();
			return;
		}
		const facts = selectionFacts(ids, bubbles);
		enterSelection({
			count: ids.length,
			subtext: [
				m.fleet_selection_groups({ count: facts.groups }),
				m.fleet_selection_offline({ count: facts.offline })
			].join(' · '),
			subtextTone: facts.offline > 0 ? 'warn' : 'neutral',
			actions: [
				{
					id: 'assign',
					label: m.common_assign(),
					primary: true,
					onRun: () => {
						setCarried({
							deviceIds: [...ids],
							label: m.fleet_selection_label({ count: ids.length })
						});
						goto(base + '/assign');
					}
				},
				{ id: 'reboot', label: m.instant_actions_reboot(), onRun: () => (rebootOpen = true) },
				{ id: 'label', label: m.bulk_label_action(), onRun: () => (labelOpen = true) }
			],
			onClear: () => clearFleetSelection(surfaceId)
		});
	});

	const LEGEND: { tone: 'ok' | 'warn' | 'crit' | 'idle'; label: () => string }[] = [
		{ tone: 'ok', label: m.fleet_legend_ok },
		{ tone: 'warn', label: m.fleet_legend_warn },
		{ tone: 'crit', label: m.fleet_legend_crit },
		{ tone: 'idle', label: m.fleet_legend_idle }
	];
</script>

<PageShell contentClass="space-y-3">
	{#snippet header()}
		<div class="flex flex-wrap items-center gap-3">
			<h1 class="text-2xl font-bold">{heading(snapshot?.total ?? devices.length)}</h1>
			<div
				data-tour="fleet-zoom"
				role="group"
				aria-label={m.fleet_zoom_label()}
				class="inline-flex overflow-hidden rounded-lg border font-mono text-[0.68rem]"
			>
				{#each ZOOMS as z (z)}
					<button
						type="button"
						data-testid="fleet-zoom-{z}"
						aria-pressed={zoom === z}
						onclick={() => setZoom(z)}
						class="border-r px-2.5 py-1 last:border-r-0 {zoom === z
							? 'bg-accent-soft font-semibold text-accent-ink'
							: 'text-muted-foreground hover:text-foreground'}"
					>
						{ZOOM_LABEL[z]()}
					</button>
				{/each}
			</div>
			<div class="ml-auto flex items-center gap-2">
				{#if headerExtra}{@render headerExtra()}{/if}
			</div>
		</div>

		<div data-tour="fleet-summary" class="flex flex-wrap gap-2">
			<Stat tone="ok" value={summary.ok} label={m.fleet_stat_healthy()} />
			<Stat tone="warn" value={summary.warn} label={m.fleet_stat_drift()} />
			<Stat tone="crit" value={summary.crit} label={m.fleet_stat_offline()} />
			<Stat tone="idle" value={summary.idle} label={m.fleet_stat_never_seen()} />
		</div>

		{#if filters}{@render filters()}{/if}
	{/snippet}

	{#if error}
		<p data-testid="fleet-error" class="rounded-lg border border-crit bg-crit-soft px-3 py-2 text-sm text-crit">
			{error}
		</p>
	{/if}

	{#if snapshot?.truncated}

		<p data-testid="fleet-truncated" class="rounded-lg border border-warn bg-warn-soft px-3 py-2 text-xs text-warn">
			{m.fleet_truncated({ shown: devices.length, total: snapshot.total })}
		</p>
	{/if}

	{#if snapshot?.groupsError}
		<p data-testid="fleet-groups-unavailable" class="text-xs text-faint">
			{m.fleet_groups_unavailable()}
		</p>
	{/if}

	{#if isEmpty}
		<FleetEmpty title={emptyTitle} hint={emptyHint} extra={emptyExtra} />
	{:else if zoom === 'device'}
		{#if deviceLevel}
			{@render deviceLevel()}
		{:else}
			<div class="rounded-xl border border-hair bg-surface shadow-plate">
				<NearPane
					rows={allWorstFirst}
					caption={m.fleet_near_caption({
						name: m.fleet_zoom_device(),
						count: devices.length
					})}
					{selected}
					collapseHealthy={false}
					onToggle={(i, shift) => toggle('__all__', allWorstFirst, i, shift)}
					onOpen={onOpenDevice}
					{openLabel}
				/>
			</div>
		{/if}
	{:else}
		<div
			class="grid gap-0 overflow-hidden rounded-xl border border-hair bg-surface {zoom === 'group'
				? 'md:grid-cols-[1fr_1.05fr]'
				: 'grid-cols-1'}"
		>
			<div
				data-tour="fleet-grid"
				class="space-y-2 bg-sunken p-3 {zoom === 'group' ? 'md:border-r' : ''}"
			>
				<div class="flex items-center justify-between font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint">
					<span>{m.fleet_far_caption()}</span>
					<span>{m.fleet_far_hint()}</span>
				</div>
				{#if loading && !snapshot}
					<p class="py-6 text-center text-sm text-muted-foreground">{m.common_loading()}</p>
				{/if}
				{#each bubbles as b (b.id)}
					<BubbleGroup
						bubble={b}
						{selected}
						focused={zoom === 'group' && focused?.id === b.id}
						onZoom={() => setZoom('group', b.id)}
						onToggle={(i, shift) => toggle(b.id, b.devices, i, shift)}
					/>
				{/each}
			</div>

			{#if zoom === 'group' && focused}
				<NearPane
					rows={focused.devices}
					caption={m.fleet_near_caption({ name: focused.name, count: focused.devices.length })}
					{selected}
					onToggle={(i, shift) => toggle(focused.id, focused.devices, i, shift)}
					onOpen={onOpenDevice}
					{openLabel}
				/>
			{/if}
		</div>
	{/if}

	{#if !isEmpty}

		<div data-tour="fleet-legend" class="flex flex-wrap gap-3.5 text-[0.7rem] text-muted-foreground">
			{#each LEGEND as l (l.tone)}
				<span class="inline-flex items-center gap-1.5">
					<span
						aria-hidden="true"
						class="h-2.5 w-2.5 rounded-[3px] {l.tone === 'idle'
							? 'border-[1.5px] border-dashed border-idle'
							: TONE_FILL[l.tone]}"
					></span>
					{l.label()}
				</span>
			{/each}
			<span class="inline-flex items-center gap-1.5">
				<span aria-hidden="true" class="h-2.5 w-2.5 rounded-[3px] bg-ok opacity-40"></span>
				{m.fleet_legend_decay()}
			</span>
		</div>
	{/if}
</PageShell>

<AlertDialog.Root bind:open={rebootOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>
				{m.bulk_reboot_confirm_title({ count: selectedDevices.length })}
			</AlertDialog.Title>
			<AlertDialog.Description>{m.bulk_reboot_confirm_description()}</AlertDialog.Description>
		</AlertDialog.Header>

		<ul data-testid="bulk-reboot-hosts" class="max-h-48 space-y-0.5 overflow-y-auto rounded-lg bg-sunken px-3 py-2 font-mono text-xs">
			{#each confirmHosts as d (d.id)}
				<li class="flex items-center gap-2">
					<span aria-hidden="true" class="h-2 w-2 shrink-0 rounded-[3px] {TONE_FILL[d.tone]}"></span>
					{d.hostname}
				</li>
			{/each}
			{#if confirmMore > 0}
				<li class="text-muted-foreground">{m.bulk_confirm_more({ count: confirmMore })}</li>
			{/if}
		</ul>

		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action data-testid="bulk-reboot-confirm" disabled={running} onclick={() => void runReboot()}>
				{running ? m.instant_actions_requesting() : m.instant_actions_reboot()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AddLabelDialog
	bind:open={labelOpen}
	title={m.bulk_label_dialog_title({ count: selectedDevices.length })}
	description={m.bulk_label_dialog_description()}
	confirmLabel={m.bulk_label_confirm({ count: selectedDevices.length })}
	onadd={(key, value) => void runLabel(key, value)}
/>
