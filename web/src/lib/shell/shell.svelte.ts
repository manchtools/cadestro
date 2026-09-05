

export type PanelSlot = 'free' | 'left' | 'right' | 'corner';

export interface ShellPanel {
	id: string;
	kind: string;
	refId: string;
	title: string;
	minimized: boolean;
	x: number;
	y: number;
	slot: PanelSlot;

	touched: number;
}

export const WINDOW_CAP = 3;

export const PANEL_W = 384;

let boundsW = 1280;
let boundsH = 800;
const TOP_MIN = 88;
export function setShellBounds(w: number, h: number) {
	boundsW = w;
	boundsH = h;
}

export type PillMode = 'nav' | 'search' | 'selection' | 'context';

export interface PillAction {
	id: string;
	label: string;

	primary?: boolean;

	tone?: 'neutral' | 'danger';
	onRun: () => void;
}

export interface PillSubtext {
	text: string;

	tone?: 'neutral' | 'warn';
}

export interface SelectionState {
	count: number;

	subtext?: string;
	subtextTone?: 'neutral' | 'warn';
	actions: PillAction[];
	onClear?: () => void;
}

export interface ContextState {

	id: string;

	route?: string;
	title: string;
	dirty: boolean;
	valid: boolean;

	commitLabel: string;

	subtext?: string;
	subtextTone?: 'neutral' | 'warn';
	onCommit: () => void;
	onCancel?: () => void;

	onStash?: () => void;

	onRestore?: () => void;

	stashPayload?: () => unknown;

	stashSubtitle?: string;
	extraActions?: PillAction[];
}

export interface StageDraft {
	id: string;

	contextId: string;
	kind: 'draft';
	title: string;
	subtitle?: string;

	route: string;

	payload?: unknown;

	onRestore?: () => void;
}

interface PillState {
	selection: SelectionState | null;
	context: ContextState | null;

	subtext: PillSubtext | null;

	cancelPending: boolean;
}

interface ShellState {
	paletteOpen: boolean;
	panels: ShellPanel[];

	drag: { panelId: string | null; slot: Exclude<PanelSlot, 'free'> | null };

	announcement: string;
	pill: PillState;
	drafts: StageDraft[];
}

function initial(): ShellState {
	return {
		paletteOpen: false,
		panels: [],
		drag: { panelId: null, slot: null },
		announcement: '',
		pill: { selection: null, context: null, subtext: null, cancelPending: false },
		drafts: []
	};
}

export const shell = $state<ShellState>(initial());

let currentPath = '';
let previousPath = '';

const pendingClaims = new Map<string, unknown>();

let suppressStashFor: string | null = null;

export function setShellPath(path: string) {
	if (path === currentPath) return;
	suppressStashFor = null;
	previousPath = currentPath;
	currentPath = path;
}

export function shellPath(): string {
	return currentPath;
}

export function shellPreviousPath(): string {
	return previousPath;
}

export function resetShell() {
	const fresh = initial();
	currentPath = '';
	previousPath = '';
	suppressStashFor = null;
	pendingClaims.clear();
	shell.paletteOpen = fresh.paletteOpen;
	shell.panels = fresh.panels;
	shell.drag = fresh.drag;
	shell.announcement = fresh.announcement;
	shell.pill = fresh.pill;
	shell.drafts = fresh.drafts;
}

export function pillMode(): PillMode {
	if (shell.paletteOpen) return 'search';
	if (shell.pill.context) return 'context';
	if (shell.pill.selection) return 'selection';
	return 'nav';
}

export function pillSubtext(): PillSubtext | null {
	const mode = pillMode();
	const own =
		mode === 'context'
			? shell.pill.context
			: mode === 'selection'
				? shell.pill.selection
				: null;
	if (own?.subtext) return { text: own.subtext, tone: own.subtextTone ?? 'neutral' };
	return shell.pill.subtext;
}

export function setPillSubtext(next: PillSubtext | string | null) {
	shell.pill.subtext = typeof next === 'string' ? { text: next, tone: 'neutral' } : next;
}

