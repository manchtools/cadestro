<script lang="ts">
	// THE ⌘K surface — one palette, rendered as the pill's search mode (the pill's
	// fourth posture; movement E, "the palette is honest about what search is").
	// It is the only search implementation: the morph bar owns the morph and hands
	// this component the box, which is why the shell's own jump targets (sections,
	// windows, terminals, drafts) are rows HERE and not a second row model there.
	//
	// Four row families with deliberately DIFFERENT rules:
	//
	//   page rows   — when a list page has registered itself (see
	//                 $lib/shell/page-search), the FIRST facet is that page and
	//                 typing drives the page's own list state. No RPC is issued:
	//                 the page already owns one, and the operator is searching
	//                 what they are looking at. ⇥ leaves the page facet and the
	//                 palette becomes the global one below.
	//   entity rows — rendered exactly as the Search RPC returned them. Server
	//                 search is prefix matching over ten scopes with keyset
	//                 paging and no relevance ranking, so the palette does no
	//                 client re-ranking, no re-sorting and no fuzzy matching:
	//                 it never pretends to rank. Paging is a "show next" row fed
	//                 by next_page_token — never numbered pages.
	//   section rows— the nav tables the layout already filtered by permission.
	//   shell rows  — open windows, live terminals and stashed drafts exist only
	//                 in this browser tab. Together with sections they are the
	//                 ONLY rows matched locally (plain substring on the title).
	//
	// Facets are the ten real SearchScope values plus "All" — UNSPECIFIED, which
	// the contract defines as "all scopes" — preceded by the registered page, if
	// any. ⇥ cycles the ring and re-scopes the RPC.
	import { untrack } from 'svelte';
	import { goto } from '$lib/navigation';
	import { apiClient } from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		Monitor,
		Group,
		Send,
		Layers,
		FolderTree,
		ShieldCheck,
		Users,
		UsersRound,
		ScrollText,
		Search,
		AppWindow,
		SquareTerminal,
		PencilLine,
		ArrowRight,
		ListFilter,
		X
	} from '@lucide/svelte';
	import type { SearchResult } from '$contractClient/client';
	import { SearchScope } from '$contract/cadestro/v1/common_pb';
	import { TONE_FILL, TONE_LABEL, type FleetTone } from '$lib/components/fleet';
	import type { PillEntry, PillGroup } from '$lib/shell/nav';
	import { shell, restorePanel, focusSession, restoreDraft } from '$lib/shell/shell.svelte';
	import { activePageSearch, type PageSearchRegistration } from '$lib/shell/page-search.svelte';

	// Sections arrive as props for the same reason the pill takes them as props:
	// the layout owns the auth client and hands down the permission-filtered
	// tables. This component may talk to the Search RPC, but not to authStore.
	let {
		open = $bindable(false),
		sections = [],
		overflow = []
	}: {
		open?: boolean;
		sections?: PillEntry[];
		overflow?: PillGroup[];
	} = $props();

	/** Palette page size. Small on purpose: the palette is a jump target, and
	 *  "show next" keeps the rest one keystroke away. */
	const PAGE_SIZE = 8;

	interface Facet {
		scope: SearchScope;
		label: () => string;
		icon: typeof Monitor;
		/** Section route. `detail: false` scopes have no per-entity page, so a
		 *  row deep-links the section's own query instead of inventing a URL. */
		route: string;
		detail: boolean;
	}

	// "All" first, then the ten scopes the server actually indexes.
	const FACETS: Facet[] = [
		{ scope: SearchScope.UNSPECIFIED, label: () => m.search_facet_all(), icon: Search, route: '', detail: false },
		{ scope: SearchScope.DEVICES, label: () => m.search_group_devices(), icon: Monitor, route: '/devices', detail: true },
		{ scope: SearchScope.DEVICE_GROUPS, label: () => m.search_group_device_groups(), icon: Group, route: '/device-groups', detail: true },
		{ scope: SearchScope.ACTIONS, label: () => m.search_group_actions(), icon: Send, route: '/actions', detail: true },
		{ scope: SearchScope.ACTION_SETS, label: () => m.search_group_action_sets(), icon: Layers, route: '/action-sets', detail: true },
		{ scope: SearchScope.DEFINITIONS, label: () => m.search_group_definitions(), icon: FolderTree, route: '/definitions', detail: true },
		{ scope: SearchScope.COMPLIANCE_POLICIES, label: () => m.search_group_compliance_policies(), icon: ShieldCheck, route: '/compliance-policies', detail: true },
		{ scope: SearchScope.USERS, label: () => m.search_group_users(), icon: Users, route: '/users', detail: true },
		{ scope: SearchScope.USER_GROUPS, label: () => m.search_group_user_groups(), icon: UsersRound, route: '/user-groups', detail: true },
		{ scope: SearchScope.AUDIT_EVENTS, label: () => m.search_group_audit_events(), icon: ScrollText, route: '/audit', detail: false }
	];
	const FACET_BY_SCOPE = new Map(FACETS.map((f) => [f.scope, f]));

	/** The chip ring: the registered page first (when there is one), then the
	 *  eleven server facets. `page: true` is the one entry that issues no RPC. */
	interface RingFacet {
		key: string;
		scope: SearchScope;
		label: () => string;
		page: boolean;
	}

	/** Captured at open, not read live: a page re-registering mid-keystroke must
	 *  not reseed the box under the operator's fingers. `$state.raw` on purpose —
	 *  a deep proxy would clone the page's getter into a snapshot and destroy the
	 *  identity the seam's release guard depends on. */
	let scoped = $state.raw<PageSearchRegistration | null>(null);

	let query = $state('');
	let facet = $state(0);
	let results = $state<SearchResult[]>([]);
	let nextPageToken = $state('');
	let totalCount = $state(0);
	let searching = $state(false);
	let failed = $state(false);
	let sel = $state(0);
	let input = $state<HTMLInputElement | null>(null);
	let debounce: ReturnType<typeof setTimeout> | undefined;
	// Monotonic guard: a slow response for an abandoned query must never
	// overwrite the rows of the query the operator is looking at.
	let issued = 0;

	const ring = $derived.by((): RingFacet[] => {
		const base: RingFacet[] = FACETS.map((f) => ({
			key: `scope-${f.scope}`,
			scope: f.scope,
			label: f.label,
			page: false
		}));
		if (!scoped) return base;
		// A page whose rows come from a plain list RPC has no SearchScope; its chip
		// still leads the ring, it just carries no scope to re-run anything with.
		return [
			{
				key: 'page',
				scope: (scoped.scope ?? SearchScope.UNSPECIFIED) as SearchScope,
				label: scoped.label,
				page: true
			},
			...base
		];
	});
	const activeFacet = $derived(ring[facet] ?? ring[0]);
	/** True while keystrokes belong to the page's list instead of the RPC. */
	const pageMode = $derived(!!scoped && !!activeFacet?.page);

	// ── rows ──────────────────────────────────────────────────────────────────
	type Row =
		| { kind: 'entity'; id: string; result: SearchResult }
		| { kind: 'next'; id: string }
		| { kind: 'page'; id: string }
		| { kind: 'clear'; id: string }
		| { kind: 'section'; id: string; href: string; title: string }
		| { kind: 'panel' | 'terminal' | 'draft'; id: string; refId: string; title: string; sub: string };

	interface RowGroup {
		key: string;
		heading: string;
		rows: Row[];
	}

	/** Entity groups in SERVER order: scopes appear in the order the response
	 *  first mentions them, rows keep their response index. Nothing is sorted. */
	const entityGroups = $derived.by((): RowGroup[] => {
		const by = new Map<SearchScope, SearchResult[]>();
		for (const r of results) {
			const bucket = by.get(r.scope);
			if (bucket) bucket.push(r);
			else by.set(r.scope, [r]);
		}
		const q = query.trim();
		return [...by.entries()].map(([scope, items]) => {
			const label = FACET_BY_SCOPE.get(scope)?.label() ?? m.search_facet_all();
			// total_count belongs to the REQUEST. Under a single facet it is that
			// scope's total, so "8 of 41" is true; under "All" it spans every
			// scope, so a per-group "of N" would be a lie and is left off.
			const heading =
				activeFacet.scope === scope && totalCount > 0
					? m.search_group_count_total({ scope: label, query: q, shown: items.length, total: totalCount })
					: m.search_group_count({ scope: label, query: q, shown: items.length });
			return {
				key: `scope-${scope}`,
				heading,
				rows: items.map((r) => ({ kind: 'entity' as const, id: `entity-${scope}-${r.id}`, result: r }))
			};
		});
	});

	/** The one honest paging affordance: the server handed back a cursor. */
	const nextRow = $derived<Row[]>(nextPageToken ? [{ kind: 'next', id: 'next-page' }] : []);

	/** The page facet's own rows: a "stay here" row (↵ closes and leaves the
	 *  filter applied) and, once the page carries a query, a way to drop it. */
	const pageGroup = $derived.by((): RowGroup | null => {
		if (!pageMode || !scoped) return null;
		const rows: Row[] = [{ kind: 'page', id: 'page-scope' }];
		if (query.trim()) rows.push({ kind: 'clear', id: 'page-clear' });
		return { key: 'this-page', heading: scoped.label(), rows };
	});

	/** "Go to" — the permission-filtered nav tables the layout handed down.
	 *  These are shell objects, so local substring matching is allowed. */
	const sectionGroup = $derived.by((): RowGroup | null => {
		const q = query.trim().toLowerCase();
		const all = [...sections, ...overflow.flatMap((g) => g.items)];
		const rows: Row[] = all
			// The label is a message ACCESSOR — call it here so the row carries the
			// label in the locale that is active while the palette renders.
			.map((s) => ({ kind: 'section' as const, id: `section-${s.href}`, href: s.href, title: s.label() }))
			.filter((r) => !q || r.title.toLowerCase().includes(q));
		return rows.length ? { key: 'go-to', heading: m.search_group_goto(), rows } : null;
	});

	/** "This shell" — windows, terminals and drafts of THIS tab. Local substring
	 *  matching is allowed here and only here (these never touch the RPC). */
	const shellGroup = $derived.by((): RowGroup | null => {
		const q = query.trim().toLowerCase();
		const hit = (title: string) => !q || title.toLowerCase().includes(q);
		const rows: Row[] = [
			...shell.panels
				.filter((p) => hit(p.title))
				.map((p) => ({
					kind: 'panel' as const,
					id: `panel-${p.id}`,
					refId: p.id,
					title: p.title,
					sub: p.minimized ? m.search_shell_restore() : m.search_shell_focus()
				})),
			...shell.terminal.sessions
				.filter((s) => hit(s.name))
				.map((s) => ({
					kind: 'terminal' as const,
					id: `terminal-${s.id}`,
					refId: s.id,
					title: s.name,
					sub: m.search_shell_focus()
				})),
			...shell.drafts
				.filter((d) => hit(d.title))
				.map((d) => ({
					kind: 'draft' as const,
					id: `draft-${d.id}`,
					refId: d.id,
					title: d.title,
					sub: d.subtitle || m.search_shell_restore()
				}))
		];
		return rows.length ? { key: 'this-shell', heading: m.search_group_shell(), rows } : null;
	});

	const groups = $derived.by((): RowGroup[] => {
		// In page mode the list behind the pill IS the result set, so the palette
		// shows no entity rows of its own — only the page row and jump targets.
		const out: RowGroup[] = pageMode ? [] : [...entityGroups];
		if (pageGroup) out.push(pageGroup);
		if (!pageMode && nextRow.length) out.push({ key: 'paging', heading: '', rows: nextRow });
		if (sectionGroup) out.push(sectionGroup);
		if (shellGroup) out.push(shellGroup);
		return out;
	});

	/** Flat row order — what ↑↓ walks and aria-activedescendant points at. */
	const flat = $derived(groups.flatMap((g) => g.rows));

	$effect(() => {
		if (sel > flat.length - 1) sel = Math.max(0, flat.length - 1);
	});

	// ── the RPC ───────────────────────────────────────────────────────────────
	async function runSearch(q: string, scope: SearchScope, pageToken = '') {
		const seq = ++issued;
		searching = true;
		failed = false;
		try {
			const res = await apiClient.search(q, scope, PAGE_SIZE, pageToken);
			if (seq !== issued) return; // a newer query already owns the list
			const page = res.results ?? [];
			results = pageToken ? [...results, ...page] : page;
			nextPageToken = res.nextPageToken ?? '';
			totalCount = res.totalCount ?? 0;
		} catch (error) {
			if (seq !== issued) return;
			console.warn('Search failed', error);
			if (!pageToken) results = [];
			nextPageToken = '';
			totalCount = 0;
			failed = true;
		} finally {
			if (seq === issued) searching = false;
		}
	}

	function clearResults() {
		issued++; // orphan any in-flight response
		results = [];
		nextPageToken = '';
		totalCount = 0;
		searching = false;
		failed = false;
	}

	function schedule(q: string, scope: SearchScope) {
		clearTimeout(debounce);
		if (!q.trim()) {
			clearResults();
			return;
		}
		debounce = setTimeout(() => runSearch(q, scope), 200);
	}

	function onInput(value: string) {
		query = value;
		sel = 0;
		// Page mode drives the page's own list state — the same setSearch its
		// removed input drove — and issues no RPC of its own.
		if (pageMode && scoped) {
			clearTimeout(debounce);
			clearResults();
			scoped.setQuery(value);
			return;
		}
		schedule(value, activeFacet.scope);
	}

	/** ⇥ / a chip click. An explicit act, so it re-runs the RPC immediately —
	 *  no debounce — and drops the previous scope's rows and cursor. Landing on
	 *  the page facet issues nothing: the page is already showing its answer. */
	function setFacet(next: number) {
		facet = (next + ring.length) % ring.length;
		sel = 0;
		clearTimeout(debounce);
		clearResults();
		if (!pageMode && query.trim()) runSearch(query, ring[facet].scope);
	}

	function showNext() {
		if (!nextPageToken) return;
		runSearch(query, activeFacet.scope, nextPageToken);
	}

	// ── activation ────────────────────────────────────────────────────────────
	function close() {
		open = false;
	}

	function openResult(r: SearchResult) {
		const target = FACET_BY_SCOPE.get(r.scope);
		if (!target || !target.route) return; // never navigate on a scope we don't know
		close();
		goto(
			target.detail
				? `${target.route}/${r.id}`
				: `${target.route}?query=${encodeURIComponent(r.name || r.id)}`
		);
	}

	function resumeDraft(id: string) {
		const route = restoreDraft(id);
		if (route) void goto(route);
	}

	function activate(row: Row) {
		if (row.kind === 'entity') openResult(row.result);
		else if (row.kind === 'next') showNext();
		// The page row is "done": the filter is already applied to the list behind
		// the pill, so ↵ just gets the pill out of the way. It never clears.
		else if (row.kind === 'page') close();
		else if (row.kind === 'clear') {
			query = '';
			sel = 0;
			scoped?.clear();
		} else if (row.kind === 'section') (close(), goto(row.href));
		else if (row.kind === 'panel') (restorePanel(row.refId), close());
		else if (row.kind === 'terminal') (focusSession(row.refId), close());
		// A draft whose surface is no longer mounted restores by going HOME first:
		// the store hands back the route and this chrome navigates. Re-entering the
		// parked context instead would revive closures over an unmounted page.
		else (resumeDraft(row.refId), close());
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') (e.preventDefault(), (sel = Math.min(sel + 1, flat.length - 1)));
		else if (e.key === 'ArrowUp') (e.preventDefault(), (sel = Math.max(sel - 1, 0)));
		else if (e.key === 'Enter') (e.preventDefault(), flat[sel] && activate(flat[sel]));
		else if (e.key === 'Escape') (e.preventDefault(), close());
		else if (e.key === 'Tab') (e.preventDefault(), setFacet(facet + (e.shiftKey ? -1 : 1)));
		// Tab is consumed by the facet ring, and the chips are not tab stops, so
		// focus can never leave the palette while it is open.
	}

	// Opening captures the registered page ONCE (untracked: reading the page's
	// live query here must not make this effect re-run on every keystroke and
	// reseed the box). Closing resets only the palette's OWN state — the page's
	// query is the page's, and Esc must leave it exactly as it stands.
	$effect(() => {
		const isOpen = open;
		untrack(() => {
			if (isOpen) {
				const reg = activePageSearch();
				scoped = reg;
				facet = 0; // ring[0] is the page facet when one is registered
				query = reg ? reg.query : '';
				sel = 0;
				clearResults();
				queueMicrotask(() => input?.focus());
			} else {
				clearTimeout(debounce);
				scoped = null;
				query = '';
				facet = 0;
				sel = 0;
				clearResults();
			}
		});
	});

	// ── row presentation ──────────────────────────────────────────────────────
	const rtf = $derived(new Intl.RelativeTimeFormat(getLocale(), { numeric: 'auto' }));

	function relative(seconds: string | undefined): string {
		if (!seconds) return '';
		const then = parseInt(seconds, 10);
		if (!Number.isFinite(then) || then <= 0) return '';
		const delta = then - Math.floor(Date.now() / 1000);
		const abs = Math.abs(delta);
		if (abs < 60) return rtf.format(Math.round(delta), 'second');
		if (abs < 3600) return rtf.format(Math.round(delta / 60), 'minute');
		if (abs < 86400) return rtf.format(Math.round(delta / 3600), 'hour');
		return rtf.format(Math.round(delta / 86400), 'day');
	}

	/** A dot only where the entity really carries a status field. Devices use
	 *  the same five-minute heartbeat window as `searchResultToDevice`;
	 *  executions use their status enum; a user's only status is `disabled`.
	 *
	 *  The tone is also the row's status WORD (`TONE_LABEL`), so the buckets have
	 *  to mean what the fleet's words say: `idle` is "never seen" and nothing
	 *  else, a device that was seen but has missed the window is unreachable —
	 *  `crit` — exactly as `deviceTone` classifies it on the fleet home. */
	function tone(r: SearchResult): FleetTone | null {
		const f = r.fields;
		if (r.scope === SearchScope.DEVICES) {
			const seen = parseInt(f['last_seen_at'] ?? '0', 10);
			if (!Number.isFinite(seen) || seen <= 0) return 'idle';
			return Math.floor(Date.now() / 1000) - seen < 300 ? 'ok' : 'crit';
		}
		if (r.scope === SearchScope.USERS && f['disabled'] === 'true') return 'idle';
		return null;
	}

	function primary(r: SearchResult): string {
		const f = r.fields;
		return r.name || f['hostname'] || f['name'] || f['email'] || f['event_type'] || r.id;
	}

	/** The muted trailing line: only meta the RPC actually returned. */
	function secondary(r: SearchResult): string {
		const f = r.fields;
		const parts: (string | undefined)[] = [];
		switch (r.scope) {
			case SearchScope.DEVICES: {
				// The status WORD leads the device meta line ("ok · 12 s"). The row's
				// tone dot is a colour, and a colour is never the only carrier of a
				// status — a monochrome screen must read the same fact.
				const t = tone(r);
				if (t) parts.push(TONE_LABEL[t]());
				parts.push([f['os_name'], f['os_version']].filter(Boolean).join(' ') || f['agent_version']);
				parts.push(relative(f['last_seen_at']));
				break;
			}
			case SearchScope.USERS:
				parts.push(f['display_name'] || f['linux_username']);
				break;
			case SearchScope.AUDIT_EVENTS:
				parts.push([f['actor_type'], f['stream_type']].filter(Boolean).join(' · '));
				parts.push(relative(f['occurred_at']));
				break;
			default:
				parts.push(r.description || f['description']);
				if (r.memberCount > 0) parts.push(m.search_member_count({ count: r.memberCount }));
		}
		return parts.filter(Boolean).join(' · ');
	}

	const shellIcon = { panel: AppWindow, terminal: SquareTerminal, draft: PencilLine };
	const sectionIcon = $derived(
		new Map([...sections, ...overflow.flatMap((g) => g.items)].map((s) => [s.href, s.icon]))
	);
