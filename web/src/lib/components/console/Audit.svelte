<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import {
		AuditActorType,
		AuditEventType,
		AuditStreamType,
		ListAuditEventsRequestSchema,
		Permission,
		type AuditEvent
	} from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage } from '$lib/api';
	import { cursorHref, formatDate } from '$lib/console';

	let events = $state<AuditEvent[]>([]);
	let nextPageToken = $state('');
	let loading = $state(true);
	let error = $state('');
	const pageToken = $derived(page.url.searchParams.get('auditCursor') ?? '');

	onMount(async () => {
		try {
			const response = await api.listAuditEvents(create(ListAuditEventsRequestSchema, { pageSize: 50, pageToken }));
			events = response.events;
			nextPageToken = response.nextPageToken;
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	});
</script>

<section class="card" aria-busy={loading}>
	<div class="section-title"><div><p class="eyebrow">Accountability</p><h1>Audit events</h1></div></div>
	{#if error}<p class="error banner" role="alert">{error}</p>{/if}
	{#if loading}
		<p role="status">Loading audit events…</p>
	{:else if events.length === 0}
		<p>No audit events.</p>
	{:else}
		<div class="table-wrap"><table><thead><tr><th>Time</th><th>Event</th><th>Resource</th><th>Actor</th></tr></thead><tbody>
			{#each events as event (event.id?.value)}
				<tr><td>{formatDate(event.occurredAt)}</td><td>{AuditEventType[event.eventType]}</td><td>{AuditStreamType[event.streamType]} <code>{event.streamId?.value}</code></td><td>{AuditActorType[event.actorType]} <code>{event.actorId?.value}</code></td></tr>
			{/each}
		</tbody></table></div>
	{/if}
	<nav class="pagination" aria-label="Audit event pages">
		{#if pageToken}<a class="button quiet" href={cursorHref(page.url, 'auditCursor', '')}>First page</a>{/if}
		{#if nextPageToken}<a class="button" href={cursorHref(page.url, 'auditCursor', nextPageToken)}>Next page</a>{/if}
	</nav>
</section>
