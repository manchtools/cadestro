<script lang="ts">
 import { onMount } from 'svelte';
 import { page } from '$app/state';
 import { goto } from '$lib/navigation';
 import { api } from '$lib/api';
 import { collectPages } from '$lib/console';
 import { Permission, type ManagedAction } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { toast } from 'svelte-sonner';
 import { getLocalizedError } from '$lib/errors';
 import { Button } from '$lib/components/ui/button';
 import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
 import PageShell from '$lib/components/page-shell.svelte';
 import ActionParamsEditor from '$lib/components/actions/action-params-editor.svelte';
 import { ArrowLeft, RefreshCw } from '@lucide/svelte';
 import type { PillAction } from '$lib/shell/shell.svelte';
 import * as m from '$lib/paraglide/messages';
 const { can } = consoleContext();
 const actionId = $derived(page.params.id ?? '');
 let editor = $state<ActionParamsEditor>();
 let action = $state<ManagedAction | null>(null);
 let loading = $state(true);
 let revision = $state(0);
 let deleteDialogOpen = $state(false);
 const entityActions = $derived<PillAction[]>([
  ...(can(Permission.CREATE_ASSIGNMENT) ? [{ id: 'assign', label: m.common_assign(), onRun: () => { void goto(`/assignments?action=${actionId}`); } }] : []),
  ...(can(Permission.DELETE_ACTION) ? [{ id: 'delete', label: m.common_delete(), tone: 'danger' as const, onRun: () => { deleteDialogOpen = true; } }] : [])
 ]);
 async function loadActionData() { if (editor?.hasUnsavedChanges()) return; loading = true; try { if (can(Permission.GET_ACTION)) action = (await api.getAction({ id: { value: actionId } })).action ?? null; else if (can(Permission.LIST_ACTIONS)) action = (await collectPages(async pageToken => { const r = await api.listActions({ pageSize: 100, pageToken }); return { items: r.actions, nextPageToken: r.nextPageToken }; })).find(item => item.id?.value === actionId) ?? null; revision++; } catch(error) { toast.error(getLocalizedError(error)); } finally { loading = false; } }
 async function deleteAction() { if (!can(Permission.DELETE_ACTION)) return; try { await api.deleteAction({ id: { value: actionId } }); await goto('/actions'); } catch(error) { toast.error(getLocalizedError(error)); } }
 onMount(loadActionData);
</script>
<PageShell contentClass="space-y-6">
	{#snippet header()}
		<div class="flex items-center gap-4">
			<Button variant="ghost" size="icon" onclick={() => history.back()}>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-2xl font-bold">{action?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{actionId}</p>
			</div>
			<Button variant="outline" onclick={loadActionData} disabled={loading || editor?.hasUnsavedChanges()}>
				<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
					<RefreshCw class="h-4 w-4" />
				</span>
				{m.common_refresh()}
			</Button>
		</div>
	{/snippet}

	{#if loading && !action}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if action}
		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">

			<div>
 {#key revision}<ActionParamsEditor bind:this={editor} {action} {entityActions} onsaved={(updated) => (action = updated)} />{/key}
			</div>
		</section>

        {#if can(Permission.LIST_ASSIGNMENTS) || can(Permission.CREATE_ASSIGNMENT)}<Button variant="outline" onclick={() => goto(`/assignments?action=${actionId}`)}>{m.action_detail_assignments()}</Button>{/if}
	{/if}
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.actions_delete_dialog_title()}
	description={m.actions_delete_dialog_description({ name: action?.name ?? '' })}
	onconfirm={deleteAction}
/>
