<script lang="ts">

	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;

		queryText: string;

		count: number | null;
		kind: 'device' | 'user';

		converting?: boolean;

		currentMembers?: number;

		note?: string;
		onconfirm: () => void;
		oncancel?: () => void;
	}

	let {
		open = $bindable(),
		queryText,
		count,
		kind,
		converting = false,
		currentMembers = 0,
		note,
		onconfirm,
		oncancel
	}: Props = $props();
</script>

<AlertDialog.Root
	bind:open
	onOpenChange={(next) => {
		if (!next) oncancel?.();
	}}
>
	<AlertDialog.Content data-testid="future-scope-dialog">
		<AlertDialog.Header>
			<AlertDialog.Title>
				{converting ? m.query_future_scope_convert_title() : m.query_future_scope_title()}
			</AlertDialog.Title>
			<AlertDialog.Description>
				{#if count === null}
					{m.query_future_scope_body_unknown()}
				{:else if kind === 'user'}
					{m.query_future_scope_body_users({ count })}
				{:else}
					{m.query_future_scope_body_devices({ count })}
				{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>

		<p
			class="rounded-md bg-sunken px-3 py-2 font-mono text-xs break-words text-muted-foreground"
			data-testid="future-scope-query"
		>
			{queryText}
		</p>
		{#if note}
			<p class="text-sm" data-testid="future-scope-note">{note}</p>
		{/if}
		{#if converting && currentMembers > 0}
			<p class="text-sm text-warn" data-testid="future-scope-convert-members">
				{m.query_future_scope_convert_members({ count: currentMembers })}
			</p>
		{/if}
		<p class="text-sm text-warn" data-testid="future-scope-standing">
			{m.query_future_scope_standing()}
		</p>

		<AlertDialog.Footer>
			<AlertDialog.Cancel data-testid="future-scope-cancel">{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onconfirm} data-testid="future-scope-confirm">
				{m.query_future_scope_confirm()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