export function enterSelection(selection: SelectionState) {
	shell.pill.selection = selection;
}

export function updateSelection(patch: Partial<SelectionState>) {
	if (shell.pill.selection) Object.assign(shell.pill.selection, patch);
}

export function clearSelection() {
	const sel = shell.pill.selection;
	shell.pill.selection = null;
	sel?.onClear?.();
}

export function exitSelection() {
	shell.pill.selection = null;
}

export function runPillAction(id: string) {
	const mode = pillMode();
	const actions =
		mode === 'context'
			? (shell.pill.context?.extraActions ?? [])
			: (shell.pill.selection?.actions ?? []);
	actions.find((a) => a.id === id)?.onRun();
}

export function enterContext(context: ContextState) {
	shell.pill.context = context;
	shell.pill.cancelPending = false;
}

export function updateContext(patch: Partial<ContextState>) {
	if (shell.pill.context) Object.assign(shell.pill.context, patch);
}

export function exitContext() {
	shell.pill.context = null;
	shell.pill.cancelPending = false;
}

export function commitContext(): boolean {
	const ctx = shell.pill.context;

	if (!ctx || !ctx.valid || !ctx.dirty) return false;

	suppressStashFor = ctx.id;
	exitContext();
	ctx.onCommit();
	return true;
}

export function requestCancelContext() {
	const ctx = shell.pill.context;
	if (!ctx) return;
	if (ctx.dirty) {
		shell.pill.cancelPending = true;
		return;
	}
	confirmCancelContext();
}

export function confirmCancelContext() {
	const ctx = shell.pill.context;
	if (!ctx) return;

	suppressStashFor = ctx.id;
	exitContext();
	ctx.onCancel?.();
}

export function dismissCancelConfirm() {
	shell.pill.cancelPending = false;
}

export function draftIdFor(contextId: string): string {
	return `draft:${contextId}`;
}

export function stashContext(): string | null {
	const ctx = shell.pill.context;
	if (!ctx) return null;
	if (!ctx.route) {
		console.error(
			`stashContext: context "${ctx.id}" declares no route — refusing to park a draft that could never be restored`
		);
		return null;
	}

	const payload = ctx.stashPayload?.();
	ctx.onStash?.();
	const id = draftIdFor(ctx.id);
	const draft: StageDraft = {
		id,
		contextId: ctx.id,
		kind: 'draft',
		title: ctx.title,
		subtitle: ctx.stashSubtitle ?? ctx.subtext,
		route: ctx.route,
		payload,
		onRestore: () => {
			enterContext(ctx);
			ctx.onRestore?.();
		}
	};
	addDraft(draft);
	exitContext();
	return id;
}

export function leaveContext(id: string): void {
	const ctx = shell.pill.context;
	if (!ctx || ctx.id !== id) return;

	if (suppressStashFor === id) {
		suppressStashFor = null;
		exitContext();
		return;
	}
	if (ctx.dirty && ctx.route) {
		stashContext();
		return;
	}
	exitContext();
}

export function handlePillKey(e: { key: string; metaKey?: boolean; ctrlKey?: boolean }): boolean {
	const mode = pillMode();
	if (mode === 'context') {
		if (e.key === 'Escape') {
			if (shell.pill.cancelPending) dismissCancelConfirm();
			else requestCancelContext();
			return true;
		}
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			commitContext();
			return true;
		}
		return false;
	}
	if (mode === 'selection' && e.key === 'Escape') {
		clearSelection();
		return true;
	}
	return false;
}

export function addDraft(draft: StageDraft) {
	const at = shell.drafts.findIndex((d) => d.id === draft.id);
	if (at >= 0) shell.drafts[at] = draft;
	else shell.drafts = [...shell.drafts, draft];
}

export function removeDraft(id: string) {
	shell.drafts = shell.drafts.filter((d) => d.id !== id);
}

export function restoreDraft(id: string): string | null {
	const draft = shell.drafts.find((d) => d.id === id);
	if (!draft) return null;
	if (draft.route !== currentPath) {

		pendingClaims.set(draft.contextId, draft.payload);
		removeDraft(id);
		return draft.route;
	}
	removeDraft(id);
	draft.onRestore?.();
	return null;
}

