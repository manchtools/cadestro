// Shell UI-state store. It holds only interaction identity and geometry:
// command-palette state, floating/minimized windows, and terminal device IDs.
// Data-bearing components resolve those IDs through the real RPC client.
//
// A module-level `$state` singleton: the shell is mounted once, above the router
// outlet, so a single instance is exactly right and every surface reads the same
// store. Tests reset it via `resetShell()`.

export type PanelSlot = 'free' | 'left' | 'right' | 'corner';

export interface ShellPanel {
	id: string;
	kind: string; // grouping category in the stage (e.g. 'window', 'device')
	refId: string;
	title: string;
	minimized: boolean;
	x: number;
	y: number;
	slot: PanelSlot;
	/** Interaction-recency stamp (monotonic) — drives LRU auto-stash at the cap. */
	touched: number;
}

/** Hard cap on live (non-minimized) windows. A deliberate tuning
 *  knob, revisited only with real-use evidence; beyond it the LRU panel
 *  auto-stashes to the stage. */
export const WINDOW_CAP = 3;

/** Panels render at a fixed width (Tailwind w-96); clamping and slot geometry
 *  need the number. */
export const PANEL_W = 384;

// Viewport bounds for clamping/slots — set by the shell layout (and by tests).
// Plain module state: geometry is computed at move/snap time, not reactively.
let boundsW = 1280;
let boundsH = 800;
const TOP_MIN = 88; // keep headers below the pill line
export function setShellBounds(w: number, h: number) {
	boundsW = w;
	boundsH = h;
}

export interface TermSession {
	id: string;
	deviceId: string;
	name: string;
}

// ── pill modes ───────────────────────────────────────────────────────────────
// The pill has four modes and shows exactly one. `nav` is the resting state,
// `search` is ⌘K, `selection` is a live multi-select, `context` is a surface
// with committable state. Modes are DERIVED from the slots below, never stored
// separately, so there is no mode/slot pair that can fall out of sync.

export type PillMode = 'nav' | 'search' | 'selection' | 'context';

/** A button in the pill. `label` is already-resolved display text — the store
 *  is not a message registry, so callers pass `m.some_key()`. */
export interface PillAction {
	id: string;
	label: string;
	/** Renders as the accented lead action (one per cluster, by convention). */
	primary?: boolean;
	/** 'danger' marks a destructive action (delete, erase). It is drawn in the
	 *  critical tone so it can never be mistaken for the neutral ones sitting
	 *  beside it — a delete that looks like "Schedule" is a trap. */
	tone?: 'neutral' | 'danger';
	onRun: () => void;
}

export interface PillSubtext {
	text: string;
	/** 'warn' is the concepts' amber caption used for validation rollups. */
	tone?: 'neutral' | 'warn';
}

export interface SelectionState {
	count: number;
	/** Selection implications ("across 3 groups · 1 offline will queue"). */
	subtext?: string;
	subtextTone?: 'neutral' | 'warn';
	actions: PillAction[];
	onClear?: () => void;
}

export interface ContextState {
	/** Stable identity — also the stashed draft's identity. */
	id: string;
	/** App-relative home route of the surface that owns this context
	 *  ('/actions/new', '/roles/01J…'). REQUIRED to stash: a parked draft that
	 *  cannot say where it came from could never be restored from another page.
	 *  Base-path free — the chrome prepends `base` when it navigates. */
	route?: string;
	title: string;
	dirty: boolean;
	valid: boolean;
	/** Already-resolved commit text ("Assign to 12 →"). */
	commitLabel: string;
	/** Validation rollup / compiled query for the detached caption. */
	subtext?: string;
	subtextTone?: 'neutral' | 'warn';
	onCommit: () => void;
	onCancel?: () => void;
	/** Snapshot hook, run before the context parks on the stage. */
	onStash?: () => void;
	/** Resume hook, run after a stashed draft re-enters context mode. Only ever
	 *  fires on the in-place path (the owner is still mounted on `route`); a
	 *  cross-route restore rehydrates through `claimDraft` instead. */
	onRestore?: () => void;
	/** The owner's edit buffer, captured at stash time and handed straight back
	 *  by `claimDraft(id)` when the surface mounts again. Surfaces that cannot
	 *  rebuild their buffer from their own persistence (useDraft, module state)
	 *  MUST provide this — otherwise a cross-route restore returns an empty form. */
	stashPayload?: () => unknown;
	/** Subtitle for the parked draft card; falls back to `subtext`. */
	stashSubtitle?: string;
	extraActions?: PillAction[];
}

