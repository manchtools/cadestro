<script lang="ts" module>
	import type { FleetTone } from '$lib/components/fleet';
	import type { QueryEditorState } from '$lib/components/query-builder.svelte';
	import * as msg from '$lib/paraglide/messages';

	export function ruleSubtext(
		state: QueryEditorState,
		kind: 'device' | 'user'
	): { text: string; tone: 'neutral' | 'warn' } {
		if (!state.text.trim()) return { text: msg.query_incomplete(), tone: 'warn' };
		if (state.validating) return { text: `${msg.query_counting()} · ${state.text}`, tone: 'neutral' };
		if (!state.valid) return { text: `${state.error} · ${state.text}`, tone: 'warn' };
		if (state.count === null) return { text: state.text, tone: 'neutral' };
		const lead =
			kind === 'user'
				? msg.query_match_count_users({ count: state.count })
				: msg.query_match_count_devices({ count: state.count });
		return { text: `${lead} · ${state.text}`, tone: 'neutral' };
	}

	export interface RulePreviewRow {
		id: string;
		primary: string;
		attributes: string[];
		hiddenAttributes?: number;
		tone: FleetTone;
	}
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import QueryBuilder from '$lib/components/query-builder.svelte';
	import { Tile, Chip } from '$lib/components/fleet';
	import { TriangleAlert } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	type Props = {
		kind: 'device' | 'user';
		savedQuery: string;
		draft: string;
		isDynamic: boolean;
		rows?: RulePreviewRow[];
		total?: number;
		onstate?: (state: QueryEditorState) => void;
		extra?: Snippet;
	};

	let { kind, savedQuery, draft = $bindable(), isDynamic, rows = [], total = 0, onstate, extra }: Props =
		$props();
	const PREVIEW_ROWS = 5;
	let editorState = $state<QueryEditorState>({
		text: '',
		valid: false,
		count: null,
		error: m.query_incomplete(),
		validating: false
	});
	const dirty = $derived(draft !== savedQuery);
	const shown = $derived(rows.slice(0, PREVIEW_ROWS));
	const more = $derived(Math.max(0, (total || rows.length) - shown.length));
</script>

<div class="space-y-3" data-testid="rule-tab" data-tour="group-rule-editor">
	<QueryBuilder
		bind:query={draft}
		{kind}
		inlineStatus={false}
		onstate={(state) => {
			editorState = state;
			onstate?.(state);
		}}
	>
		{#snippet banner()}
			<div
				class="flex items-start gap-2 border-b border-hair bg-warn-soft px-3 py-2 text-xs text-warn"
				data-testid="rule-futurebar"
			>
				<TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
				<span>{isDynamic ? m.query_futurebar() : m.query_futurebar_convert()}</span>
			</div>
		{/snippet}

		{#snippet preview()}
			<div class="px-3 pt-2 pb-3" data-testid="rule-preview">
				<div class="flex items-baseline justify-between font-mono text-[0.62rem] tracking-[0.08em] text-faint uppercase">
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
					{#if more > 0}<p class="pt-1.5 text-[0.7rem] text-faint">{m.query_preview_more({ count: more })}</p>{/if}
				{/if}
			</div>
		{/snippet}
	</QueryBuilder>
	{#if extra}{@render extra()}{/if}
</div>
