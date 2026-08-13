<script lang="ts" module>
	import type { FleetTone } from '$lib/components/fleet';
	import type { QueryEditorState } from '$lib/components/query-builder.svelte';
	import * as msg from '$lib/paraglide/messages';

	/** The rule's one-line caption: compiled query plus live match count.
	 *
	 *  Exported because the OWNER of the pill context renders it — this editor no
	 *  longer holds a context of its own. There is one implementation so the
	 *  caption cannot drift from what the editor actually computed. */
	export function ruleSubtext(
		state: QueryEditorState,
		kind: 'device' | 'user'
	): { text: string; tone: 'neutral' | 'warn' } {
		if (!state.complete) return { text: msg.query_incomplete(), tone: 'warn' };
		if (state.validating) {
			return { text: `${msg.query_counting()} · ${state.text}`, tone: 'neutral' };
		}
		if (state.valid === false) return { text: `${state.error} · ${state.text}`, tone: 'warn' };
		if (state.count === null) {
			return {
				text: state.text || msg.query_future_scope_empty_query(),
				tone: state.text ? 'neutral' : 'warn'
			};
		}
		const lead =
			kind === 'user'
				? msg.query_match_count_users({ count: state.count })
				: msg.query_match_count_devices({ count: state.count });
		// The empty match-all rule is counted too now — it has no query text to
		// append, so the caption is the count alone rather than "N match · ".
		return { text: state.text ? `${lead} · ${state.text}` : lead, tone: 'neutral' };
	}

	/** One matching entity, already resolved from real reads by the owner. */
	export interface RulePreviewRow {
		id: string;
		/** Hostname or e-mail — whatever names the entity. */
		primary: string;
		/** Real attributes only (labels, OS, agent version); never invented. */
		attributes: string[];
		/** Attributes the owner capped away, so the row says it is showing a
		 *  subset instead of implying the entity carries only these. */
		hiddenAttributes?: number;
		tone: FleetTone;
	}
</script>

<script lang="ts">
	// The Rule tab: chips edit the rule, the pill carries the commit.
	//
	// Three things are deliberately split here:
	//   · the compiled query + live match count go to the pill's subtext, so
	//     there is exactly ONE copy of them on screen (B3's "single source of
	//     truth"), never a second copy inside the card;
	//   · the warn strip states the standing-rule consequence while editing;
	//   · the save itself is gated by a real confirm, because a banner you can
	//     scroll past is not an acknowledgement.
	//
	// The preview list is the group's CURRENT matches, read from the group. No
	// RPC returns the entities matching an unsaved draft — only its count — so
	// while the draft is dirty the list is labelled as the applied rule's result
	// rather than pretending to preview the edit.
	import { type Snippet } from 'svelte';
	import QueryBuilder, { type PropertyGroup } from '$lib/components/query-builder.svelte';
	import { Tile, Chip } from '$lib/components/fleet';
	import { TriangleAlert } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		kind: 'device' | 'user';
		/** The rule as STORED. The owner rebases this after a save. */
		savedQuery: string;
		/** The edit buffer. Owned by the page, because the page owns the single
		 *  pill context that commits it together with the group's name — saving a
		 *  renamed group with an edited rule used to take two separate saves. */
		draft: string;
		/** False while the group is still static — saving converts it. */
		isDynamic: boolean;
		propertyGroups?: PropertyGroup[];
		propertyExampleOverrides?: Record<string, () => string>;
		advancedPlaceholder?: string;
		advancedHint?: string;
		/** Current matches, resolved from real reads. */
		rows?: RulePreviewRow[];
		/** Total matches behind `rows`, for the "+N more" tail. */
		total?: number;
		/** Reports validation/count upward so the owner's context can gate its
		 *  commit and caption itself. */
		onstate?: (state: QueryEditorState) => void;
		extra?: Snippet;
	}

	let {
		kind,
		savedQuery,
		draft = $bindable(),
		isDynamic,
		propertyGroups,
		propertyExampleOverrides,
		advancedPlaceholder,
		advancedHint,
		rows = [],
		total = 0,
		onstate,
		extra
	}: Props = $props();

	const PREVIEW_ROWS = 5;

	// svelte-ignore state_referenced_locally
	let editorState = $state<QueryEditorState>({
		text: savedQuery,
		complete: true,
		valid: null,
		count: null,
		error: '',
		validating: false
	});

	const dirty = $derived(draft !== savedQuery);
	const shown = $derived(rows.slice(0, PREVIEW_ROWS));
	const more = $derived(Math.max(0, (total || rows.length) - shown.length));
</script>

<div class="space-y-3" data-testid="rule-tab">
	<QueryBuilder
		bind:query={draft}
		{kind}
		{propertyGroups}
		{propertyExampleOverrides}
		{advancedPlaceholder}
		{advancedHint}
		inlineStatus={false}
		onstate={(s) => {
			editorState = s;
			onstate?.(s);
		}}
	>
		{#snippet banner()}
			<div
				class="flex items-start gap-2 border-b border-hair bg-warn-soft px-3 py-2 text-xs text-warn"
				data-testid="rule-futurebar"
			>
				<TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
				<span>
					{isDynamic ? m.query_futurebar() : m.query_futurebar_convert()}
					{#if !editorState.text}
						<span class="font-semibold">
							{kind === 'user'
								? m.user_groups_empty_query_warning()
								: m.device_groups_empty_query_warning()}
						</span>
					{/if}
				</span>
			</div>
		{/snippet}

		{#snippet preview()}
			<div class="px-3 pt-2 pb-3" data-testid="rule-preview">
				<div
					class="flex items-baseline justify-between font-mono text-[0.62rem] tracking-[0.08em] text-faint uppercase"
				>
					<span>{m.query_preview_title()}</span>
					<span>{dirty ? m.query_preview_applied_note() : m.query_preview_live_note()}</span>
				</div>
				{#if shown.length === 0}
					<p class="py-3 text-center text-xs text-muted-foreground">{m.query_preview_empty()}</p>
				{:else}
					<div class="mt-1">
						{#each shown as row (row.id)}
							<div class="flex items-center gap-2.5 border-t border-hair py-1.5 text-sm">
								<span class="w-3.5 shrink-0"><Tile tone={row.tone} label={row.primary} /></span>
								<span class="truncate font-mono text-[0.82rem]">{row.primary}</span>
								<span class="flex min-w-0 flex-wrap items-center gap-1">
									{#each row.attributes as attribute (attribute)}
										<Chip tone="idle" label={attribute} />
									{/each}
									{#if row.hiddenAttributes}
										<span class="font-mono text-[0.66rem] text-faint">
											{m.device_labels_more({ count: row.hiddenAttributes })}
										</span>
									{/if}
								</span>
							</div>
						{/each}
					</div>
					{#if more > 0}
						<p class="pt-1.5 text-[0.7rem] text-faint">{m.query_preview_more({ count: more })}</p>
					{/if}
				{/if}
			</div>
		{/snippet}
	</QueryBuilder>

	{#if extra}{@render extra()}{/if}
</div>
