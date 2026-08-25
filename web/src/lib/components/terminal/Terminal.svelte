<script lang="ts">
	import { onMount } from 'svelte';
	import { mode } from 'mode-watcher';
	import type { Terminal as XtermTerminal } from '@xterm/xterm';
	import type { FitAddon } from '@xterm/addon-fit';

	interface Props {
		terminalUrl: string;
		sessionId: string;
		sessionToken: string;
		ttyUser: string;
		onclose?: () => void;
		onerror?: (msg: string) => void;
	}

	let { terminalUrl, sessionId, sessionToken, ttyUser, onclose, onerror }: Props = $props();

	let terminalEl: HTMLDivElement;
	let terminalInstance: XtermTerminal | null = $state(null);
	let ws: WebSocket | null = $state(null);
	let fitAddon: FitAddon | null = null;
	let resizeObserver: ResizeObserver | null = null;
	let lastSentCols = 0;
	let lastSentRows = 0;
	let resizeTimeout: ReturnType<typeof setTimeout> | null = null;

	const darkTheme = {
		background: '#1a1b26',
		foreground: '#a9b1d6',
		cursor: '#c0caf5',
		cursorAccent: '#1a1b26',
		selectionBackground: '#33467c',
		black: '#32344a',
		red: '#f7768e',
		green: '#9ece6a',
		yellow: '#e0af68',
		blue: '#7aa2f7',
		magenta: '#ad8ee6',
		cyan: '#449dab',
		white: '#787c99',
		brightBlack: '#444b6a',
		brightRed: '#ff7a93',
		brightGreen: '#b9f27c',
		brightYellow: '#ff9e64',
		brightBlue: '#7da6ff',
		brightMagenta: '#bb9af7',
		brightCyan: '#0db9d7',
		brightWhite: '#acb0d0'
	};

	const lightTheme = {
		background: '#fafafa',
		foreground: '#383a42',
		cursor: '#526eff',
		cursorAccent: '#fafafa',
		selectionBackground: '#bfceff',
		black: '#383a42',
		red: '#e45649',
		green: '#50a14f',
		yellow: '#c18401',
		blue: '#4078f2',
		magenta: '#a626a4',
		cyan: '#0184bc',
		white: '#a0a1a7',
		brightBlack: '#4f525e',
		brightRed: '#e06c75',
		brightGreen: '#98c379',
		brightYellow: '#e5c07b',
		brightBlue: '#61afef',
		brightMagenta: '#c678dd',
		brightCyan: '#56b6c2',
		brightWhite: '#d4d4d4'
	};

	function getTheme() {
		return mode.current === 'dark' ? darkTheme : lightTheme;
	}

	// Root element's background tracks xterm's own theme background so
	// any sub-row leftover pixels (xterm renders in whole row heights;
	// a container that isn't an exact multiple of row-height has a
	// thin strip below the last row) are invisible. Without this the
	// root inherits bg-background from the page and that strip shows
	// as a gap under the terminal.
	let rootBg = $derived(mode.current === 'dark' ? darkTheme.background : lightTheme.background);

	function sendResize(cols: number, rows: number) {
		if (ws && ws.readyState === WebSocket.OPEN && (cols !== lastSentCols || rows !== lastSentRows)) {
			ws.send(JSON.stringify({ type: 'resize', cols, rows }));
			lastSentCols = cols;
			lastSentRows = rows;
		}
	}

	function handleResize() {
		if (!fitAddon || !terminalInstance) return;
		fitAddon.fit();
		if (resizeTimeout) clearTimeout(resizeTimeout);
		resizeTimeout = setTimeout(() => {
			if (terminalInstance) {
				sendResize(terminalInstance.cols, terminalInstance.rows);
			}
		}, 100);
	}

	onMount(() => {
		let disposed = false;

		(async () => {
			// Dynamic imports to avoid SSR issues — xterm.js is browser-only.
			const { Terminal } = await import('@xterm/xterm');
			const { FitAddon } = await import('@xterm/addon-fit');
			const { WebLinksAddon } = await import('@xterm/addon-web-links');
			await import('@xterm/xterm/css/xterm.css');

			if (disposed) return;

			const term = new Terminal({
				cursorBlink: true,
				fontSize: 14,
				fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
				theme: getTheme(),
				allowProposedApi: true
			});

			const fit = new FitAddon();
			fitAddon = fit;
			term.loadAddon(fit);
			term.loadAddon(new WebLinksAddon());
			term.open(terminalEl);
			fit.fit();
			terminalInstance = term;

			// Open WebSocket. rc10: reject anything that isn't wss://
			// at the client so a misconfigured control server (or a
			// MITM response) cannot downgrade the terminal channel to
			// cleartext ws://. The terminal URL is handed to us by the
			// StartTerminalSession RPC response — trust but verify.
			if (!/^wss:\/\//i.test(terminalUrl)) {
				const msg = `refusing to open insecure terminal WebSocket (${terminalUrl}); operator must configure the control server to return a wss:// URL`;
				console.error(msg);
				terminalInstance?.writeln('\r\n\x1b[1;31m[terminal] ' + msg + '\x1b[0m\r\n');
				return;
			}
			const url = `${terminalUrl}?session_id=${encodeURIComponent(sessionId)}`;
			// WS11: the session token travels in the WebSocket subprotocol
			// (bearer.<token>), never the URL query string, so it cannot
			// leak into reverse-proxy access logs, Referer headers, or
			// devtools. Control echoes the same subprotocol back to
			// complete the handshake.
			const socket = new WebSocket(url, [`bearer.${sessionToken}`]);
			socket.binaryType = 'arraybuffer';
			ws = socket;

			socket.onopen = () => {
				// Send initial resize if the rendered size differs from the
				// default 80x24 the RPC was called with.
				sendResize(term.cols, term.rows);
			};

			socket.onmessage = (event) => {
				if (event.data instanceof ArrayBuffer) {
					term.write(new Uint8Array(event.data));
				} else if (typeof event.data === 'string') {
					// JSON control message from control (future use).
					try {
						const msg = JSON.parse(event.data);
						if (msg.type === 'error') {
							onerror?.(msg.message || 'Unknown error');
						}
					} catch {
						// Ignore unparseable text frames.
					}
				}
			};

			socket.onclose = (event) => {
				if (!disposed) {
					term.write('\r\n\x1b[90m[Session ended]\x1b[0m\r\n');
					onclose?.();
				}
			};

			socket.onerror = () => {
				if (!disposed) {
					onerror?.('WebSocket connection failed');
				}
			};

			// Terminal input → WebSocket binary frames.
			term.onData((data) => {
				if (socket.readyState === WebSocket.OPEN) {
					socket.send(new TextEncoder().encode(data));
				}
			});

			term.onBinary((data) => {
				if (socket.readyState === WebSocket.OPEN) {
					const buf = new Uint8Array(data.length);
					for (let i = 0; i < data.length; i++) {
						buf[i] = data.charCodeAt(i) & 0xff;
					}
					socket.send(buf);
				}
			});

			// Observe container resize → fit → send resize control message.
			resizeObserver = new ResizeObserver(() => handleResize());
			resizeObserver.observe(terminalEl);
		})();

		return () => {
			disposed = true;
			if (resizeTimeout) clearTimeout(resizeTimeout);
			resizeObserver?.disconnect();
			if (ws && ws.readyState <= WebSocket.OPEN) {
				ws.close();
			}
			terminalInstance?.dispose();
		};
	});

	// Sync theme on mode change.
	$effect(() => {
		if (terminalInstance) {
			terminalInstance.options.theme = getTheme();
		}
	});
