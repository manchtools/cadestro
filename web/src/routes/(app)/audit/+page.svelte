<script lang="ts">
	import { onMount } from 'svelte';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		fetchAllPages,
		type AuditEvent,
		type User,
		type Device,
		formatTimestampDateTime
	} from '$lib/sdk';
	import { SearchScope, SortField } from '$sdk/powermanage/v1/common_pb';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import { auditEventSummary, auditEventOutcome } from '$lib/audit-summaries';
	import { Button } from '$lib/components/ui/button';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Badge } from '$lib/components/ui/badge';
	import { Chip } from '$lib/components/fleet';
	import * as Select from '$lib/components/ui/select';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
	import PageShell from '$lib/components/page-shell.svelte';
	import {
		RowList,
		DataTablePagination,
		createSearchListState,
		type SearchDateFilter
	} from '$lib/components/data-table';
	import { DateRangePicker } from '$lib/components/ui/date-range-picker';
	import { type DateValue, getLocalTimeZone, parseDate } from '@internationalized/date';
	import { ClipboardList, RefreshCw, Download } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { codecs, type Codec } from '$lib/url-state';
	import { searchResultToAuditEvent } from '$lib/search-adapters';

	// This is an evidence viewer: the raw payload is rendered exactly as the
	// server returned it (already redacted server-side) — no field is
	// reinterpreted and no secret-bearing column is synthesised here.
	//
	// The list is the shared row grammar (RowList): one dense row per event, no
	// column headers. The evidence panel stays a Sheet — it shows the same fields
	// the row summarises plus the raw payload; no payload was dropped.
	let users = $state<Map<string, User>>(new Map());
	let devices = $state<Map<string, Device>>(new Map());
	let detailEvent = $state<AuditEvent | null>(null);
	let exporting = $state(false);

	type SortKey = 'timestamp' | 'actor' | 'event_type' | 'stream_type';
	type Filters = {
		stream: string[];
		actor: string;
		occurredStart: DateValue | undefined;
		occurredEnd: DateValue | undefined;
	};

	// Dates serialize as ISO YYYY-MM-DD; a bad value falls back to undefined
	// rather than crashing the page.
	const DATE_CODEC: Codec<DateValue | undefined> = {
		parse: (p) => {
			if (!p) return undefined;
			try {
				return parseDate(p);
			} catch (err) {
				console.warn(err);
				return undefined;
			}
		},
		serialize: (v) => (v ? v.toString() : null)
	};

	/** occurred_at range → the server's only interval channel (tags are exact-value). */
	function occurredRange(f: Filters): SearchDateFilter[] | undefined {
		if (!f.occurredStart && !f.occurredEnd) return undefined;
		const tz = getLocalTimeZone();
		return [
			{
				field: 'occurred_at',
				start: f.occurredStart
					? BigInt(Math.floor(f.occurredStart.toDate(tz).getTime() / 1000))
					: 0n,
				// Inclusive end-of-day: the selected day plus one.
				end: f.occurredEnd
					? BigInt(Math.floor(f.occurredEnd.toDate(tz).getTime() / 1000) + 86400)
					: 0n
			}
		];
	}

	const table = createSearchListState<AuditEvent, SortKey, Filters>({
		scope: SearchScope.AUDIT_EVENTS,
		adapter: searchResultToAuditEvent,
		sortKeys: ['timestamp', 'actor', 'event_type', 'stream_type'],
		defaultSort: 'timestamp',
		// The "actor" column sorts by actor_type. Subset of the server's
		// scopeSortableFields["audit_events"].
		sortFieldMap: {
			timestamp: SortField.OCCURRED_AT,
			actor: SortField.ACTOR_TYPE,
			event_type: SortField.EVENT_TYPE,
			stream_type: SortField.STREAM_TYPE
		},
		// Timestamps read newest-first, both on a bare link and when switched to.
		defaultSortDir: 'desc',
		sortDir: (key) => (key === 'timestamp' ? 'desc' : 'asc'),
		filters: {
			stream: { key: 'stream', codec: codecs.stringArray([]) },
			actor: { key: 'actor', codec: codecs.string('') },
			occurredStart: { key: 'occurredStart', codec: DATE_CODEC },
			occurredEnd: { key: 'occurredEnd', codec: DATE_CODEC }
		},
		filterToTags: (f) => {
			const tags: Record<string, string> = {};
			if (f.stream.length > 0) tags.stream_type = f.stream.join('|');
			if (f.actor) tags.actor_id = f.actor;
			return tags;
		},
		dateFilters: occurredRange
	});

	const streamTypeFilterItems = [
		{ id: 'user', label: m.audit_stream_type_user() },
		{ id: 'device', label: m.audit_stream_type_device() },
		{ id: 'action', label: m.audit_stream_type_action() },
		{ id: 'action_set', label: m.audit_stream_type_action_set() },
		{ id: 'definition', label: m.audit_stream_type_definition() },
		{ id: 'device_group', label: m.audit_stream_type_device_group() },
		{ id: 'token', label: m.audit_stream_type_token() },
		{ id: 'execution', label: m.audit_stream_type_execution() },
		{ id: 'assignment', label: m.audit_stream_type_assignment() },
		{ id: 'user_selection', label: m.audit_stream_type_user_selection() },
		{ id: 'role', label: m.audit_stream_type_role() },
		{ id: 'user_role', label: m.audit_stream_type_user_role() },
		{ id: 'user_group', label: m.audit_stream_type_user_group() },
		{ id: 'identity_provider', label: m.audit_stream_type_identity_provider() },
		{ id: 'scim_group_mapping', label: m.audit_stream_type_scim_group_mapping() },
		{ id: 'compliance', label: m.audit_stream_type_compliance() },
		{ id: 'compliance_policy', label: m.audit_stream_type_compliance_policy() },
		{ id: 'server_settings', label: m.audit_stream_type_server_settings() },
		{ id: 'lps_password', label: m.audit_stream_type_lps_password() },
		{ id: 'luks_key', label: m.audit_stream_type_luks_key() }
	];

	// Headerless rows: the sort keys that were column headers now ride the row
	// list's sort bar, reusing the same labels. The server-sortable set is
	// unchanged — "target" still sorts by stream_type, as its column header did.
	const sortOptions = [
		{ key: 'timestamp' as const, label: m.audit_table_timestamp() },
		{ key: 'actor' as const, label: m.audit_table_actor() },
		{ key: 'event_type' as const, label: m.audit_table_event_type() },
		{ key: 'stream_type' as const, label: m.audit_table_target() }
	];

	// Both bounds belong to one operator gesture: seed the start directly and let
	// the end's setFilter do the single page-reset + URL commit for the pair.
	function onDateRangeChange(v: { start?: DateValue; end?: DateValue }) {
		table.filters.occurredStart = v.start;
		table.setFilter('occurredEnd', v.end);
	}

	function onActorChange(next: string) {
		table.setFilter('actor', next === 'all' ? '' : next);
	}

	onMount(() => {
		loadUsers();
		loadDevices();
	});

	// The actor dropdown and the device/user target labels are resolved from
	// these two best-effort lookups; a failure degrades to short ids.
	async function loadUsers() {
		try {
			const response = await apiClient.listUsers();
			const map = new Map<string, User>();
			for (const user of response.users) {
				map.set(user.id, user);
			}
			users = map;
		} catch (error) {
			console.error('Failed to load users', error);
		}
	}

	async function loadDevices() {
		try {
			const all = await fetchAllPages<Device>(async (size, token) => {
				const r = await apiClient.listDevices(size, token);
				return { items: r.devices, nextPageToken: r.nextPageToken };
			});
			const map = new Map<string, Device>();
			for (const device of all) {
				map.set(device.id, device);
			}
			devices = map;
		} catch (error) {
			console.error('Failed to load devices', error);
		}
	}

	function getActorLabel(actorType: string, actorId: string): string {
		if (actorType === 'user' && actorId) {
			const user = users.get(actorId);
			return user?.email ?? actorId.slice(0, 8) + '...';
		}
		if (actorType === 'system') return 'System';
		if (actorType === 'agent') return 'Agent';
		if (!actorType && !actorId) return '-';
		return `${actorType}: ${actorId.slice(0, 8)}...`;
	}

	function getStreamTypeLabel(streamType: string): string {
		const item = streamTypeFilterItems.find((i) => i.id === streamType);
		return item?.label ?? streamType;
	}

	function deviceLabel(id: string): string {
		if (!id) return '-';
		return devices.get(id)?.hostname ?? id.slice(0, 8) + '...';
	}

	// Target: the stream the event acted on, resolved to a human label where the
	// id is a device or user.
	function getTargetLabel(event: AuditEvent): string {
		if (!event.streamId) return '-';
		if (event.streamType === 'device') return deviceLabel(event.streamId);
		if (event.streamType === 'user') {
			return users.get(event.streamId)?.email ?? event.streamId.slice(0, 8) + '...';
		}
		return event.streamId.slice(0, 8) + '...';
	}

	function getSummary(event: AuditEvent): string | null {
		return auditEventSummary(event, { deviceName: deviceLabel });
	}

	function formatEventType(eventType: string): string {
		return eventType.replace(/([A-Z])/g, ' $1').trim();
	}

	// Export: server-side chunked export with the SAME filters the view applies.
	// The free-text search box maps to the export's event-type substring filter;
	// the server applies the ListAuditEvents redaction and permission gate.
	async function exportAudit(format: 'csv' | 'json') {
		if (exporting) return;
		exporting = true;
		try {
			const tz = getLocalTimeZone();
			const { stream, actor, occurredStart, occurredEnd } = table.filters;
			const chunks: Uint8Array[] = [];
			let token = '';
			do {
				const resp = await apiClient.exportAuditEvents({
					format,
					actorId: actor || undefined,
					streamTypes: stream.length > 0 ? stream : undefined,
					eventType: table.query.trim() || undefined,
					occurredFrom: occurredStart ? timestampFromDate(occurredStart.toDate(tz)) : undefined,
					// Mirror the list view's inclusive end-of-day bound.
					occurredTo: occurredEnd
						? timestampFromDate(new Date(occurredEnd.toDate(tz).getTime() + 86_400_000))
						: undefined,
					pageToken: token || undefined
				});
				chunks.push(resp.chunk);
				token = resp.nextPageToken;
			} while (token !== '');

			const blob = new Blob(chunks as BlobPart[], {
				type: format === 'csv' ? 'text/csv' : 'application/json'
			});
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `audit-log-${new Date().toISOString().slice(0, 10)}.${format}`;
			a.click();
			URL.revokeObjectURL(url);
			toast.success(m.audit_export_done());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error('audit export failed', error);
		} finally {
			exporting = false;
		}
	}

	function formatData(data: string): string {
		try {
			const parsed = JSON.parse(data);
			return JSON.stringify(parsed, null, 2);
		} catch (err) {
			console.warn('audit data JSON parse failed', err);
			return data;
		}
	}

	// The query lives in the pill now: ⌘K opens search already on this facet and
	// its keystrokes land on the same setSearch the removed input drove. The
	// registration is withdrawn on unmount so the next page never inherits it.
	$effect(() =>
		registerPageSearch({
			scope: SearchScope.AUDIT_EVENTS,
			label: m.nav_audit_short,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);
</script>

<PageShell contentClass="space-y-4">
	<!-- The header band keeps only what acts on the page itself — the export runs
	     over the whole filtered set, so it belongs here, not on the list's bar. The
	     search box is gone — ⌘K is the search, already scoped to this page. -->
	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.audit_title()}</h1>
				<p class="text-muted-foreground">{m.audit_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">
				<!-- The list's filters ride IN the list's own toolbar, next to sort:
				     narrowing a list is one act, so it reads as one bar. The page band
				     keeps only what acts on the page itself. -->
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button {...props} variant="outline" disabled={exporting}>
								<Download class="mr-2 h-4 w-4" />
								{exporting ? m.audit_export_running() : m.audit_export()}
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end">
						<DropdownMenu.Item onclick={() => exportAudit('csv')}>
							{m.audit_export_csv()}
						</DropdownMenu.Item>
						<DropdownMenu.Item onclick={() => exportAudit('json')}>
							{m.audit_export_json()}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
				<Button onclick={() => table.refresh()} variant="outline" disabled={table.loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={table.loading}>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	<!-- Evidence density is deliberate: the list keeps every filter, sort key and
	     export it had; only the ink changed — mono for machine-issued strings
	     (ULIDs, timestamps), status chips for the outcome.

	     An audit row navigates nowhere — there is no /audit/[id] route — so this
	     list passes no `href`. The row body is a real <button> instead: it opens
	     the evidence Sheet with the native click/Enter/Space and focus semantics
	     a div-with-role would have to re-implement, and its accessible name is
	     the row's own evidence. -->
	<div data-tour="audit-table">
	<RowList {table} {sortOptions} rowKey={(e) => e.id}>
		{#snippet filters()}
			<MultiSelectCombobox
				items={streamTypeFilterItems}
				selected={table.filters.stream}
				onSelectedChange={(next) => table.setFilter('stream', next)}
				placeholder={m.audit_filter_stream_type()}
				searchPlaceholder={m.common_search()}
				class="w-48"
			/>
			<Select.Root
				type="single"
				value={table.filters.actor || 'all'}
				onValueChange={(v) => v && onActorChange(v)}
			>
				<Select.Trigger class="w-48">
					{table.filters.actor
						? (users.get(table.filters.actor)?.email ?? table.filters.actor.slice(0, 8) + '...')
						: m.audit_filter_actor()}
				</Select.Trigger>
				<Select.Content>
					<Select.Item value="all">{m.audit_filter_actor()}</Select.Item>
					{#each [...users.values()] as user (user.id)}
						<Select.Item value={user.id}>{user.email}</Select.Item>
					{/each}
				</Select.Content>
			</Select.Root>
			<DateRangePicker
				start={table.filters.occurredStart}
				end={table.filters.occurredEnd}
				onChange={onDateRangeChange}
				placeholder={m.common_date_filter()}
				class="w-52"
			/>
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
					{getActorLabel(event.actorType, event.actorId)}
				</span>
				<span class="min-w-0 flex-1">
					<span class="block truncate text-sm font-medium">{formatEventType(event.eventType)}</span>
					<span class="block truncate font-mono text-[0.66rem] text-faint">{event.id}</span>
				</span>
				<span class="flex shrink-0 items-center gap-1.5">
					<Badge variant="outline">{getStreamTypeLabel(event.streamType)}</Badge>
					<span class="max-w-40 truncate font-mono text-xs">{getTargetLabel(event)}</span>
				</span>
				<!-- Headerless rows lose their column labels, so the chip carries its
				     former header as a tooltip. -->
				<span class="shrink-0" title={m.audit_table_outcome()}>
					{#if auditEventOutcome(event.eventType) === 'denied'}
						<Chip tone="crit" label={m.audit_outcome_denied()} />
					{:else}
						<Chip tone="ok" label={m.audit_outcome_success()} />
					{/if}
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
				<Sheet.Description class="text-xs">{event.id}</Sheet.Description>
			</Sheet.Header>

			<div class="flex-1 overflow-y-auto px-4 pb-6 space-y-4">
				<!-- Movement F's grammar, applied to evidence: an operation summary
				     over its effect rows. Everything below is a field the audit
				     contract actually returns (AuditEvent: id, event_type,
				     stream_type, stream_id, actor_type, actor_id, data,
				     occurred_at) — nothing is reconstructed. -->
				<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
					<h4 class="text-xs font-semibold uppercase tracking-wide text-faint">
						{m.audit_operation_label()}
					</h4>
					<div class="mt-2 flex flex-wrap items-center gap-2">
						{#if auditEventOutcome(event.eventType) === 'denied'}
							<Chip tone="crit" label={m.audit_outcome_denied()} />
						{:else}
							<Chip tone="ok" label={m.audit_outcome_success()} />
						{/if}
						<span class="font-medium">{formatEventType(event.eventType)}</span>
						<span class="ml-auto font-mono text-xs text-muted-foreground">
							{getActorLabel(event.actorType, event.actorId)} · {formatTimestampDateTime(
								event.occurredAt
							)}
						</span>
					</div>
					<p class="mt-2 font-mono text-xs text-faint">{event.id}</p>
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
								{#if auditEventOutcome(event.eventType) === 'denied'}
									<Chip tone="crit" label={m.audit_outcome_denied()} />
								{:else}
									<Chip tone="ok" label={m.audit_outcome_success()} />
								{/if}
							</span>
						</li>
					</ul>
					<p class="mt-2 text-xs text-muted-foreground">{m.audit_effects_note()}</p>
				</section>

				<div>
					<h4 class="text-sm font-medium mb-2">{m.audit_details()}</h4>
					{#if getSummary(event)}
						<p class="text-sm mb-2">{getSummary(event)}</p>
					{/if}
					{#if event.data && event.data !== '{}'}
						<pre
							class="text-xs font-mono whitespace-pre-wrap break-all bg-muted p-3 rounded border max-h-96 overflow-auto">{formatData(
								event.data
							)}</pre>
					{:else}
						<p class="text-sm text-muted-foreground">{m.audit_no_data()}</p>
					{/if}
				</div>
			</div>
		{/if}
	</Sheet.Content>
</Sheet.Root>
