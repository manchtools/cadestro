<script lang="ts">
 import { onMount } from 'svelte';
 import { toast } from 'svelte-sonner';
 import { api } from '$lib/api';
 import type { ExecutionResult } from '$contract/cadestro/v1/control_pb';
 import { ExecutionStatus, ComplianceStatus } from '$contract/cadestro/v1/common_pb';
 import { getLocalizedError } from '$lib/errors';
 import { formatDate } from '$lib/console';
 import { Button } from '$lib/components/ui/button';
 import { Chip } from '$lib/components/fleet';
 import * as Sheet from '$lib/components/ui/sheet';
 let { deviceId }: { deviceId: string } = $props();
 let rows = $state<ExecutionResult[]>([]);
 let next = $state('');
 let loading = $state(true);
 let selected = $state<ExecutionResult | null>(null);
 async function load(pageToken = '') { loading = true; try { const response = await api.listExecutionResults({ deviceId: { value: deviceId }, pageSize: 50, pageToken }); rows = pageToken ? [...rows, ...response.results] : response.results; next = response.nextPageToken; } catch(error) { toast.error(getLocalizedError(error)); } finally { loading = false; } }
 onMount(() => load());
</script>
<section class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate">
 <div class="border-b border-hair px-4 py-3 font-mono text-xs uppercase tracking-wide text-faint">Execution history</div>
 {#each rows as result (result.runId?.value)}<button type="button" onclick={() => { selected = result; }} class="flex w-full flex-wrap items-center gap-3 border-b border-hair px-4 py-3 text-left last:border-b-0 hover:bg-sunken"><span class="text-sm font-medium">{result.actionName}</span><Chip tone="idle" label={ExecutionStatus[result.status]} /><span class="ml-auto text-xs text-muted-foreground">{formatDate(result.completedAt)}</span></button>{:else}<p class="px-4 py-8 text-center text-sm text-muted-foreground">{loading ? 'Loading history…' : 'No execution results.'}</p>{/each}
 {#if next}<div class="border-t p-3"><Button size="sm" variant="outline" onclick={() => load(next)} disabled={loading}>Load more</Button></div>{/if}
</section>
<Sheet.Root open={!!selected} onOpenChange={open => { if (!open) selected = null; }}><Sheet.Content><Sheet.Header><Sheet.Title>{selected?.actionName}</Sheet.Title><Sheet.Description>{selected ? `${ExecutionStatus[selected.status]} · ${ComplianceStatus[selected.complianceStatus]}` : ''}</Sheet.Description></Sheet.Header>
 {#if selected}{#each [{ label: 'Command output', output: selected.output }, { label: 'Detection output', output: selected.detectionOutput }] as entry}{#if entry.output}<section class="space-y-2 p-4"><h3 class="text-xs font-semibold uppercase text-muted-foreground">{entry.label}</h3><div class="rounded-md bg-sunken p-3 font-mono text-xs"><pre class="whitespace-pre-wrap">{entry.output.stdout}</pre><pre class="whitespace-pre-wrap text-destructive">{entry.output.stderr}</pre><p>Exit code: {entry.output.exitCode}</p></div></section>{/if}{/each}{/if}
</Sheet.Content></Sheet.Root>
