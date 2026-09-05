<script lang="ts">
 import { DateRangePicker } from '$lib/components/ui/date-range-picker';
 import { dateCodec } from '$lib/components/ui/date-range-picker/date-codec';
 import { type DateValue, getLocalTimeZone } from '@internationalized/date';
 import { api } from '$lib/api';
 import { Permission, AuditActorType, AuditEventType, AuditStreamType, type AuditEvent } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { collectPages, formatDate as formatTimestampDateTime } from '$lib/console';
 import { registerPageSearch } from '$lib/shell/page-search.svelte';
 import { Button } from '$lib/components/ui/button';
 import { Input } from '$lib/components/ui/input';
 import * as Sheet from '$lib/components/ui/sheet';
 import { Badge } from '$lib/components/ui/badge';
 import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
 import PageShell from '$lib/components/page-shell.svelte';
 import { RowList, DataTablePagination, createClientListState } from '$lib/components/data-table';
 import { ClipboardList, RefreshCw } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
 import { codecs } from '$lib/url-state';
 const { can } = consoleContext();
 let detailEvent = $state<AuditEvent | null>(null);
 type SortKey = 'timestamp' | 'actor' | 'event_type' | 'stream_type';
 const table = createClientListState<AuditEvent, SortKey, { stream: string[]; actor: string; occurredStart: DateValue | undefined; occurredEnd: DateValue | undefined }>({
  load: () => can(Permission.LIST_AUDIT_EVENTS) ? collectPages(async pageToken => { const r = await api.listAuditEvents({ pageSize: 100, pageToken }); return { items: r.events, nextPageToken: r.nextPageToken }; }) : Promise.resolve([]),
  searchFields: event => [AuditEventType[event.eventType], event.actorId?.value, event.streamId?.value], sortKeys: ['timestamp', 'actor', 'event_type', 'stream_type'], defaultSort: 'timestamp', sortDir: key => key === 'timestamp' ? 'desc' : 'asc',
  sortComparators: { timestamp: (a,b) => Number((a.occurredAt?.seconds ?? 0n)-(b.occurredAt?.seconds ?? 0n)), actor: (a,b) => a.actorType-b.actorType, event_type: (a,b) => a.eventType-b.eventType, stream_type: (a,b) => a.streamType-b.streamType },
  filters: { stream: { key: 'stream', codec: codecs.stringArray([]) }, actor: { key: 'actor', codec: codecs.string('') }, occurredStart: { key: 'occurredStart', codec: dateCodec }, occurredEnd: { key: 'occurredEnd', codec: dateCodec } },
  filterRow: (event, filters) => (!filters.stream.length || filters.stream.includes(String(event.streamType))) && (!filters.actor || event.actorId?.value === filters.actor) && (!filters.occurredStart || Number(event.occurredAt?.seconds ?? 0n)*1000 >= filters.occurredStart.toDate(getLocalTimeZone()).getTime()) && (!filters.occurredEnd || Number(event.occurredAt?.seconds ?? 0n)*1000 < filters.occurredEnd.add({ days: 1 }).toDate(getLocalTimeZone()).getTime())
 });
 const streamTypeFilterItems = Object.values(AuditStreamType).filter((value): value is AuditStreamType => typeof value === 'number' && value > 0).map(value => ({ id: String(value), label: AuditStreamType[value].replaceAll('_', ' ') }));
 const sortOptions = [{ key: 'timestamp' as const, label: m.audit_table_timestamp() }, { key: 'actor' as const, label: m.audit_table_actor() }, { key: 'event_type' as const, label: m.audit_table_event_type() }, { key: 'stream_type' as const, label: m.audit_table_target() }];
 function getActorLabel(type: AuditActorType, id: string) { return `${AuditActorType[type]} · ${id}`; }
 function getStreamTypeLabel(type: AuditStreamType) { return AuditStreamType[type].replaceAll('_', ' '); }
 function formatEventType(type: AuditEventType) { return AuditEventType[type].replaceAll('_', ' '); }
 function getTargetLabel(event: AuditEvent) { return event.streamId?.value ?? '—'; }
 $effect(() => registerPageSearch({ scope: null, label: m.audit_title, get query() { return table.query; }, setQuery: table.setSearch, clear: () => table.setSearch('') }));