/** A parked work-in-progress on the stage rail. `kind` is fixed so the rail can
 *  tell drafts from minimized windows without a type test. */
export interface StageDraft {
	id: string;
	/** The owner's context id (the card id is `draft:${contextId}`). Kept so a
	 *  restore can hand the payload to the owner by context id without parsing. */
	contextId: string;
	kind: 'draft';
	title: string;
	subtitle?: string;
	/** App-relative route of the owning surface — where restoring goes. */
	route: string;
	/** Opaque buffer snapshot, handed back to the owner by `claimDraft`. */
	payload?: unknown;
	/** In-place resume, valid ONLY while the owner is still mounted on `route`.
	 *  Never called after a navigation — its closures would be dead. */
	onRestore?: () => void;
}

interface PillState {
	selection: SelectionState | null;
	context: ContextState | null;
	/** Mode-independent caption; a mode's own subtext takes precedence. */
	subtext: PillSubtext | null;
	/** Esc on a dirty context asks first — the pill renders the confirmation. */
	cancelPending: boolean;
}

interface ShellState {
	paletteOpen: boolean;
	panels: ShellPanel[];
	/** Live drag state — the layout renders the matching snap zone from it.
	 *  `slot` is a dockable slot only; 'free' is never a drag target. */
	drag: { panelId: string | null; slot: Exclude<PanelSlot, 'free'> | null };
	/** aria-live text for auto-stash events (empty = silent). */
	announcement: string;
	pill: PillState;
	drafts: StageDraft[];
	terminal: {
		open: boolean;
		activeId: string | null;
		navsSinceOpen: number;
		sessions: TermSession[];
	};
}

function initial(): ShellState {
	return {
		paletteOpen: false,
		panels: [],
		drag: { panelId: null, slot: null },
		announcement: '',
		pill: { selection: null, context: null, subtext: null, cancelPending: false },
		drafts: [],
		terminal: { open: false, activeId: null, navsSinceOpen: 0, sessions: [] }
	};
}

export const shell = $state<ShellState>(initial());

// Where the app currently is, app-relative and base-path free. Plain module
// state, fed by the layout on every navigation: the store must stay free of
// `$app/*` (chrome-stays-API-free guard + node-environment store tests), so the
// route arrives as data instead of being read from the router.
let currentPath = '';
let previousPath = '';

/** Payloads handed off by a cross-route restore, keyed by context id. The stage
 *  card is removed the instant the operator clicks it (so it never lingers), and
 *  the owning surface reclaims its buffer here on (re)mount via claimDraft. This
 *  decouples "card visible" from "payload handed back", so a reused [id]
 *  component or a slow load can never strand a card on the rail. */
const pendingClaims = new Map<string, unknown>();

/** One-shot guard: a context that was just committed or cancelled must NOT be
 *  auto-stashed by the navigation those actions trigger. A builder's reactive
 *  effect can re-enter a still-dirty context right after exitContext, so the
 *  teardown would otherwise park work the operator explicitly saved or discarded.
 *  Consumed by the next leave, and cleared on navigation as a backstop. */
let suppressStashFor: string | null = null;

/** The layout publishes the active app-relative path here after every
 *  navigation. Restoring a draft compares against it to tell "my owner is still
 *  on screen" from "my owner unmounted three pages ago". The prior path is kept
 *  too: Stash returns the operator to where they opened the editor FROM, so
 *  parking work drops them back on the list instead of stranding them on the
 *  now-empty create/edit surface. */
