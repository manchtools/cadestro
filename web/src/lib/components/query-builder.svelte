<script lang="ts" module>
	export type PropertyDef = { value: string; label: string; numeric?: boolean };
	export type PropertyGroup = { label: string; items: PropertyDef[] };

	/** What the editor knows about the draft, pushed to whoever owns the commit
	 *  surface (the context pill on a detail page, an inline line in a create
	 *  dialog). `text` is byte-for-byte what the RPC will receive. */
	export interface QueryEditorState {
		text: string;
		/** Every chip filled in — the only state a commit may be offered from. */
		complete: boolean;
		/** Server verdict. Null until the debounced validation lands. */
		valid: boolean | null;
		/** Server match count for `text`; null when unknown or not yet asked. */
		count: number | null;
		error: string;
		validating: boolean;
	}
</script>

<script lang="ts">
	// B3 — chips ARE the editor.
	//
	// The rule is drawn as mono chips (key · operator · value) joined by AND/OR
	// chiplets, with parenthesised alternatives inside dashed brackets. The
	// authority is still the query STRING: every edit recompiles through
	// query-model.ts and the recompiled text is what validation and the save RPC
	// see, so the drawing can never drift from the rule.
	//
	// The live match count comes from ValidateDynamicQuery / ValidateUserGroupQuery
	// — the only RPCs that count an ARBITRARY draft query. EvaluateDynamicGroup
	// takes a group id, re-runs the SAVED query and MUTATES membership, so it is
	// not a preview and is never called from here.
	import { onDestroy, type Snippet } from 'svelte';
	import { apiClient } from '$lib/sdk';
	import * as Select from '$lib/components/ui/select';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Plus, Trash2, X } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import {
		LABEL_CUSTOM,
		compileCond,
		compileQuery,
		emptyCond,
		emptyModel,
		isUnary,
		modelComplete,
		parseQuery,
		resolveProperty,
		type Cond,
		type Join,
		type QueryModel
	} from './query-model';

	const defaultPropertyGroups: PropertyGroup[] = [
		{
			label: m.qb_group_device_info(),
			items: [
				{ value: 'device.hostname', label: m.qb_prop_hostname() },
				{ value: 'device.name', label: m.qb_prop_name() },
				{ value: 'device.os', label: m.qb_prop_os() },
				{ value: 'device.os_version', label: m.qb_prop_os_version() },
				{ value: 'device.os_major', label: m.qb_prop_os_major(), numeric: true },
				{ value: 'device.os_minor', label: m.qb_prop_os_minor(), numeric: true },
				{ value: 'device.os_arch', label: m.qb_prop_os_arch() },
				{ value: 'device.os_platform', label: m.qb_prop_os_platform() },
				{ value: 'device.kernel', label: m.qb_prop_kernel() }
			]
		},
		{
			label: m.qb_group_hardware(),
			items: [
				{ value: 'device.cpu_type', label: m.qb_prop_cpu_type() },
				{ value: 'device.cpu_brand', label: m.qb_prop_cpu_brand() },
				{ value: 'device.cpu_cores', label: m.qb_prop_cpu_cores(), numeric: true },
				{ value: 'device.cpu_logical_cores', label: m.qb_prop_cpu_logical_cores(), numeric: true },
				{ value: 'device.memory_total', label: m.qb_prop_memory_total(), numeric: true }
			]
		},
		{
			label: m.qb_group_membership(),
			items: [{ value: 'device.group', label: m.qb_prop_group() }]
		},
		{
			label: m.qb_group_labels(),
			items: [{ value: LABEL_CUSTOM, label: m.qb_prop_custom_label() }]
		}
	];

	const defaultPropertyExamples: Record<string, () => string> = {
		'device.hostname': () => m.qb_hint_prop_hostname(),
		'device.name': () => m.qb_hint_prop_name(),
		'device.os': () => m.qb_hint_prop_os(),
		'device.os_version': () => m.qb_hint_prop_os_version(),
		'device.os_major': () => m.qb_hint_prop_os_major(),
		'device.os_minor': () => m.qb_hint_prop_os_minor(),
		'device.os_arch': () => m.qb_hint_prop_os_arch(),
		'device.os_platform': () => m.qb_hint_prop_os_platform(),
		'device.kernel': () => m.qb_hint_prop_kernel(),
		'device.cpu_type': () => m.qb_hint_prop_cpu_type(),
		'device.cpu_brand': () => m.qb_hint_prop_cpu_brand(),
		'device.cpu_cores': () => m.qb_hint_prop_cpu_cores(),
		'device.cpu_logical_cores': () => m.qb_hint_prop_cpu_logical_cores(),
		'device.memory_total': () => m.qb_hint_prop_memory_total(),
		'device.group': () => m.qb_hint_prop_group(),
		[LABEL_CUSTOM]: () => m.qb_hint_prop_label()
	};

	interface Props {
		query: string;
		propertyGroups?: PropertyGroup[];
		propertyExampleOverrides?: Record<string, () => string>;
		advancedPlaceholder?: string;
		advancedHint?: string;
		/** Which validate RPC counts the draft. */
		kind?: 'device' | 'user';
		/** Push the compiled text + live count somewhere (the pill, usually). */
		onstate?: (state: QueryEditorState) => void;
		/** Render the compiled text + count inside the card. Off when the pill
		 *  subtext is carrying them, so there is exactly one copy on screen. */
		inlineStatus?: boolean;
		/** Standing-rule warning strip, injected by the owner. */
		banner?: Snippet;
		/** Matching-entity preview, injected by the owner. */
		preview?: Snippet;
	}

	let {
		query = $bindable(''),
		propertyGroups = defaultPropertyGroups,
		propertyExampleOverrides,
		advancedPlaceholder = m.device_groups_dynamic_query_placeholder(),
		advancedHint = m.device_groups_query_operators(),
		kind = 'device',
		onstate,
		inlineStatus = true,
		banner,
		preview
	}: Props = $props();

	const allProperties = $derived(propertyGroups.flatMap((g) => g.items));
	const hasCustomLabels = $derived(allProperties.some((p) => p.value === LABEL_CUSTOM));
	const propertyExamples: Record<string, () => string> = $derived(
		propertyExampleOverrides ?? defaultPropertyExamples
	);

	type OperatorDef = { value: string; label: string };

	const stringOperators: OperatorDef[] = [
		{ value: 'equals', label: m.qb_op_equals() },
		{ value: 'notEquals', label: m.qb_op_not_equals() },
		{ value: 'contains', label: m.qb_op_contains() },
		{ value: 'notContains', label: m.qb_op_not_contains() },
		{ value: 'startsWith', label: m.qb_op_starts_with() },
		{ value: 'endsWith', label: m.qb_op_ends_with() },
		{ value: 'in', label: m.qb_op_in() },
		{ value: 'notIn', label: m.qb_op_not_in() },
		{ value: 'matches', label: m.qb_op_matches() },
		{ value: 'notMatches', label: m.qb_op_not_matches() },
		{ value: 'exists', label: m.qb_op_exists() },
		{ value: 'notExists', label: m.qb_op_not_exists() }
	];

	const numericOperators: OperatorDef[] = [
		{ value: 'equals', label: m.qb_op_equals() },
		{ value: 'notEquals', label: m.qb_op_not_equals() },
		{ value: 'greaterThan', label: m.qb_op_greater_than() },
		{ value: 'lessThan', label: m.qb_op_less_than() },
		{ value: 'exists', label: m.qb_op_exists() },
		{ value: 'notExists', label: m.qb_op_not_exists() }
	];

	// ── editor state ─────────────────────────────────────────────────────────
	let model = $state<QueryModel>(emptyModel());
	/** The chips cannot draw every legal query (nested groups, NOT); those stay
	 *  editable as text so the stored rule is never silently rewritten. */
	let rawMode = $state(false);
	let undrawable = $state(false);
	/** Which chip's editor row is open: node index, and the position inside a
	 *  group (-1 for a bare condition). */
	let openChip = $state<{ node: number; sub: number } | null>(null);

	let validation = $state<{ valid: boolean; error: string; count: number | null } | null>(null);
	let validating = $state(false);

	/** The last text this editor itself produced — anything else on `query` came
	 *  from the outside and has to be re-parsed. Starts null (not '') so the
	 *  FIRST run always processes the initial text: an editor mounted on the
	 *  empty match-all rule must count it like any other rule instead of
	 *  silently skipping validation. */
	let ownText: string | null = null;
	let debounce: ReturnType<typeof setTimeout> | undefined;
	let validateSeq = 0;

	const compiled = $derived(rawMode ? query : compileQuery(model));
	const complete = $derived(rawMode ? true : modelComplete(model));

	$effect(() => {
		const incoming = query;
		if (incoming === ownText) return;
		ownText = incoming;
		const parsed = parseQuery(incoming, { hasCustomLabels });
		if (parsed) {
			model = parsed;
			undrawable = false;
			rawMode = false;
		} else {
			undrawable = true;
			rawMode = true;
		}
		openChip = null;
		schedule(incoming, true);
	});

	// The state push is deliberately its own effect: the owner writes it into
	// the pill, which must not feed back into parsing.
	$effect(() => {
		onstate?.({
			text: compiled,
			complete,
			valid: complete ? (validation?.valid ?? null) : false,
			count: validation?.count ?? null,
			error: complete ? (validation?.valid === false ? validation.error : '') : m.query_incomplete(),
			validating
		});
	});

	onDestroy(() => clearTimeout(debounce));

	function getOperators(property: string): OperatorDef[] {
		return allProperties.find((p) => p.value === property)?.numeric
			? numericOperators
			: stringOperators;
	}

	/** Recompile, publish, and re-arm the debounced count. */
	function sync() {
		const text = compileQuery(model);
		ownText = text;
		query = text;
		schedule(text, complete);
	}

	function schedule(text: string, canValidate: boolean) {
		clearTimeout(debounce);
		validateSeq++;
		validating = false;
		// A half-typed rule is not a query: it never reaches the server, and it
		// never counts as valid for a commit.
		if (!canValidate) {
			validation = null;
			return;
		}
		// The EMPTY query is not skipped: it is the legal match-all rule — the
		// server parses '' as the always-true tree and returns the real
		// fleet/org-wide count, so "will match N" appears for it like any rule.
		validating = true;
		debounce = setTimeout(() => runValidate(text), 300);
	}

	async function runValidate(text: string) {
		const seq = ++validateSeq;
		validating = true;
		try {
			if (kind === 'user') {
				const r = await apiClient.validateUserGroupQuery(text);
				if (seq !== validateSeq) return;
				validation = {
					valid: r.valid,
					error: r.error,
					count: r.valid ? r.matchingUserCount : null
				};
			} else {
				const r = await apiClient.validateDynamicQuery(text);
				if (seq !== validateSeq) return;
				validation = {
					valid: r.valid,
					error: r.error,
					count: r.valid ? r.matchingDeviceCount : null
				};
			}
		} catch (error) {
			if (seq !== validateSeq) return;
			console.error('dynamic query validation failed', error);
			validation = {
				valid: false,
				error: kind === 'user' ? m.user_groups_query_failed() : m.device_groups_query_failed(),
				count: null
			};
		} finally {
			if (seq === validateSeq) validating = false;
		}
	}

	// ── chip mutations ───────────────────────────────────────────────────────
	function condAt(pos: { node: number; sub: number }): Cond | null {
		const node = model.nodes[pos.node];
		if (!node) return null;
		if (node.kind === 'cond') return pos.sub < 0 ? node.cond : null;
		return node.conds[pos.sub] ?? null;
	}

	const openCond = $derived(openChip ? condAt(openChip) : null);

	function toggleChip(node: number, sub: number) {
		openChip = openChip && openChip.node === node && openChip.sub === sub ? null : { node, sub };
	}

	function addCondition() {
		model.nodes = [...model.nodes, { kind: 'cond', cond: emptyCond() }];
		model.joins = [...model.joins, 'AND'];
		openChip = { node: model.nodes.length - 1, sub: -1 };
		sync();
	}

	/** Turn the open condition into a parenthesised alternative — the only way to
	 *  author a group from chips alone. */
	function addAlternative() {
		if (!openChip) return;
		const node = model.nodes[openChip.node];
		if (!node) return;
		if (node.kind === 'cond') {
			model.nodes[openChip.node] = { kind: 'group', join: 'OR', conds: [node.cond, emptyCond()] };
			openChip = { node: openChip.node, sub: 1 };
		} else {
			node.conds = [...node.conds, emptyCond()];
			openChip = { node: openChip.node, sub: node.conds.length - 1 };
		}
		sync();
	}

	function removeChip(nodeIndex: number, sub: number) {
		const node = model.nodes[nodeIndex];
		if (!node) return;
		if (node.kind === 'group' && sub >= 0) {
			const rest = node.conds.filter((_, i) => i !== sub);
			model.nodes[nodeIndex] =
				rest.length === 1 ? { kind: 'cond', cond: rest[0] } : { ...node, conds: rest };
			if (rest.length === 0) removeNode(nodeIndex);
		} else {
			removeNode(nodeIndex);
		}
		openChip = null;
		sync();
	}

	function removeNode(index: number) {
		const nodes = model.nodes.filter((_, i) => i !== index);
		// The join that bound this node to its neighbour goes with it.
		const joins = model.joins.filter((_, i) => i !== (index > 0 ? index - 1 : 0));
		model.nodes = nodes.length ? nodes : [{ kind: 'cond', cond: emptyCond() }];
		model.joins = nodes.length ? joins : [];
	}

	function toggleJoin(index: number) {
		model.joins[index] = model.joins[index] === 'AND' ? 'OR' : 'AND';
		sync();
	}

	function toggleGroupJoin(index: number) {
		const node = model.nodes[index];
		if (node?.kind !== 'group') return;
		node.join = node.join === 'AND' ? 'OR' : 'AND';
		sync();
	}

	function patchOpen(patch: Partial<Cond>) {
		if (!openChip) return;
		const c = condAt(openChip);
		if (!c) return;
		Object.assign(c, patch);
		sync();
	}

	function changeProperty(value: string | undefined) {
		if (value === undefined) return;
		const ops = getOperators(value);
		const c = openChip ? condAt(openChip) : null;
		const operator = c && ops.some((o) => o.value === c.operator) ? c.operator : 'equals';
		patchOpen({ property: value, labelKey: '', operator });
	}

	function changeOperator(value: string | undefined) {
		if (value === undefined) return;
		patchOpen(isUnary(value) ? { operator: value, value: '' } : { operator: value });
	}

	function toggleRaw() {
		if (rawMode) {
			const parsed = parseQuery(query, { hasCustomLabels });
			if (!parsed) {
				undrawable = true;
				return;
			}
			model = parsed;
			undrawable = false;
			rawMode = false;
			ownText = query;
			schedule(query, modelComplete(parsed));
			return;
		}
		rawMode = true;
		openChip = null;
	}

	function onRawInput() {
		ownText = query;
		schedule(query, true);
	}

	// ── labels ───────────────────────────────────────────────────────────────
	function chipKey(c: Cond): string {
		return resolveProperty(c) || m.qb_placeholder_property();
	}

	function chipValue(c: Cond): string {
		if (isUnary(c.operator)) return '';
		if (!c.value) return '…';
		const text = compileCond(c);
		const at = text.indexOf(c.operator);
		return at < 0 ? c.value : text.slice(at + c.operator.length).trim();
	}

	function propertyLabel(prop: string): string {
		return allProperties.find((p) => p.value === prop)?.label ?? prop;
	}

	function operatorLabel(op: string, prop: string): string {
		return getOperators(prop).find((o) => o.value === op)?.label ?? op;
	}

	const operatorHints: Record<string, (params: { example: string }) => string> = {
		equals: (p) => m.qb_hint_equals(p),
		notEquals: (p) => m.qb_hint_not_equals(p),
		contains: (p) => m.qb_hint_contains(p),
		notContains: (p) => m.qb_hint_not_contains(p),
		startsWith: (p) => m.qb_hint_starts_with(p),
		endsWith: (p) => m.qb_hint_ends_with(p),
		greaterThan: (p) => m.qb_hint_greater_than(p),
		lessThan: (p) => m.qb_hint_less_than(p),
		in: (p) => m.qb_hint_in(p),
		notIn: (p) => m.qb_hint_not_in(p),
		matches: (p) => m.qb_hint_matches(p),
		notMatches: (p) => m.qb_hint_not_matches(p)
	};

	const unaryOperatorHints: Record<string, () => string> = {
		exists: () => m.qb_hint_exists(),
		notExists: () => m.qb_hint_not_exists()
	};

	function conditionHint(c: Cond): string {
		if (isUnary(c.operator) && unaryOperatorHints[c.operator]) return unaryOperatorHints[c.operator]();
		if (c.operator && operatorHints[c.operator]) {
			const example = c.property && propertyExamples[c.property] ? propertyExamples[c.property]() : '';
			return operatorHints[c.operator]({ example });
		}
		return '';
	}

	function valuePlaceholder(c: Cond): string {
		return c.property && propertyExamples[c.property]
			? propertyExamples[c.property]()
			: m.qb_placeholder_value();
	}

	const CHIP =
		'inline-flex items-center gap-1.5 rounded-[7px] border border-border-strong bg-surface px-2 py-1 font-mono text-xs';
	const JOIN =
		'rounded-[5px] bg-warn-soft px-1.5 py-0.5 font-mono text-[0.66rem] tracking-wide text-warn';