</script>
<PageShell contentClass="space-y-4">

	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.audit_title()}</h1>
				<p class="text-muted-foreground">{m.audit_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">

				<Button onclick={() => table.refresh()} variant="outline" disabled={table.loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={table.loading}>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	<div data-tour="audit-table">
	<RowList {table} {sortOptions} rowKey={(e) => (e.id?.value ?? '')}>
		{#snippet filters()}
			<MultiSelectCombobox
				items={streamTypeFilterItems}
				selected={table.filters.stream}
				onSelectedChange={(next) => table.setFilter('stream', next)}
				placeholder={m.audit_filter_stream_type()}
				searchPlaceholder={m.common_search()}
				class="w-48"
			/>
            <Input aria-label="Actor ID" placeholder="Actor ID" value={table.filters.actor} oninput={event => table.setFilter('actor', event.currentTarget.value)} class="w-48" />
			<DateRangePicker start={table.filters.occurredStart} end={table.filters.occurredEnd} onChange={({start, end}) => { table.setFilter('occurredStart', start); table.setFilter('occurredEnd', end); }} placeholder="Occurred date" />
		{/snippet}
		{#snippet row(event)}
			<button
				type="button"
				data-testid="audit-row-open"
				onclick={() => (detailEvent = event)}
				class="flex min-w-0 flex-1 items-center gap-3 rounded-[10px] text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
			>
				<span class="shrink-0 font-mono text-xs tabular-nums whitespace-nowrap text-muted-foreground">
					{formatTimestampDateTime(event.occurredAt)}
				</span>
				<span class="w-40 shrink-0 truncate text-xs text-muted-foreground">
					{getActorLabel(event.actorType, (event.actorId?.value ?? ''))}
				</span>
				<span class="min-w-0 flex-1">
					<span class="block truncate text-sm font-medium">{formatEventType(event.eventType)}</span>
					<span class="block truncate font-mono text-[0.66rem] text-faint">{event.id?.value ?? ''}</span>
				</span>
				<span class="flex shrink-0 items-center gap-1.5">
					<Badge variant="outline">{getStreamTypeLabel(event.streamType)}</Badge>
					<span class="max-w-40 truncate font-mono text-xs">{getTargetLabel(event)}</span>
				</span>

				<span class="shrink-0" title={m.audit_table_outcome()}>

				</span>
			</button>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<ClipboardList class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.audit_empty()}</h3>
				<p class="text-muted-foreground">
					{table.query ||
					table.filters.stream.length > 0 ||
					table.filters.actor ||
					table.filters.occurredStart ||
					table.filters.occurredEnd
						? m.common_try_different_search()
						: m.audit_empty_hint()}
				</p>
			</div>
		{/snippet}
	</RowList>
	</div>

	<DataTablePagination {table} />
</PageShell>

<Sheet.Root open={!!detailEvent} onOpenChange={(open) => !open && (detailEvent = null)}>
	<Sheet.Content class="sm:max-w-2xl w-full flex flex-col">
		{#if detailEvent}
			{@const event = detailEvent}
			<Sheet.Header>
				<Sheet.Title>{formatEventType(event.eventType)}</Sheet.Title>
				<Sheet.Description class="text-xs">{event.id?.value ?? ''}</Sheet.Description>
			</Sheet.Header>

			<div class="flex-1 overflow-y-auto px-4 pb-6 space-y-4">

				<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
					<h4 class="text-xs font-semibold uppercase tracking-wide text-faint">
						{m.audit_operation_label()}
					</h4>
					<div class="mt-2 flex flex-wrap items-center gap-2">

						<span class="font-medium">{formatEventType(event.eventType)}</span>
						<span class="ml-auto font-mono text-xs text-muted-foreground">
							{getActorLabel(event.actorType, (event.actorId?.value ?? ''))} · {formatTimestampDateTime(
								event.occurredAt
							)}
						</span>
					</div>
					<p class="mt-2 font-mono text-xs text-faint">{event.id?.value ?? ''}</p>
				</section>

				<section>
					<h4 class="text-xs font-semibold uppercase tracking-wide text-faint">
						{m.audit_effects_label()}
					</h4>
					<ul class="mt-2 divide-y divide-hair rounded-xl border border-hair bg-surface">
						<li class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2.5 text-sm">
							<Badge variant="outline">{getStreamTypeLabel(event.streamType)}</Badge>
							<span class="truncate font-mono text-xs">{getTargetLabel(event)}</span>
							<span class="font-mono text-xs text-muted-foreground">
								{formatEventType(event.eventType)}
							</span>
							<span class="ml-auto">

							</span>
						</li>
					</ul>
					<p class="mt-2 text-xs text-muted-foreground">{m.audit_effects_note()}</p>
				</section>

			</div>
		{/if}
	</Sheet.Content>
</Sheet.Root>