export function setShellPath(path: string) {
	if (path === currentPath) return;
	suppressStashFor = null;
	previousPath = currentPath;
	currentPath = path;
}

export function shellPath(): string {
	return currentPath;
}

/** The app-relative path the operator was on BEFORE the current one — the
 *  origin Stash returns to. Empty when the editor was the first surface
 *  (a deep link), in which case the chrome leaves them put. */
export function shellPreviousPath(): string {
	return previousPath;
}

/** Test/reset seam — restore a pristine shell between tests. */
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
	shell.terminal = fresh.terminal;
}

// ── pill state machine ───────────────────────────────────────────────────────
// Precedence, highest first: search (an explicit ⌘K act always wins), context
// (a surface holding uncommitted state must keep its commit on screen),
// selection, nav. Nothing here is cleared on navigation — pill state rides
// across routes exactly like panels do.

export function pillMode(): PillMode {
	if (shell.paletteOpen) return 'search';
	if (shell.pill.context) return 'context';
	if (shell.pill.selection) return 'selection';
	return 'nav';
}

/** The caption under the pill: the active mode's own text first, then the
 *  mode-independent strip. `null` means the strip does not render at all. */
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

/** Set (or clear, with `null`) the mode-independent caption. */
export function setPillSubtext(next: PillSubtext | string | null) {
	shell.pill.subtext = typeof next === 'string' ? { text: next, tone: 'neutral' } : next;
}

// ── selection mode ──
export function enterSelection(selection: SelectionState) {
	shell.pill.selection = selection;
}

export function updateSelection(patch: Partial<SelectionState>) {
	if (shell.pill.selection) Object.assign(shell.pill.selection, patch);
}

/** Operator-driven clear (the ✕): notifies the owner, then drops the mode. */
export function clearSelection() {
	const sel = shell.pill.selection;
	shell.pill.selection = null;
	sel?.onClear?.();
}

/** Drop selection mode without notifying the owner (owner-driven teardown). */
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

// ── context mode ──
export function enterContext(context: ContextState) {
	shell.pill.context = context;
	shell.pill.cancelPending = false;
}

export function updateContext(patch: Partial<ContextState>) {
	if (shell.pill.context) Object.assign(shell.pill.context, patch);
}

/** Drop context mode without committing or cancelling. */
export function exitContext() {
	shell.pill.context = null;
	shell.pill.cancelPending = false;
}

/** ⌘S / the commit button. An invalid context can never commit — the guard is
 *  here, not only in the disabled attribute, so the keyboard path is closed
 *  too. Returns whether the commit ran. */
export function commitContext(): boolean {
	const ctx = shell.pill.context;
	// An edit surface holds the pill for its whole visit so the entity's actions
	// (delete, assignments, schedule) have a home, which means a CLEAN context is
	// now a normal resting state. There is nothing to save in it, and ⌘S must not
	// fire a no-op round trip — the guard lives here, not only on the button.
	if (!ctx || !ctx.valid || !ctx.dirty) return false;
	// A commit resolves the context: the navigation it triggers must not
	// auto-stash the (still-dirty) buffer the effect may briefly re-enter.
	suppressStashFor = ctx.id;
	exitContext();
	ctx.onCommit();
	return true;
}

/** Esc / the Cancel button. A dirty context asks first; a clean one just goes. */
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
	// Cancel is an explicit DISCARD: suppress the auto-stash that the leave would
	// otherwise perform when the builder's effect re-enters the still-dirty
	// context after exitContext. Without this, cancelling parked the work anyway.
	suppressStashFor = ctx.id;
	exitContext();
	ctx.onCancel?.();
}

export function dismissCancelConfirm() {
	shell.pill.cancelPending = false;
}

/** The stage-card id a context parks under. */
export function draftIdFor(contextId: string): string {
	return `draft:${contextId}`;
}

