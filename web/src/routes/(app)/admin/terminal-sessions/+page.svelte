<script lang="ts">

	import { apiClient, formatTimestampDateTime } from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { toast } from 'svelte-sonner';
	import { base } from '$app/paths';
	import { Button } from '$lib/components/ui/button';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import PageShell from '$lib/components/page-shell.svelte';
	import { RefreshCw, SquareTerminal, Ban } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { TerminalSessionInfo } from '$contractClient/client';
	import { RowList, DataTablePagination, createClientListState } from '$lib/components/data-table';

	type SortKey = 'user' | 'device' | 'started' | 'activity';

	const seconds = (t: { seconds: bigint } | undefined) => Number(t?.seconds ?? 0n);

	const table = createClientListState<TerminalSessionInfo, SortKey>({
		load: async () =>
			(await apiClient.listActiveTerminalSessions(100)).sessions as TerminalSessionInfo[],
		searchFields: (s) => [s.userEmail, (s.userId?.value ?? ''), s.deviceHostname, s.deviceId?.value ?? '', s.ttyUser, (s.sessionId?.value ?? '')],
		sortKeys: ['user', 'device', 'started', 'activity'],
		sortComparators: {
			user: (a, b) => (a.userEmail || a.userId?.value || '').localeCompare(b.userEmail || b.userId?.value || ''),
			device: (a, b) => (a.deviceHostname || a.deviceId?.value || '').localeCompare(b.deviceHostname || b.deviceId?.value || ''),
			started: (a, b) => seconds(a.startedAt) - seconds(b.startedAt),
			activity: (a, b) => seconds(a.lastActivityAt) - seconds(b.lastActivityAt)
		},
		defaultSort: 'started',

		sortDir: (key) => (key === 'started' || key === 'activity' ? 'desc' : 'asc')
	});

	const sortOptions = [
		{ key: 'user' as const, label: m.terminal_sessions_user() },
		{ key: 'device' as const, label: m.terminal_sessions_device() },
		{ key: 'started' as const, label: m.terminal_sessions_started_at() },
		{ key: 'activity' as const, label: m.terminal_sessions_last_activity() }
	];

	const stamp = (t: TerminalSessionInfo['startedAt']) => (t ? formatTimestampDateTime(t) : '—');

	let terminateDialogOpen = $state(false);
	let terminateSession = $state<TerminalSessionInfo | null>(null);
	let terminateReason = $state('');
	let terminating = $state(false);

	function openTerminateDialog(session: TerminalSessionInfo) {
		terminateSession = session;
		terminateReason = '';
		terminateDialogOpen = true;
	}

	async function handleTerminate() {
		if (!terminateSession) return;
		const id = terminateSession.sessionId?.value ?? '';
		terminating = true;
		try {
			await apiClient.terminateTerminalSession(id, terminateReason);
			toast.success(m.terminal_sessions_terminated());
			terminateDialogOpen = false;

			table.patchRows((rows) => rows.filter((s) => s.sessionId?.value !== id));
		} catch (err) {
			toast.error(m.terminal_sessions_terminate_failed(), { description: getLocalizedError(err) });
			console.error(err);
		} finally {
			terminating = false;
		}
	}

	$effect(() =>
		registerPageSearch({
			scope: null,
			label: m.nav_terminal_sessions,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);
</script>

<svelte:head>
	<title>{m.terminal_sessions_title()} — Cadestro</title>
</svelte:head>

<PageShell contentClass="space-y-4">

	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.terminal_sessions_title()}</h1>
				<p class="text-muted-foreground">{m.terminal_sessions_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">
				<Button variant="outline" onclick={() => table.refresh()} disabled={table.loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={table.loading}>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	<RowList {table} {sortOptions} rowKey={(s) => (s.sessionId?.value ?? '')}>
		{#snippet row(session)}
			<span class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<SquareTerminal class="h-3.5 w-3.5 text-accent-ink" />
			</span>
			<span class="min-w-0 flex-1">

				<span
					class="block truncate font-mono text-xs text-muted-foreground"
					title={m.terminal_sessions_session()}
				>
					{session.sessionId?.value ?? ''}
				</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<a
						href="{base}/devices/{session.deviceId?.value ?? ''}"
						data-testid="terminal-session-device-link"
						class="shrink-0 truncate font-mono text-sm text-accent-ink hover:underline"
					>
						{session.deviceHostname || session.deviceId?.value}
					</a>
					<span class="truncate text-xs text-muted-foreground">
						{session.userEmail || session.userId?.value || ''}
					</span>
				</span>
			</span>
			<span class="shrink-0 font-mono text-[0.66rem] text-faint" title={m.terminal_sessions_tty_user()}>
				{session.ttyUser}
			</span>

			<span
				class="shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
				title="{m.terminal_sessions_started_at()}: {stamp(
					session.startedAt
				)} · {m.terminal_sessions_last_activity()}: {stamp(session.lastActivityAt)}"
			>
				{stamp(session.startedAt)}
			</span>
		{/snippet}

		{#snippet rowEnd(session)}
			<Button
				variant="ghost"
				size="sm"
				class="text-crit hover:bg-crit-soft hover:text-crit"
				onclick={() => openTerminateDialog(session)}
			>
				<Ban class="mr-1.5 h-3.5 w-3.5" />
				{m.terminal_sessions_terminate()}
			</Button>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<SquareTerminal class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.terminal_sessions_empty()}</h3>
				<p class="text-sm text-muted-foreground">
					{table.query ? m.common_try_different_search() : m.terminal_sessions_empty_hint()}
				</p>
			</div>
		{/snippet}
	</RowList>

	<DataTablePagination {table} />
</PageShell>

<AlertDialog.Root bind:open={terminateDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.terminal_sessions_terminate()}</AlertDialog.Title>
			<AlertDialog.Description>
				{#if terminateSession}
					{m.terminal_sessions_terminate_confirm({
			userEmail: terminateSession.userEmail || terminateSession.userId?.value || '',
						deviceHostname: terminateSession.deviceHostname || terminateSession.deviceId?.value || ''
					})}
				{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>
		{#if terminateSession}
			<p class="font-mono text-xs text-muted-foreground">{terminateSession.sessionId?.value ?? ''}</p>
		{/if}
		<div class="space-y-2 py-2">
			<Label for="terminate-reason">{m.terminal_sessions_terminate_reason()}</Label>
			<Input
				id="terminate-reason"
				bind:value={terminateReason}
				placeholder={m.terminal_sessions_terminate_reason_placeholder()}
			/>
		</div>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={handleTerminate} disabled={terminating}>
				{m.terminal_sessions_terminate()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