</script>

{#if open}
	<!-- No backdrop and no dialog role: this IS the pill's search mode, not a
	     modal over the page. The list the operator is filtering has to stay
	     visible behind it — that is the whole point of absorbing search into the
	     pill. The morph bar owns the surrounding surface and the dismiss layer. -->
	<div
		role="search"
		aria-label={m.search_dialog_label()}
		data-testid="global-search"
		data-page-mode={pageMode ? 'true' : 'false'}
		class="flex max-h-[62vh] w-[min(36rem,calc(100vw-3rem))] flex-col overflow-hidden text-foreground"
	>
			<!-- query -->
			<div class="flex items-center gap-2 border-b px-3.5 py-3">
				<Search class="h-4 w-4 shrink-0 text-muted-foreground" />
				<input
					bind:this={input}
					data-tour="palette-input"
					role="combobox"
					aria-expanded="true"
					aria-controls="palette-listbox"
					aria-activedescendant={flat[sel]?.id}
					aria-label={m.search_dialog_label()}
					autocomplete="off"
					spellcheck="false"
					class="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
					placeholder={pageMode && scoped ? m.search_page_placeholder({ scope: scoped.label() }) : m.search_placeholder()}
					value={query}
					oninput={(e) => onInput(e.currentTarget.value)}
					{onkeydown}
				/>
				<kbd class="rounded border border-border-strong bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-faint">esc</kbd>
			</div>

			<!-- facets: the registered page (if any), then the ten server scopes plus
			     "all". Not tab stops — ⇥ cycles them, so the palette keeps focus and
			     the footer hint stays true. -->
			<div
				class="flex flex-wrap gap-1.5 border-b px-3 py-2"
				data-tour="palette-facets"
				data-testid="palette-facets"
				role="group"
				aria-label={m.search_facets_label()}
			>
				{#each ring as f, i (f.key)}
					<button
						type="button"
						tabindex="-1"
						data-testid="palette-facet"
						data-scope={f.scope}
						data-page-facet={f.page ? 'true' : undefined}
						aria-pressed={i === facet}
						onclick={() => setFacet(i)}
						class="rounded-full border px-2 py-0.5 font-mono text-[10px] {i === facet
							? 'border-accent-ink text-accent-ink'
							: 'border-border text-muted-foreground hover:text-foreground'} {f.page
							? 'border-dashed'
							: ''}"
					>
						{f.label()}
					</button>
				{/each}
			</div>

			<!-- results -->
			<ul id="palette-listbox" role="listbox" aria-label={m.search_results_label()} class="min-h-0 flex-1 overflow-y-auto p-1.5">
				{#each groups as g (g.key)}
					<li role="presentation">
						{#if g.heading}
							<div
								id="palette-head-{g.key}"
								class="px-2 pb-1 pt-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-faint"
							>
								{g.heading}
							</div>
						{/if}
						<ul role="group" aria-labelledby={g.heading ? `palette-head-${g.key}` : undefined} class="contents">
							{#each g.rows as row (row.id)}
								{@const i = flat.indexOf(row)}
								<!-- Options in an aria-activedescendant combobox must NOT be focus
								     stops: the keyboard contract lives on the input (↑↓ ↵ ⇥ esc),
								     which is why there is no per-row key handler. -->
								<!-- svelte-ignore a11y_click_events_have_key_events -->
								<li
									id={row.id}
									role="option"
									aria-selected={i === sel}
									data-testid="palette-row"
									data-kind={row.kind}
									onmouseenter={() => (sel = i)}
									onmousedown={(e) => e.preventDefault()}
									onclick={() => activate(row)}
									class="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm {i === sel
										? 'bg-accent text-accent-foreground'
										: ''}"
								>
									{#if row.kind === 'entity'}
										{@const t = tone(row.result)}
										{#if t}
											<!-- The dot carries a real accessible name, from the same
											     tone vocabulary the fleet tiles use. -->
											<span role="img" aria-label={TONE_LABEL[t]()} data-testid="palette-dot" data-tone={t} class="h-1.5 w-1.5 shrink-0 rounded-full {TONE_FILL[t]}"></span>
										{:else}
											<span class="h-1.5 w-1.5 shrink-0"></span>
										{/if}
										<span class="truncate font-mono">{primary(row.result)}</span>
										{@const meta = secondary(row.result)}
										{#if meta}
											<span class="ml-auto shrink-0 truncate pl-3 text-xs text-muted-foreground">{meta}</span>
										{/if}
									{:else if row.kind === 'next'}
										<ArrowRight class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
										<span class="text-accent-ink" data-testid="palette-show-next">
											{totalCount > results.length
												? m.search_show_next({ count: totalCount - results.length })
												: m.search_show_more()}
										</span>
										<span class="ml-auto shrink-0 font-mono text-xs text-faint">{m.search_keyset()}</span>
									{:else if row.kind === 'page'}
										<ListFilter class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
										<span class="truncate" data-testid="palette-page-row">
											{m.search_page_row({ scope: scoped?.label() ?? '' })}
										</span>
										<span class="ml-auto shrink-0 truncate pl-3 text-xs text-muted-foreground">
											{m.search_page_row_hint()}
										</span>
									{:else if row.kind === 'clear'}
										<X class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
										<span class="truncate" data-testid="palette-page-clear">{m.search_page_clear()}</span>
									{:else if row.kind === 'section'}
										{@const SectionIcon = sectionIcon.get(row.href)}
										{#if SectionIcon}<SectionIcon class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />{/if}
										<span class="truncate">{row.title}</span>
										<span class="ml-auto shrink-0 truncate pl-3 text-xs text-muted-foreground">{m.shell_row_section()}</span>
									{:else}
										{@const Icon = shellIcon[row.kind]}
										<Icon class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
										<span class="truncate">{row.title}</span>
										<span class="ml-auto shrink-0 truncate pl-3 text-xs text-muted-foreground">{row.sub}</span>
									{/if}
								</li>
							{/each}
						</ul>
					</li>
				{:else}
					<li class="px-2.5 py-6 text-center text-sm text-muted-foreground" role="presentation">
						{#if failed}
							{m.search_failed()}
						{:else if searching}
							{m.search_searching()}
						{:else if query.trim()}
							{m.search_no_results()}
						{:else}
							{m.search_hint_empty()}
						{/if}
					</li>
				{/each}
			</ul>

			<!-- footer: the keyboard contract on the left, the search contract on
			     the right — prefix matching, server order, keyset paging. In page
			     mode there is no RPC to describe, so it says whose list is moving. -->
			<div class="flex items-center justify-between gap-3 border-t px-3 py-2 text-[11px] text-faint">
				<span>{m.search_footer_keys()}</span>
				<span class="truncate font-mono" data-testid="palette-footer-contract">
					{pageMode ? m.search_footer_page() : m.search_footer_contract()}
				</span>
			</div>
	</div>
{/if}
