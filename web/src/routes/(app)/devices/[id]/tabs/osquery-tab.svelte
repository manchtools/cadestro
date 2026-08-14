<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient } from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import type { OSQueryRow } from '$contract/cadestro/v1/agent_pb';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import {
		RefreshCw,
		Search,
		Terminal,
		Activity,
		Network,
		Server,
		Users,
		HardDrive,
		Clock,
		UserCheck,
		Shield,
		KeyRound,
		Play,
		UsersRound,
		Code
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		deviceId: string;
	}

	let { deviceId }: Props = $props();

	// Preset queries
	const presets = [
		{ table: 'processes', label: () => m.osquery_processes(), icon: Activity },
		{ table: 'listening_ports', label: () => m.osquery_ports(), icon: Network },
		{ table: 'systemd_units', label: () => m.osquery_systemd(), icon: Server },
		{ table: 'users', label: () => m.osquery_users(), icon: Users },
		{ table: 'logged_in_users', label: () => m.osquery_logged_in(), icon: UserCheck },
		{ table: 'groups', label: () => m.osquery_groups(), icon: UsersRound },
		{ table: 'mounts', label: () => m.osquery_mounts(), icon: HardDrive },
		{ table: 'crontab', label: () => m.osquery_cron(), icon: Clock },
		{ table: 'iptables', label: () => m.osquery_firewall(), icon: Shield },
		{ table: 'authorized_keys', label: () => m.osquery_ssh_keys(), icon: KeyRound },
		{ table: 'startup_items', label: () => m.osquery_startup(), icon: Play }
	];

	// Query state
	let customTable = $state('');
	let customLimit = $state(100);
	let customSql = $state('');
	let activeQueryId = $state('');
	let queryLoading = $state(false);
	let queryTable = $state('');
	let queryRows = $state<OSQueryRow[]>([]);
	let queryError = $state('');
	let queryCompleted = $state(false);

	async function executeQuery(table: string, limit: number = 100, rawSql?: string) {
		queryLoading = true;
		queryTable = rawSql ? 'SQL' : table;
		queryRows = [];
		queryError = '';
		queryCompleted = false;

		try {
			const queryId = await apiClient.dispatchOSQuery(
				deviceId,
				rawSql ? '' : table,
				[],
				rawSql ? 0 : limit,
				rawSql
			);
			activeQueryId = queryId;

			// Poll for result
			const maxPolls = 60;
			for (let i = 0; i < maxPolls; i++) {
				await new Promise((r) => setTimeout(r, 500));
				try {
					const result = await apiClient.getOSQueryResult(queryId);
					if (result.completed) {
						queryCompleted = true;
						if (result.success) {
							queryRows = result.rows;
						} else {
							queryError = result.error || 'Unknown error';
						}
						break;
					}
				} catch (err) {
					console.error('Poll error:', err);
				}
			}

			if (!queryCompleted) {
				queryError = 'Query timed out';
			}
		} catch (error) {
			queryError = getLocalizedError(error);
			toast.error(queryError);
		} finally {
			queryLoading = false;
		}
	}

	function getResultColumns(): string[] {
		if (queryRows.length === 0) return [];
		return Object.keys(queryRows[0].data).sort();
	}
</script>

<div class="space-y-6">
	<!-- Preset Buttons -->
	<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
		<div class="flex items-center gap-2">
			<Search class="h-4 w-4 text-faint" />
			<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
				{m.osquery_title()}
			</span>
		</div>
		<div class="mt-3 space-y-4">
			<div class="flex flex-wrap gap-2">
				{#each presets as preset}
					<Button
						variant="outline"
						onclick={() => executeQuery(preset.table)}
						disabled={queryLoading}
					>
						<preset.icon class="h-4 w-4 mr-2" />
						{preset.label()}
					</Button>
				{/each}
			</div>

			<hr />

			<div class="space-y-2">
				<Label>{m.osquery_custom()}</Label>
				<div class="flex gap-2">
					<Input
						type="text"
						placeholder={m.osquery_table_name()}
						bind:value={customTable}
						class="flex-1"
					/>
					<Input
						type="number"
						placeholder={m.osquery_limit()}
						bind:value={customLimit}
						class="w-24"
						min={1}
						max={10000}
					/>
					<Button
						onclick={() => executeQuery(customTable, customLimit)}
						disabled={queryLoading || !customTable}
					>
						<Terminal class="h-4 w-4 mr-2" />
						{m.osquery_execute()}
					</Button>
				</div>
			</div>

			<hr />

			<div class="space-y-2">
				<Label>{m.osquery_custom_sql()}</Label>
				<Textarea
					placeholder={m.osquery_sql_placeholder()}
					bind:value={customSql}
					rows={3}
					class="font-mono text-sm"
				/>
				<Button
					onclick={() => executeQuery('', 0, customSql)}
					disabled={queryLoading || !customSql}
				>
					<Code class="h-4 w-4 mr-2" />
					{m.osquery_execute()}
				</Button>
			</div>
		</div>
	</section>

	<!-- Results -->
	{#if queryLoading}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="mr-2 h-6 w-6 animate-spin text-muted-foreground" />
			<span class="text-muted-foreground">{m.osquery_loading()}</span>
		</div>
	{:else if queryError}
		<div class="rounded-xl border border-hair bg-surface px-4 py-8 shadow-plate">
			<p class="text-center text-crit">{m.osquery_error({ error: queryError })}</p>
		</div>
	{:else if queryCompleted && queryRows.length === 0}
		<div class="rounded-xl border border-hair bg-surface px-4 py-8 shadow-plate">
			<p class="text-center text-muted-foreground">{m.osquery_no_results()}</p>
		</div>
	{:else if queryRows.length > 0}
		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{queryTable}
				</span>
				<Badge variant="outline">{m.osquery_result_rows({ count: String(queryRows.length) })}</Badge>
			</div>
			<div class="mt-3">
				<div class="max-h-[500px] overflow-auto">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								{#each getResultColumns() as col}
									<Table.Head class="whitespace-nowrap">{col}</Table.Head>
								{/each}
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each queryRows.slice(0, 500) as row}
								<Table.Row>
									{#each getResultColumns() as col}
										<Table.Cell class="text-xs max-w-[300px] truncate" title={row.data[col] ?? ''}>
											{row.data[col] ?? ''}
										</Table.Cell>
									{/each}
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
					{#if queryRows.length > 500}
						<p class="text-sm text-muted-foreground text-center py-2">
							{m.osquery_truncated({ shown: 500, total: queryRows.length })}
						</p>
					{/if}
				</div>
			</div>
		</section>
	{/if}
</div>
