<script lang="ts">
	// The standing-rule acknowledgement, as a REAL confirm rather than a banner.
	//
	// A dynamic rule keeps matching after it is saved: devices and users that
	// enrol tomorrow inherit it without anyone approving them. The warn strip in
	// the editor says so while you type; this dialog makes the operator say it
	// back before the save RPC is allowed to run.
	//
	// It states no forecast. The concept sketched "17 could match in the next
	// 30 days"; no RPC exposes an enrolment trend, so the prediction is omitted
	// rather than invented — the acknowledgement stands on the rule itself and
	// on the server's own match count.
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;
		/** The exact query the RPC will receive. */
		queryText: string;
		/** Server match count for that query, or null when unknown. */
		count: number | null;
		kind: 'device' | 'user';
		/** static → dynamic is a bigger step than editing an existing rule. */
		converting?: boolean;
		/** Members the group holds right now. Converting to a rule drops them —
		 *  the server clears them in the same transaction that sets the mode — so
		 *  the confirm has to say so rather than only naming the mode change. */
		currentMembers?: number;
		/** Already-resolved extra consequence line, for callers whose confirm also
		 *  creates something (the assign surface names the group it will create).
		 *  The component is not a message registry — the owner passes the text. */
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
			{queryText || m.query_future_scope_empty_query()}
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