</script>

<!--
  Two-layer structure. The outer .xterm-host owns the card-level
  chrome (rounded corners, theme background, padding), the inner
  bind target is where xterm mounts. Putting padding on the OUTER
  is deliberate: FitAddon computes rows from
  getComputedStyle(xterm.parentElement).height, which returns the
  border-box height of the element xterm is attached to. If I put
  padding on that element directly, FitAddon ignores the padding
  (it only subtracts padding of .xterm itself) and over-fits —
  content overflows by the padding amount and gets clipped by
  overflow-hidden. Wrapping terminalEl in a separate padded outer
  keeps terminalEl's border-box height equal to the outer's
  content area, so FitAddon's math is right by construction and
  the padding shows up as a uniform inset around the text.
-->
<div
	class="xterm-host h-full w-full overflow-hidden rounded-md p-3"
	style:background-color={rootBg}
>
	<div bind:this={terminalEl} class="h-full w-full"></div>
</div>

<style>
	/*
	 * xterm.js renders its text canvas at rows × cell-height inside
	 * .xterm-screen, but .xterm and .xterm-viewport stretch to 100%
	 * of our container. When the container height isn't a clean
	 * multiple of cell-height, the leftover pixels at the bottom
	 * show .xterm-viewport's own background — which in the DOM
	 * renderer doesn't always match the theme we pass in (there's
	 * a separate internal theming path for the viewport vs. the
	 * canvas).
	 *
	 * Force the whole xterm tree to inherit the host div's
	 * background. Since the host is bound to the current theme's
	 * .background via style:background-color, every descendant
	 * picks up the same colour and the leftover strip is visually
	 * absorbed. No effect on text rendering — the text layer is a
	 * canvas painted by xterm, not a DOM background.
	 */
	.xterm-host :global(.xterm),
	.xterm-host :global(.xterm-viewport),
	.xterm-host :global(.xterm-screen) {
		background-color: inherit;
	}
</style>
