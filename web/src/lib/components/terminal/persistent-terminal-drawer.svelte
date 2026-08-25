<script lang="ts">
	import { apiClient, type StartTerminalResponse } from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import { shell, closeSession, focusSession, toggleTerminal, type TermSession } from '$lib/shell/shell.svelte';
	import { Button } from '$lib/components/ui/button';
	import Terminal from './Terminal.svelte';
	import { Minus, RotateCw, X } from '@lucide/svelte';
	import { toast } from 'svelte-sonner';
	import { onDestroy } from 'svelte';
	import * as m from '$lib/paraglide/messages';

	interface ConnectionState {
		loading: boolean;
		response: StartTerminalResponse | null;
		error: string;
		ended: boolean;
	}

	let connections: Record<string, ConnectionState> = $state({});
	let strip = $state<HTMLElement | null>(null);
	let destroyed = false;
	const active = $derived(
		shell.terminal.sessions.find((session) => session.id === shell.terminal.activeId) ??
			shell.terminal.sessions[0]
	);

	async function stopResponse(response: StartTerminalResponse | null) {
		if (!response) return;
		await apiClient.stopTerminal((response.sessionId?.value ?? ''));
	}

	async function start(session: TermSession) {
		const previous = connections[session.id]?.response ?? null;
		const state: ConnectionState = { loading: true, response: null, error: '', ended: false };
		connections[session.id] = state;
		if (previous) {
			try {
				await stopResponse(previous);
			} catch (cause) {
				console.warn('stop previous terminal session', cause);
			}
		}
		if (destroyed || connections[session.id] !== state || !shell.terminal.sessions.some((candidate) => candidate.id === session.id)) return;
		try {
			const response = await apiClient.startTerminal(session.deviceId);
			if (destroyed || !shell.terminal.sessions.some((candidate) => candidate.id === session.id) || connections[session.id] !== state) {
				try {
					await stopResponse(response);
				} catch (cause) {
					console.warn('stop superseded terminal session', cause);
				}
				return;
			}
			state.response = response;
		} catch (cause) {
			if (connections[session.id] === state) state.error = getLocalizedError(cause);
		} finally {
			if (connections[session.id] === state) state.loading = false;
		}
	}

	async function disconnect(session: TermSession) {
		const state = connections[session.id];
		delete connections[session.id];
		closeSession(session.id);
		try {
			await stopResponse(state?.response ?? null);
		} catch (cause) {
			toast.error(getLocalizedError(cause));
		}
	}

	function markEnded(sessionId: string, error = '') {
		const state = connections[sessionId];
		if (!state) return;
		state.ended = true;
		state.error = error;
	}

	$effect(() => {
		for (const session of shell.terminal.sessions) {
			if (!connections[session.id]) void start(session);
		}
	});

	// A newly opened session is appended to the far end of a scrolling strip, so
	// the tab for the terminal actually on screen would start out of view. Keep
	// the active tab inside the strip's window; `nearest` on both axes so this
	// never drags the page itself.
	$effect(() => {
		const id = active?.id;
		if (!id || !strip) return;
		strip
			.querySelector(`[data-session-tab="${CSS.escape(id)}"]`)
			?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
	});

	onDestroy(() => {
		destroyed = true;
		for (const state of Object.values(connections)) {
			void stopResponse(state.response).catch((cause) => {
				console.warn('stop terminal during shell teardown', cause);
			});
		}
	});
</script>

{#if shell.terminal.sessions.length > 0}
	<div
		data-testid="terminal"
		aria-hidden={!shell.terminal.open}
		class="fixed bottom-4 right-4 z-40 flex h-[28rem] w-[min(94vw,48rem)] flex-col overflow-hidden rounded-xl border border-hair bg-surface text-foreground shadow-pill transition-[transform,opacity] duration-200 {shell.terminal.open ? '' : 'pointer-events-none translate-y-[calc(100%+2rem)] opacity-0'}"
	>
		<div class="flex items-center gap-1 border-b border-hair bg-sunken px-2 py-1.5">
			<!-- The strip owns its own horizontal scroll. The drawer is a fixed width
			     and clips (overflow-hidden), so a tab row that merely overflows is a
			     tab the operator can neither read, focus, nor disconnect. Tabs keep
			     their width (shrink-0) and the strip scrolls instead. -->
			<div bind:this={strip} data-testid="terminal-tabs" class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
				{#each shell.terminal.sessions as session (session.id)}
					<div data-session-tab={session.id} class="group flex shrink-0 items-center rounded-md {session.id === active?.id ? 'bg-surface text-foreground' : 'text-muted-foreground hover:bg-surface'}">
						<button type="button" onclick={() => focusSession(session.id)} class="max-w-44 truncate px-2 py-1 font-mono text-xs">
							{session.name}
						</button>
						<button type="button" aria-label={`${m.terminal_disconnect()} ${session.name}`} onclick={() => void disconnect(session)} class="mr-1 rounded p-0.5 opacity-60 hover:bg-sunken hover:opacity-100">
							<X class="h-3 w-3" />
						</button>
					</div>
				{/each}
			</div>
			<button type="button" aria-label={m.shell_minimise_terminal()} onclick={toggleTerminal} class="ml-1 shrink-0 rounded-md p-1 text-muted-foreground hover:bg-sunken">
				<Minus class="h-4 w-4" />
			</button>
		</div>

		<div class="relative min-h-0 flex-1">
			{#each shell.terminal.sessions as session (session.id)}
				{@const state = connections[session.id]}
				<div class="absolute inset-0 p-2 {session.id === active?.id ? 'visible' : 'invisible'}">
					{#if !state || state.loading}
						<div class="flex h-full items-center justify-center text-sm text-muted-foreground">{m.terminal_connecting()}</div>
					{:else if state.error && !state.response}
						<div class="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
							<p>{state.error}</p>
							<Button size="sm" variant="outline" onclick={() => void start(session)}>
								<RotateCw class="h-3.5 w-3.5" /> {m.terminal_reconnect()}
							</Button>
						</div>
					{:else if state.ended}
						<div class="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
							<p>{state.error || m.terminal_session_ended()}</p>
							<Button size="sm" variant="outline" onclick={() => void start(session)}>
								<RotateCw class="h-3.5 w-3.5" /> {m.terminal_reconnect()}
							</Button>
						</div>
					{:else if state.response}
						<Terminal
							terminalUrl={state.response.terminalUrl}
							sessionId={(state.response.sessionId?.value ?? '')}
							sessionToken={state.response.sessionToken}
							ttyUser={state.response.ttyUser}
							onclose={() => markEnded(session.id)}
							onerror={(message) => markEnded(session.id, message)}
						/>
					{/if}
				</div>
			{/each}
		</div>
	</div>
{/if}
