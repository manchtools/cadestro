<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { Minus, X } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import {
		shell,
		minimizePanel,
		closePanel,
		movePanel,
		snapPanel,
		slotForCenter,
		touchPanel,
		PANEL_W,
		type ShellPanel
	} from '$lib/shell/shell.svelte';

	let { panel, content }: { panel: ShellPanel; content?: Snippet } = $props();

	// The header is the only drag surface. A press on a header button never starts
	// a drag because pointer capture would eat the click. Position is store state;
	// the component renders it and movePanel clamps it.
	let dragging = false;
	let sx = 0;
	let sy = 0;
	let ox = 0;
	let oy = 0;

	function onHeaderPointerDown(e: PointerEvent) {
		if ((e.target as Element).closest('button')) return;
		dragging = true;
		sx = e.clientX;
		sy = e.clientY;
		ox = panel.x;
		oy = panel.y;
		(e.currentTarget as Element).setPointerCapture(e.pointerId);
		touchPanel(panel.id);
	}

	function onHeaderPointerMove(e: PointerEvent) {
		if (!dragging) return;
		movePanel(panel.id, ox + e.clientX - sx, oy + e.clientY - sy);
		shell.drag = { panelId: panel.id, slot: slotForCenter(panel.x + PANEL_W / 2, panel.y + 40) };
	}

	function onHeaderPointerUp() {
		if (!dragging) return;
		dragging = false;
		const slot = shell.drag.slot;
		if (slot) snapPanel(panel.id, slot);
		shell.drag = { panelId: null, slot: null };
	}

	// Keyboard movement: arrows 16px, Shift+arrows 48px, Escape parks.
	function onHeaderKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			minimizePanel(panel.id);
			return;
		}
		const step = e.shiftKey ? 48 : 16;
		const delta: Record<string, [number, number]> = {
			ArrowLeft: [-step, 0],
			ArrowRight: [step, 0],
			ArrowUp: [0, -step],
			ArrowDown: [0, step]
		};
		const d = delta[e.key];
		if (!d) return;
		e.preventDefault();
		movePanel(panel.id, panel.x + d[0], panel.y + d[1]);
		touchPanel(panel.id);
	}
</script>

<div
	class="fixed z-30 flex max-h-[78vh] w-96 flex-col overflow-hidden rounded-xl border bg-card text-card-foreground shadow-pill"
	style="left:{panel.x}px; top:{panel.y}px"
	data-testid="panel"
	data-panel-id={panel.id}
	data-slot={panel.slot}
	in:fly={{ y: 10, duration: 160 }}
	out:fly={{ x: 260, y: -180, duration: 260 }}
>
	<div
		class="flex cursor-grab touch-none items-center justify-between gap-2 border-b bg-muted/50 px-3 py-2 active:cursor-grabbing"
		data-testid="panel-header"
		role="button"
		tabindex="0"
		aria-label={m.shell_panel_drag_hint({ title: panel.title })}
		onpointerdown={onHeaderPointerDown}
		onpointermove={onHeaderPointerMove}
		onpointerup={onHeaderPointerUp}
		onpointercancel={onHeaderPointerUp}
		onkeydown={onHeaderKeydown}
	>
		<span class="min-w-0 flex-1 truncate font-mono text-sm font-medium">{panel.title}</span>
		<div class="flex shrink-0 items-center gap-1">
			<button type="button" aria-label={m.shell_minimise()} onclick={() => minimizePanel(panel.id)} class="rounded p-1 hover:bg-accent hover:text-accent-foreground [&_svg]:pointer-events-none">
				<Minus class="h-4 w-4" />
			</button>
			<button type="button" aria-label={m.shell_close()} onclick={() => closePanel(panel.id)} class="rounded p-1 hover:bg-destructive hover:text-destructive-foreground [&_svg]:pointer-events-none">
				<X class="h-4 w-4" />
			</button>
		</div>
	</div>

	<div class="overflow-y-auto p-4">
		{@render content?.()}
	</div>
</div>
