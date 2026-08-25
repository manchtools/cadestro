<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import QueryBuilder, { type QueryEditorState } from '$lib/components/query-builder.svelte';
	import { TriangleAlert } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { RuleGroup } from './assign-data';

	let {
		query = $bindable(''),
		groupName = $bindable(''),
		state,
		savedGroup = null,
		savedGroupHref = '',
		savingGroup = false,
		error = null,
		onstate
	}: {
		query: string;
		groupName: string;
		/** The editor's published state — owned by the page, shown here read-only. */
		state: QueryEditorState;
		/** A dynamic group already created from this draft, if any. */
		savedGroup?: RuleGroup | null;
		savedGroupHref?: string;
		savingGroup?: boolean;
		error?: string | null;
		onstate: (next: QueryEditorState) => void;
	} = $props();

	const nameMissing = $derived(groupName.trim().length === 0);
</script>

<div class="flex min-w-0 flex-col gap-3 bg-sunken p-4" data-testid="assign-rule-stage">
	<div
		class="flex items-baseline justify-between gap-2 font-mono text-[0.66rem] tracking-[0.08em] text-faint uppercase"
	>
		<span>{m.assign_rule_stage_label()}</span>
	</div>

	<div class="grid gap-1">
		<label for="assign-rule-name" class="text-xs font-medium">{m.assign_rule_name_label()}</label>
		<Input
			id="assign-rule-name"
			bind:value={groupName}
			data-testid="assign-rule-name"
			placeholder={m.assign_rule_name_placeholder()}
			class="h-8 font-mono text-sm"
		/>
		<p class="text-xs text-faint">
			{nameMissing ? m.assign_rule_name_required() : m.assign_rule_name_hint()}
		</p>
	</div>

	<QueryBuilder bind:query kind="device" inlineStatus={false} {onstate}>
		{#snippet banner()}
			<!-- The standing-rule warning while editing. The acknowledgement itself is
			     the confirm in front of the commit, not this strip. -->
			<div
				class="flex items-start gap-2 border-b border-hair bg-warn-soft px-3 py-2 text-xs text-warn"
				data-testid="assign-rule-futurebar"
			>
				<TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
				<span>
					{m.query_futurebar()}
				</span>
			</div>
		{/snippet}

		{#snippet preview()}
			<div class="px-3 pt-2 pb-3" data-testid="assign-rule-preview">
				<div
					class="flex items-baseline justify-between font-mono text-[0.62rem] tracking-[0.08em] text-faint uppercase"
				>
					<span>{m.query_preview_title()}</span>
				</div>
				<p class="pt-1.5 text-sm" data-testid="assign-rule-count">
					{#if !state.text.trim()}
						<span class="text-warn">{m.query_incomplete()}</span>
					{:else if state.validating}
						<span class="text-muted-foreground">{m.query_counting()}</span>
					{:else if !state.valid}
						<span class="text-crit">{state.error}</span>
					{:else if state.count !== null}
						<span class="font-semibold">{m.query_match_count_devices({ count: state.count })}</span>
					{:else}
						<span class="text-muted-foreground">{m.assign_rule_preview_pending()}</span>
					{/if}
				</p>
				<p class="pt-1 text-[0.7rem] text-faint">{m.assign_rule_preview_note()}</p>
			</div>
		{/snippet}
	</QueryBuilder>

	<p class="text-xs text-faint">{m.assign_rule_assignment_hint()}</p>

	{#if savingGroup}
		<p class="text-xs text-muted-foreground" data-testid="assign-rule-saving">
			{m.assign_rule_saving_group()}
		</p>
	{/if}

	{#if savedGroup}
		<div
			class="flex flex-wrap items-center gap-2 rounded-lg border border-ok/40 bg-ok-soft px-3 py-2 text-xs text-ok"
			data-testid="assign-rule-saved"
		>
			<span>{m.assign_rule_saved({ name: savedGroup.name })}</span>
			<a class="font-medium underline" href={savedGroupHref}>{m.assign_rule_saved_link()}</a>
		</div>
	{/if}

	{#if error}
		<p
			class="rounded-lg border border-crit/40 bg-crit-soft px-3 py-2 text-xs text-crit"
			data-testid="assign-rule-error"
		>
			{error}
		</p>
	{/if}
</div>