export function discardDraft(id: string) {
	const draft = shell.drafts.find((d) => d.id === id);
	if (!draft) return;
	pendingClaims.delete(draft.contextId);
	removeDraft(id);
}

export function claimDraft(contextId: string): unknown {

	if (pendingClaims.has(contextId)) {
		const payload = pendingClaims.get(contextId);
		pendingClaims.delete(contextId);
		return payload;
	}
	const id = draftIdFor(contextId);
	const draft = shell.drafts.find((d) => d.id === id);
	if (!draft) return undefined;
	removeDraft(id);
	return draft.payload;
}

let panelSeq = 0;
let touchSeq = 0;

export function touchPanel(id: string) {
	const p = shell.panels.find((x) => x.id === id);
	if (p) p.touched = ++touchSeq;
}

function enforceCap(excludeId: string) {
	for (;;) {
		const live = shell.panels.filter((p) => !p.minimized);
		if (live.length <= WINDOW_CAP) return;
		const victims = live.filter((p) => p.id !== excludeId);
		if (!victims.length) return;
		const lru = victims.reduce((a, b) => (a.touched <= b.touched ? a : b));
		lru.minimized = true;
		shell.announcement = `${lru.title} parked to stage — ${WINDOW_CAP} windows max`;
	}
}

export function openPanel(kind: string, refId: string, title: string): string {
	const id = `${kind}:${refId}`;
	const existing = shell.panels.find((p) => p.id === id);
	if (existing) {
		existing.minimized = false;
		touchPanel(id);
		enforceCap(id);
		return id;
	}
	const n = panelSeq++;
	shell.panels.push({
		id,
		kind,
		refId,
		title,
		minimized: false,
		x: 96 + (n % 6) * 26,
		y: 108 + (n % 6) * 26,
		slot: 'free',
		touched: ++touchSeq
	});
	enforceCap(id);
	return id;
}

export function minimizePanel(id: string) {
	const p = shell.panels.find((x) => x.id === id);
	if (p) p.minimized = true;
}

export function restorePanel(id: string) {
	const p = shell.panels.find((x) => x.id === id);
	if (!p) return;
	p.minimized = false;
	touchPanel(id);
	enforceCap(id);
}

export function movePanel(id: string, x: number, y: number) {
	const p = shell.panels.find((q) => q.id === id);
	if (!p) return;
	p.x = Math.max(8, Math.min(x, boundsW - PANEL_W - 8));
	p.y = Math.max(TOP_MIN, Math.min(y, boundsH - 56));
	p.slot = 'free';
}

export function snapPanel(id: string, slot: Exclude<PanelSlot, 'free'>) {
	const p = shell.panels.find((q) => q.id === id);
	if (!p) return;
	if (slot === 'left') {
		p.x = 16;
		p.y = TOP_MIN + 8;
	} else if (slot === 'right') {
		p.x = boundsW - PANEL_W - 16;
		p.y = TOP_MIN + 8;
	} else {
		p.x = boundsW - PANEL_W - 16;
		p.y = boundsH - 336;
	}
	p.slot = slot;
}

export function slotForCenter(cx: number, cy: number): Exclude<PanelSlot, 'free'> | null {
	if (cx < boundsW * 0.2) return 'left';
	if (cx > boundsW * 0.8) return cy > boundsH * 0.6 ? 'corner' : 'right';
	return null;
}

export function closePanel(id: string) {
	shell.panels = shell.panels.filter((x) => x.id !== id);
}

export function stagedByKind(): { kind: string; panels: ShellPanel[] }[] {
	const by = new Map<string, ShellPanel[]>();
	for (const p of shell.panels) {
		if (!p.minimized) continue;
		const bucket = by.get(p.kind) ?? [];
		if (!by.has(p.kind)) by.set(p.kind, bucket);
		bucket.push(p);
	}
	return [...by.entries()].map(([kind, panels]) => ({ kind, panels }));
}
