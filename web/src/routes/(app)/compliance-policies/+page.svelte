<script lang="ts">
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		fetchAllPages,
		type CompliancePolicy,
		type Device,
		type SearchResult
	} from '$lib/sdk';
	import { ComplianceStatus, SearchScope, SortField } from '$contract/cadestro/v1/common_pb';
	import { TONE_FILL } from '$lib/components/fleet';
	import CompliancePolicyDetailSheet, {
		openCompliancePolicySheet
	} from '$lib/components/compliance-policy-detail-sheet.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Chip } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import {
		RowList,
		DataTablePagination,
		createSearchListState,
		type SearchDateFilter
	} from '$lib/components/data-table';
	import {
		ShieldCheck,
		Plus,
		MoreHorizontal,
		RefreshCw,
		Trash2,
		ExternalLink
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { DateRangePicker } from '$lib/components/ui/date-range-picker';
	import { type DateValue, getLocalTimeZone, parseDate } from '@internationalized/date';
	import { codecs, type Codec } from '$lib/url-state';
	import { searchResultToCompliancePolicy } from '$lib/search-adapters';

	type PolicyRow = CompliancePolicy & { indexedRuleCount: number; createdAtSeconds: number };

	function intOr(raw: string | undefined, fallback: number): number {
		const n = parseInt(raw ?? '', 10);
		return Number.isFinite(n) ? n : fallback;
	}

	function toRow(r: SearchResult): PolicyRow {
		const policy = searchResultToCompliancePolicy(r);
		return {
			...policy,
			indexedRuleCount: intOr(r.fields['rule_count'], policy.ruleCount),
			createdAtSeconds: intOr(r.fields['created_at'], 0)
		};
	}

	type SortKey = 'name' | 'rules' | 'created';

	type Zoom = 'overview' | 'list';
	const ZOOMS = ['overview', 'list'] as const;
	const ZOOM_LABEL: Record<Zoom, () => string> = { overview: m.zoom_overview, list: m.zoom_list };
	const ZOOM_CODEC = codecs.enum<Zoom>(ZOOMS, 'overview');

	type Filters = {
		zoom: Zoom;
		noRules: boolean;
		createdStart: DateValue | undefined;
		createdEnd: DateValue | undefined;
	};

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

	function createdRange(f: Filters): SearchDateFilter[] | undefined {
		if (!f.createdStart && !f.createdEnd) return undefined;
		const tz = getLocalTimeZone();
		return [
			{
				field: 'created_at',
				start: f.createdStart ? BigInt(Math.floor(f.createdStart.toDate(tz).getTime() / 1000)) : 0n,

				end: f.createdEnd ? BigInt(Math.floor(f.createdEnd.toDate(tz).getTime() / 1000) + 86400) : 0n
			}
		];
	}

	const table = createSearchListState<PolicyRow, SortKey, Filters>({
		scope: SearchScope.COMPLIANCE_POLICIES,
		adapter: toRow,
		sortKeys: ['name', 'rules', 'created'],
		defaultSort: 'created',

		sortFieldMap: {
			name: SortField.NAME,
			rules: SortField.RULE_COUNT,
			created: SortField.CREATED_AT
		},

		defaultSortDir: 'desc',
		sortDir: (key) => (key === 'created' ? 'desc' : 'asc'),
		filters: {
			zoom: { key: 'zoom', codec: ZOOM_CODEC },
			noRules: { key: 'noRules', codec: codecs.bool(false) },
			createdStart: { key: 'createdStart', codec: DATE_CODEC },
			createdEnd: { key: 'createdEnd', codec: DATE_CODEC }
		},

		filterToTags: (f) => (f.noRules ? { rule_count: '0' } : undefined),
		dateFilters: createdRange,

		paused: (f) => f.zoom !== 'list'
	});

	let overviewDevices = $state<Device[] | null>(null);
	let overviewPolicies = $state<CompliancePolicy[]>([]);
	let sweeping = $state(false);
	let sweepError = $state<string | null>(null);

	let swept = false;

	async function sweep() {
		swept = true;
		sweeping = true;
		sweepError = null;
		try {
			const [devices, policies] = await Promise.all([
				fetchAllPages<Device>(async (size, token) => {
					const resp = await apiClient.listDevices(size, token);
					return { items: resp.devices, nextPageToken: resp.nextPageToken };
				}),
				fetchAllPages<CompliancePolicy>(async (size, token) => {
					const resp = await apiClient.listCompliancePolicies(size, token);
					return { items: resp.policies, nextPageToken: resp.nextPageToken };
				})
			]);
			overviewDevices = devices;
			overviewPolicies = policies;
		} catch (err) {
			sweepError = getLocalizedError(err);
			console.error(err);
		} finally {
			sweeping = false;
		}
	}

	$effect(() => {
		if (table.filters.zoom !== 'overview' || swept) return;
		void sweep();
	});

	function refresh() {
		if (table.filters.zoom === 'overview') void sweep();
		else table.refresh();
	}

	function complianceWord(status: ComplianceStatus): string {
		switch (status) {
			case ComplianceStatus.COMPLIANT:
				return m.compliance_status_compliant();
			case ComplianceStatus.NON_COMPLIANT:
				return m.compliance_status_non_compliant();
			case ComplianceStatus.IN_GRACE_PERIOD:
				return m.compliance_status_in_grace_period();
			default:
				return m.compliance_status_unknown();
		}
	}

	function complianceTone(status: ComplianceStatus): 'ok' | 'warn' | 'idle' {
		if (status === ComplianceStatus.COMPLIANT) return 'ok';
		if (status === ComplianceStatus.NON_COMPLIANT || status === ComplianceStatus.IN_GRACE_PERIOD)
			return 'warn';
		return 'idle';
	}

	let deleteDialogOpen = $state(false);
	let policyToDelete = $state<PolicyRow | null>(null);

	const sortOptions = [
		{ key: 'name' as const, label: m.actions_table_name() },
		{ key: 'rules' as const, label: m.compliance_policies_table_rules() },
		{ key: 'created' as const, label: m.actions_table_created() }
	];

	const anyFilterActive = $derived(
		table.query.length > 0 ||
			table.filters.noRules ||
			!!table.filters.createdStart ||
			!!table.filters.createdEnd
	);

	function onDateRangeChange(v: { start?: DateValue; end?: DateValue }) {
		table.filters.createdStart = v.start;
		table.setFilter('createdEnd', v.end);
	}

	function createdLabel(policy: PolicyRow): string {
		if (policy.createdAtSeconds <= 0) return m.common_unknown();
		return new Date(policy.createdAtSeconds * 1000).toLocaleDateString();
	}

	function confirmDelete(policy: PolicyRow) {
		policyToDelete = policy;
		deleteDialogOpen = true;
	}

	async function deletePolicy() {
		if (!policyToDelete) return;
		try {
			await apiClient.deleteCompliancePolicy((policyToDelete.id?.value ?? ''));
			toast.success(m.compliance_policies_deleted());
			table.patchRows((rows) => rows.filter((p) => p.id !== policyToDelete!.id));
			table.refresh();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			policyToDelete = null;
		}
	}

	$effect(() =>
		registerPageSearch({
			scope: SearchScope.COMPLIANCE_POLICIES,
			label: m.nav_compliance_short,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);
</script>

<PageShell contentClass="space-y-4">

	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.compliance_policies_title()}</h1>
				<p class="text-muted-foreground">{m.compliance_policies_subtitle()}</p>
			</div>
			<div
				role="group"
				aria-label={m.zoom_label()}
				class="inline-flex overflow-hidden rounded-lg border font-mono text-[0.68rem]"
			>
				{#each ZOOMS as z (z)}
					<button
						type="button"
						data-testid="compliance-zoom-{z}"
						aria-pressed={table.filters.zoom === z}
						onclick={() => table.setFilter('zoom', z)}
						class="border-r px-2.5 py-1 last:border-r-0 {table.filters.zoom === z
							? 'bg-accent-soft font-semibold text-accent-ink'
							: 'text-muted-foreground hover:text-foreground'}"
					>
						{ZOOM_LABEL[z]()}
					</button>
				{/each}
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">

				<Button
					onclick={refresh}
					variant="outline"
					disabled={table.filters.zoom === 'overview' ? sweeping : table.loading}
				>
					<span
						class="mr-2 h-4 w-4"
						class:animate-spin={table.filters.zoom === 'overview' ? sweeping : table.loading}
					>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
				<Button onclick={() => goto('/compliance-policies/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.compliance_policies_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	{#if table.filters.zoom === 'overview'}
		{#if sweepError}
			<div class="rounded-xl border border-crit/50 bg-crit-soft p-4 text-sm text-crit">
				{sweepError}
			</div>
		{:else if overviewDevices !== null && overviewDevices.length === 0}
			<div
				class="flex flex-col items-center justify-center rounded-xl border bg-surface px-6 py-12 text-center"
			>
				<ShieldCheck class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.fleet_empty_title()}</h3>
				<p class="text-muted-foreground">{m.fleet_empty_hint()}</p>
			</div>
		{:else}
			<div data-testid="compliance-overview" class="space-y-2 rounded-xl border bg-sunken p-3">
				<div class="font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint">
					{m.compliance_overview_caption()}
				</div>
				{#if overviewPolicies.length > 0}

					<div role="group" aria-label={m.compliance_overview_policies()} class="flex flex-wrap gap-1.5">
						{#each overviewPolicies as policy (policy.id)}
							<button
								type="button"
								data-testid="compliance-policy-chip"
								onclick={() => openCompliancePolicySheet((policy.id?.value ?? ''))}
								class="inline-flex items-center gap-1.5 rounded-full border bg-surface px-2.5 py-1 text-xs hover:border-border-strong focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
							>
								<ShieldCheck class="h-3 w-3 text-accent-ink" />
								<span class="font-medium">{policy.name}</span>
								<span class="font-mono text-[0.66rem] text-faint">
									{m.compliance_overview_policy_meta({ count: policy.ruleCount })}
								</span>
							</button>
						{/each}
					</div>
				{/if}
				<div class="grid grid-cols-[repeat(auto-fill,minmax(14px,1fr))] gap-[3px]">
					{#each overviewDevices ?? [] as device (device.id)}
						{@const tone = complianceTone(device.complianceStatus)}
						<button
							type="button"
							data-testid="compliance-tile"
							data-device-id={device.id}
							data-tone={tone}
							aria-label="{device.hostname} · {complianceWord(device.complianceStatus)}"
							title="{device.hostname} · {complianceWord(device.complianceStatus)}"
							onclick={() => goto(`/devices/${device.id}?tab=compliance`)}
							class="relative block aspect-square w-full min-w-[14px] rounded-[4px] p-0 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring {tone ===
							'idle'
								? 'border-[1.5px] border-dashed border-idle bg-transparent'
								: TONE_FILL[tone] + ' border-0'}"
						>
							{#if tone === 'warn'}

								<span
									data-marker="dot"
									class="pointer-events-none absolute bottom-[3px] left-[3px] h-1 w-1 rounded-full bg-marker"
								></span>
							{/if}
						</button>
					{/each}
				</div>
			</div>
		{/if}
	{:else}

	<RowList {table} {sortOptions} rowKey={(p) => (p.id?.value ?? '')}>
		{#snippet filters()}
			<DateRangePicker
				start={table.filters.createdStart}
				end={table.filters.createdEnd}
				onChange={onDateRangeChange}
				placeholder={m.common_date_filter_created()}
				class="w-52"
			/>
			<label class="flex select-none items-center gap-2 text-sm">
				<Checkbox
					checked={table.filters.noRules}
					onCheckedChange={(checked) => table.setFilter('noRules', checked === true)}
				/>
				{m.compliance_policies_filter_no_rules()}
			</label>
		{/snippet}
		{#snippet row(policy)}
			<button
				type="button"
				data-testid="compliance-policy-open"
				onclick={() => openCompliancePolicySheet((policy.id?.value ?? ''))}
				class="flex min-w-0 flex-1 items-center gap-3 rounded-[10px] text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
			>
				<span class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
					<ShieldCheck class="h-3.5 w-3.5 text-accent-ink" />
				</span>
				<span class="min-w-0 flex-1">
					<span class="block truncate text-sm font-semibold">{policy.name}</span>
					<span class="flex min-w-0 items-baseline gap-2">
						<span class="shrink-0 font-mono text-[0.66rem] text-faint">{policy.id}</span>
						<span class="truncate text-xs text-muted-foreground">
							{policy.description || m.common_no_description()}
						</span>
					</span>
				</span>
				<Chip
					tone={policy.indexedRuleCount > 0 ? 'info' : 'idle'}
					label={m.compliance_policies_rule_count({ count: policy.indexedRuleCount })}
				/>
				<span class="shrink-0 font-mono text-xs tabular-nums text-muted-foreground">
					{createdLabel(policy)}
				</span>
			</button>
		{/snippet}

		{#snippet rowEnd(policy)}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
							<MoreHorizontal class="h-4 w-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					<DropdownMenu.Item onclick={() => openCompliancePolicySheet((policy.id?.value ?? ''))}>
						<ExternalLink class="mr-2 h-4 w-4" />
						{m.compliance_policy_open()}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item onclick={() => confirmDelete(policy)} class="text-destructive">
						<Trash2 class="mr-2 h-4 w-4" />
						{m.common_delete()}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<ShieldCheck class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.compliance_policies_empty()}</h3>
				<p class="text-muted-foreground">
					{anyFilterActive ? m.common_try_different_search() : m.compliance_policies_empty_hint()}
				</p>
				{#if !anyFilterActive}
					<Button class="mt-4" onclick={() => goto('/compliance-policies/new')}>
						<Plus class="mr-2 h-4 w-4" />
						{m.compliance_policies_create()}
					</Button>
				{/if}
			</div>
		{/snippet}
	</RowList>

	<DataTablePagination {table} />
	{/if}
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.compliance_policies_delete_dialog_title()}
	description={m.compliance_policies_delete_dialog_description({ name: policyToDelete?.name ?? '' })}
	onconfirm={deletePolicy}
/>

<CompliancePolicyDetailSheet onupdated={() => table.refresh()} />