</script>

{#snippet chip(c: Cond, node: number, sub: number)}
	{@const active = openChip?.node === node && openChip?.sub === sub}
	<!-- items-STRETCH, not items-center: the two halves are one chip cut in two,
	     and they only read that way if they are the same height. The body is
	     sized by its text (16px line box) and the remove button by a 12px icon,
	     so centring them left the ✕ inset and the joined borders not meeting. -->
	<span class="inline-flex items-stretch">
		<button
			type="button"
			data-testid="query-chip"
			data-query={compileCond(c)}
			aria-expanded={active}
			class="{CHIP} {active ? 'ring-2 ring-accent-ink/60' : ''} rounded-r-none border-r-0"
			onclick={() => toggleChip(node, sub)}
		>
			<span class="text-accent-ink">{chipKey(c)}</span>
			<span class="text-warn">{c.operator}</span>
			{#if chipValue(c)}<span class="text-ok">{chipValue(c)}</span>{/if}
		</button>
		<button
			type="button"
			data-testid="query-chip-remove"
			aria-label={m.query_remove_condition()}
			class="{CHIP} rounded-l-none px-1.5 text-faint hover:text-crit"
			onclick={() => removeChip(node, sub)}
		>
			<X class="h-3 w-3" />
		</button>
	</span>
{/snippet}

<div
	class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate"
	data-tour="group-rule-editor"
	data-testid="query-editor"
	data-mode={rawMode ? 'raw' : 'chips'}
>
	<div
		class="flex flex-wrap items-center gap-1.5 border-b bg-sunken px-3 py-2.5"
		data-testid="query-chip-bar"
	>
		{#if rawMode}
			<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
				{m.query_raw_label()}
			</span>
		{:else}
			{#each model.nodes as node, i (i)}
				{#if i > 0}
					<button
						type="button"
						data-testid="query-join"
						aria-label={m.query_toggle_join()}
						class={JOIN}
						onclick={() => toggleJoin(i - 1)}
					>
						{model.joins[i - 1] ?? 'AND'}
					</button>
				{/if}
				{#if node.kind === 'cond'}
					{@render chip(node.cond, i, -1)}
				{:else}
					<span
						data-testid="query-group"
						class="inline-flex flex-wrap items-center gap-1.5 rounded-[8px] border border-dashed border-border-strong px-1.5 py-0.5"
					>
						<span class="font-mono text-[0.6rem] text-info">(</span>
						{#each node.conds as c, j (j)}
							{#if j > 0}
								<button
									type="button"
									data-testid="query-group-join"
									aria-label={m.query_toggle_join()}
									class="{JOIN} bg-transparent text-faint"
									onclick={() => toggleGroupJoin(i)}
								>
									{node.join}
								</button>
							{/if}
							{@render chip(c, i, j)}
						{/each}
						<span class="font-mono text-[0.6rem] text-info">)</span>
					</span>
				{/if}
			{/each}
			<button
				type="button"
				data-testid="query-add-condition"
				class="rounded-[7px] border border-dashed border-border-strong px-2 py-1 font-mono text-xs text-faint hover:text-foreground"
				onclick={addCondition}
			>
				<Plus class="mr-1 inline h-3 w-3" />{m.qb_add_condition()}
			</button>
		{/if}
		<button
			type="button"
			data-testid="query-raw-toggle"
			class="ml-auto rounded-[6px] border px-1.5 py-0.5 font-mono text-[0.66rem] text-faint hover:text-foreground"
			onclick={toggleRaw}
		>
			{rawMode ? m.query_mode_chips() : m.query_mode_raw()}
		</button>
	</div>

	{#if banner}{@render banner()}{/if}

	{#if rawMode}
		<div class="space-y-2 p-3">
			{#if undrawable}
				<p class="rounded-md bg-warn-soft px-2.5 py-2 text-xs text-warn">{m.qb_complex_query()}</p>
			{/if}
			<Textarea
				bind:value={query}
				oninput={onRawInput}
				placeholder={advancedPlaceholder}
				rows={3}
				class="font-mono text-sm"
				aria-label={m.query_raw_label()}
			/>
			<p class="text-xs text-muted-foreground">{advancedHint}</p>
		</div>
	{:else if openCond}
		<div class="space-y-2 border-b border-hair bg-frame p-3" data-testid="query-chip-editor">
			<div class="flex flex-wrap items-start gap-2">
				<div class="min-w-40 flex-1">
					<Select.Root
						type="single"
						value={openCond.property}
						onValueChange={(v) => changeProperty(v)}
					>
						<Select.Trigger size="sm" class="w-full" aria-label={m.qb_placeholder_property()}>
							{openCond.property ? propertyLabel(openCond.property) : m.qb_placeholder_property()}
						</Select.Trigger>
						<Select.Content>
							{#each propertyGroups as group (group.label)}
								<Select.Group>
									<Select.GroupHeading>{group.label}</Select.GroupHeading>
									{#each group.items as item (item.value)}
										<Select.Item value={item.value}>{item.label}</Select.Item>
									{/each}
								</Select.Group>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>

				{#if hasCustomLabels && openCond.property === LABEL_CUSTOM}
					<Input
						placeholder={m.qb_placeholder_label_key()}
						value={openCond.labelKey}
						oninput={(e) => patchOpen({ labelKey: e.currentTarget.value })}
						class="h-8 min-w-32 flex-1 font-mono text-sm"
						aria-label={m.qb_placeholder_label_key()}
					/>
				{/if}

				<div class="min-w-32 flex-1">
					<Select.Root
						type="single"
						value={openCond.operator}
						onValueChange={(v) => changeOperator(v)}
					>
						<Select.Trigger size="sm" class="w-full" aria-label={m.query_operator_label()}>
							{operatorLabel(openCond.operator, openCond.property)}
						</Select.Trigger>
						<Select.Content>
							{#each getOperators(openCond.property) as op (op.value)}
								<Select.Item value={op.value}>{op.label}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>

				{#if !isUnary(openCond.operator)}
					<Input
						placeholder={valuePlaceholder(openCond)}
						value={openCond.value}
						oninput={(e) => patchOpen({ value: e.currentTarget.value })}
						class="h-8 min-w-40 flex-1 font-mono text-sm"
						aria-label={m.qb_placeholder_value()}
					/>
				{/if}

				<Button
					variant="ghost"
					size="sm"
					class="h-8 shrink-0 text-xs"
					onclick={addAlternative}
					data-testid="query-add-alternative"
				>
					{m.query_add_alternative()}
				</Button>
				<Button
					variant="ghost"
					size="icon-sm"
					class="shrink-0 text-muted-foreground hover:text-destructive"
					aria-label={m.query_remove_condition()}
					onclick={() => openChip && removeChip(openChip.node, openChip.sub)}
				>
					<Trash2 class="h-4 w-4" />
				</Button>
			</div>
			{#if conditionHint(openCond)}
				<p class="text-xs text-muted-foreground">{conditionHint(openCond)}</p>
			{/if}
		</div>
	{/if}

	{#if inlineStatus}
		<div
			class="flex flex-wrap items-baseline gap-x-2 gap-y-1 px-3 py-2 text-xs"
			data-testid="query-status"
		>
			{#if !complete}
				<span class="text-warn">{m.query_incomplete()}</span>
			{:else if validating}
				<span class="text-muted-foreground">{m.query_counting()}</span>
			{:else if validation && !validation.valid}
				<span class="text-crit">{validation.error}</span>
			{:else if validation?.count !== null && validation?.count !== undefined}
				<span class="font-semibold text-foreground">
					{kind === 'user'
						? m.query_match_count_users({ count: validation.count })
						: m.query_match_count_devices({ count: validation.count })}
				</span>
			{/if}
			{#if compiled}
				<span class="font-mono break-words text-faint">{compiled}</span>
			{/if}
		</div>
	{/if}

	{#if preview}{@render preview()}{/if}
</div>
