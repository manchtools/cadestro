<script lang="ts" module>
	export interface QueryEditorState {
		text: string;
		valid: boolean;
		count: number | null;
		error: string;
		validating: boolean;
	}
</script>

<script lang="ts">
	import { onDestroy, type Snippet } from 'svelte';
	import { apiClient } from '$lib/sdk';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as m from '$lib/paraglide/messages';

	type Props = {
		query: string;
		kind?: 'device' | 'user';
		onstate?: (state: QueryEditorState) => void;
		inlineStatus?: boolean;
		banner?: Snippet;
		preview?: Snippet;
	};

	let {
		query = $bindable(''),
		kind = 'device',
		onstate,
		inlineStatus = true,
		banner,
		preview
	}: Props = $props();

	let validation = $state<{ valid: boolean; error: string; count: number | null } | null>(null);
	let validating = $state(false);
	let sequence = 0;
	let debounce: ReturnType<typeof setTimeout> | undefined;

	const editorState = $derived.by((): QueryEditorState => {
		const text = query;
		const empty = text.trim().length === 0;
		return {
			text,
			valid: !empty && !validating && validation?.valid === true,
			count: !empty && !validating && validation?.valid === true ? validation.count : null,
			error: empty ? m.query_incomplete() : validation?.error ?? '',
			validating
		};
	});

	function schedule(text: string) {
		if (debounce !== undefined) clearTimeout(debounce);
		sequence += 1;
		validation = null;
		validating = false;
		if (!text.trim()) return;
		validating = true;
		const current = sequence;
		debounce = setTimeout(() => void validate(text, current), 300);
	}

	async function validate(text: string, current: number) {
		try {
			if (kind === 'user') {
				const result = await apiClient.validateUserGroupQuery(text);
				if (current !== sequence) return;
				validation = {
					valid: result.valid,
					error: result.error,
					count: result.valid ? result.matchingUserCount : null
				};
			} else {
				const result = await apiClient.validateDynamicQuery(text);
				if (current !== sequence) return;
				validation = {
					valid: result.valid,
					error: result.error,
					count: result.valid ? result.matchingDeviceCount : null
				};
			}
		} catch (error) {
			if (current !== sequence) return;
			console.error(error);
			validation = {
				valid: false,
				error: kind === 'user' ? m.user_groups_query_failed() : m.device_groups_query_failed(),
				count: null
			};
		} finally {
			if (current === sequence) validating = false;
		}
	}

	$effect(() => {
		schedule(query);
	});

	$effect(() => {
		onstate?.(editorState);
	});

	onDestroy(() => {
		if (debounce !== undefined) clearTimeout(debounce);
		sequence += 1;
	});
</script>

<div class="space-y-2" data-testid="query-editor">
	{#if banner}{@render banner()}{/if}
	<label for="query-editor-text" class="text-xs font-medium">{m.query_raw_label()}</label>
	<Textarea
		id="query-editor-text"
		bind:value={query}
		rows={4}
		class="font-mono text-sm"
		placeholder={kind === 'user'
			? m.user_groups_dynamic_query_placeholder()
			: m.device_groups_dynamic_query_placeholder()}
		aria-describedby={inlineStatus ? 'query-editor-hint query-editor-status' : 'query-editor-hint'}
		data-testid="query-input"
	/>
	<p id="query-editor-hint" class="text-xs text-faint">{m.query_cel_hint()}</p>
	{#if inlineStatus}
		<div id="query-editor-status" class="text-xs" data-testid="query-status" aria-live="polite">
			{#if editorState.validating}
				<span class="text-muted-foreground">{m.query_counting()}</span>
			{:else if !editorState.valid}
				<span class="text-crit">{editorState.error}</span>
			{:else if editorState.count !== null}
				<span class="text-muted-foreground">
					{kind === 'user'
						? m.query_match_count_users({ count: editorState.count })
						: m.query_match_count_devices({ count: editorState.count })}
				</span>
			{/if}
		</div>
	{/if}
	{#if preview}{@render preview()}{/if}
</div>
