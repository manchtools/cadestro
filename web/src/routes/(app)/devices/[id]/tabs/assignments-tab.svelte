<script lang="ts">
 import { onMount } from 'svelte';
 import { toast } from 'svelte-sonner';
 import { api } from '$lib/api';
 import { Permission, type ManagedAction } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { getLocalizedError } from '$lib/errors';
 import { Button } from '$lib/components/ui/button';
 import { Chip } from '$lib/components/fleet';
 import { actionChoice, getActionTypeInfoByValue } from '$lib/components/actions/action-type';
 const { can } = consoleContext();
 let { deviceId }: { deviceId: string } = $props();
 let actions = $state<ManagedAction[]>([]);
 let loading = $state(true);
 onMount(async () => { try { actions = (await api.getDeviceAssignments({ deviceId: { value: deviceId } })).actions; } catch(error) { toast.error(getLocalizedError(error)); } finally { loading = false; } });
</script>
<section class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate">
 <div class="flex items-center justify-between border-b border-hair px-4 py-3"><span class="font-mono text-xs uppercase tracking-wide text-faint">Assigned actions</span>{#if can(Permission.CREATE_ASSIGNMENT)}<Button size="sm" variant="outline" href={`/assignments?devices=${deviceId}`}>Assign</Button>{/if}</div>
 {#each actions as action (action.id?.value)}{@const info = getActionTypeInfoByValue(actionChoice(action))}<div class="flex items-center gap-3 border-b border-hair px-4 py-3 last:border-b-0"><info.icon class="h-4 w-4 text-accent-ink" /><a href={can(Permission.GET_ACTION) ? `/actions/${action.id?.value ?? ''}` : undefined} class="min-w-0 flex-1 truncate text-sm font-medium">{action.name}</a><Chip tone="info" label={info.label} /></div>{:else}<p class="px-4 py-8 text-center text-sm text-muted-foreground">{loading ? 'Loading assigned actions…' : 'No actions assigned.'}</p>{/each}
</section>