/** The third exit: park the context on the stage as a draft and free the pill
 *  back to nav.
 *
 *  A stashable context MUST declare its `route`. Stash is the one exit that
 *  outlives the surface: the operator can navigate anywhere before coming back,
 *  and a draft with no home could only ever restore into dead closures. So a
 *  routeless context is REFUSED loudly rather than parked — no surface can
 *  silently opt into the broken behaviour. */
export function stashContext(): string | null {
	const ctx = shell.pill.context;
	if (!ctx) return null;
	if (!ctx.route) {
		console.error(
			`stashContext: context "${ctx.id}" declares no route — refusing to park a draft that could never be restored`
		);
		return null;
	}
	// Snapshot BEFORE onStash: the owner's stash hook typically releases the pill
	// (parked = true), and a released surface may already be tearing down.
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

/** The owning surface is going away while it still holds the live context — a
 *  navigation, NOT a commit or cancel (both clear the context before the
 *  surface unmounts, so by teardown the id no longer matches). This is the
 *  "auto-stash-on-navigate" rule from the shell concepts: leaving an editor —
 *  including by RESTORING a different parked draft, which navigates away from
 *  the open one — must never destroy the operator's unsaved field state. A
 *  dirty, stashable context parks itself on the stage exactly as the Stash
 *  button would; a clean one, or one with no route to restore to, is simply
 *  released. Restoring draft X while editing Y therefore parks Y instead of
 *  discarding it — the data-loss the single-slot `exitContext` used to cause. */
export function leaveContext(id: string): void {
	const ctx = shell.pill.context;
	if (!ctx || ctx.id !== id) return;
	// A commit or cancel just resolved this id — the leave that follows must
	// drop it, not park it.
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

/** Keyboard seam, shared by the pill chrome and the store tests. Returns true
 *  when the store consumed the key (the caller then preventDefault()s).
 *  ⌘S/Ctrl+S is consumed even when invalid, so the browser's Save dialog can
 *  never appear over an uncommittable draft. */
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

// ── stage drafts ──
// A generic registry: anything with committable state can park a card here,
// not just the context pill.

export function addDraft(draft: StageDraft) {
	const at = shell.drafts.findIndex((d) => d.id === draft.id);
	if (at >= 0) shell.drafts[at] = draft;
	else shell.drafts = [...shell.drafts, draft];
}

export function removeDraft(id: string) {
	shell.drafts = shell.drafts.filter((d) => d.id !== id);
}

/** Resume a parked draft.
 *
 *  Returns the app-relative route the CHROME must navigate to, or `null` when
 *  nothing needs navigating (the owner is already on screen and resumed in
 *  place, or the id is unknown). The store never navigates itself — it stays
 *  free of `$app/*` so the shell chrome keeps its API-free guarantee and the
 *  store tests keep running in node.
 *
 *  The cross-route path deliberately does NOT re-enter the parked context: its
 *  onCommit/onCancel close over a surface that unmounted, which is how a
 *  restored pill used to end up unable to see or do anything. The card stays on
 *  the rail until the owning surface mounts on `route` and calls `claimDraft`,
 *  which hands back the payload so the surface can re-enter with LIVE closures. */
export function restoreDraft(id: string): string | null {
	const draft = shell.drafts.find((d) => d.id === id);
	if (!draft) return null;
	if (draft.route !== currentPath) {
		// Pop the card NOW and stage its payload for the owner. Leaving the card
		// standing until the surface happened to call claimDraft made restoring
		// look unreliable: SvelteKit reuses a [id] component across editor→editor
		// navigation (so a mount-time claim never re-fires) and a load-gated claim
		// lags, both leaving the card sitting on the rail after the click.
		pendingClaims.set(draft.contextId, draft.payload);
		removeDraft(id);
		return draft.route;
	}
	removeDraft(id);
	draft.onRestore?.();
	return null;
}

/** Throw a parked draft away from the rail — the ✕ on a stage card. The
 *  operator should not have to restore work just to cancel it. Any staged
 *  payload goes with it, so a later mount cannot resurrect the buffer. */
export function discardDraft(id: string) {
	const draft = shell.drafts.find((d) => d.id === id);
	if (!draft) return;
	pendingClaims.delete(draft.contextId);
	removeDraft(id);
}

/** The owning surface takes its parked draft back — by CONTEXT id, the same id
 *  it passes to `enterContext`, not the stage-card id.
 *
 *  Read-and-remove: returns the stashed payload (possibly `undefined` for a
 *  surface that rehydrates from its own persistence) and drops the card, so the
 *  second call finds nothing and no orphaned copy can survive the restore.
 *  Call it on mount, unconditionally — with nothing parked it is a no-op. */
export function claimDraft(contextId: string): unknown {
	// A cross-route restore already popped the card and staged the payload here;
	// take that first so the handoff never depends on the card still existing.
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

// ── windows / stage ──────────────────────────────────────────────────────────
let panelSeq = 0;
let touchSeq = 0;

/** Bump a panel's interaction recency (open, restore, focus/raise, drag,
 *  keyboard move all route here) — the LRU order for cap eviction. */
export function touchPanel(id: string) {
	const p = shell.panels.find((x) => x.id === id);
	if (p) p.touched = ++touchSeq;
}

/** Enforce WINDOW_CAP: auto-stash least-recently-touched live panels, never
 *  the just-opened/restored one, announcing each eviction. */
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

/** Move a panel to (x, y), clamped so its header stays reachable.
 *  A free move always leaves slot state. */
export function movePanel(id: string, x: number, y: number) {
	const p = shell.panels.find((q) => q.id === id);
	if (!p) return;
	p.x = Math.max(8, Math.min(x, boundsW - PANEL_W - 8));
	p.y = Math.max(TOP_MIN, Math.min(y, boundsH - 56));
	p.slot = 'free';
}

/** Dock a panel into a slot's geometry. */
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

/** Which slot a panel center is over; null means free placement.
 *  Edge-proximity semantics (Aero-Snap convention): zones engage only near the
 *  viewport edges, so a small drag in the middle never accidentally docks. */
export function slotForCenter(cx: number, cy: number): Exclude<PanelSlot, 'free'> | null {
	if (cx < boundsW * 0.2) return 'left';
	if (cx > boundsW * 0.8) return cy > boundsH * 0.6 ? 'corner' : 'right';
	return null;
}

export function closePanel(id: string) {
	shell.panels = shell.panels.filter((x) => x.id !== id);
}

/** Minimized windows grouped by category, for the stage rail. */
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

// ── keep-alive terminal ──────────────────────────────────────────────────────
// The persistent terminal component owns the RPC and WebSocket. This store only
// preserves which device sessions exist and which one is visible.
export function openTerminal(deviceId: string, name: string): string {
	let s = shell.terminal.sessions.find((x) => x.deviceId === deviceId);
	if (!s) {
		s = {
			id: `terminal:${deviceId}`,
			deviceId,
			name
		};
		shell.terminal.sessions = [...shell.terminal.sessions, s];
		shell.terminal.navsSinceOpen = 0;
	} else if (s.name !== name) {
		s.name = name;
	}
	shell.terminal.activeId = s.id;
	shell.terminal.open = true;
	return s.id;
}

export function focusSession(id: string) {
	shell.terminal.activeId = id;
	shell.terminal.open = true;
}

export function closeSession(id: string) {
	shell.terminal.sessions = shell.terminal.sessions.filter((s) => s.id !== id);
	if (shell.terminal.activeId === id) shell.terminal.activeId = shell.terminal.sessions[0]?.id ?? null;
	if (!shell.terminal.sessions.length) shell.terminal.open = false;
}

export function toggleTerminal() {
	shell.terminal.open = !shell.terminal.open;
	if (shell.terminal.open && !shell.terminal.activeId) shell.terminal.activeId = shell.terminal.sessions[0]?.id ?? null;
}

/** Called after each shell navigation: proves the terminal outlived the route. */
export function notifyNavigated() {
	if (shell.terminal.sessions.length) shell.terminal.navsSinceOpen++;
}
