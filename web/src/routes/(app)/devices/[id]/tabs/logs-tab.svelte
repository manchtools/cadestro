<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient } from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Select from '$lib/components/ui/select';
	import { RefreshCw, ScrollText, Search } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		deviceId: string;
	}

	let { deviceId }: Props = $props();

	// Query parameters (pre-filled defaults for one-click agent log query)
	let unit = $state('power-manage-agent.service');
	let lines = $state(100);
	let since = $state('');
	let until = $state('');
	let priority = $state('');
	let grep = $state('');
	let kernel = $state(false);

	// Result state
	let queryLoading = $state(false);
	let queryLogs = $state('');
	let queryError = $state('');
	let queryCompleted = $state(false);

	const priorities = [
		{ value: '', label: '-' },
		{ value: 'emerg', label: 'emerg (0)' },
		{ value: 'alert', label: 'alert (1)' },
		{ value: 'crit', label: 'crit (2)' },
		{ value: 'err', label: 'err (3)' },
		{ value: 'warning', label: 'warning (4)' },
		{ value: 'notice', label: 'notice (5)' },
		{ value: 'info', label: 'info (6)' },
		{ value: 'debug', label: 'debug (7)' }
	];

	async function executeQuery() {
		queryLoading = true;
		queryLogs = '';
		queryError = '';
		queryCompleted = false;

		try {
			const queryId = await apiClient.queryDeviceLogs(deviceId, {
				lines: lines > 0 ? lines : undefined,
				unit: unit || undefined,
				since: since || undefined,
				until: until || undefined,
				priority: priority || undefined,
				grep: grep || undefined,
				kernel: kernel || undefined
			});

			// Poll for result
			const maxPolls = 120;
			for (let i = 0; i < maxPolls; i++) {
				await new Promise((r) => setTimeout(r, 500));
				try {
					const result = await apiClient.getDeviceLogResult(queryId);
					if (result.completed) {
						queryCompleted = true;
						if (result.success) {
							queryLogs = result.logs;
						} else {
							queryError = result.error || 'Unknown error';
						}
						break;
					}
				} catch (err) {
					console.error('Poll error:', err);
				}
			}

			if (!queryCompleted) {
				queryError = 'Query timed out';
			}
		} catch (error) {
			queryError = getLocalizedError(error);
			toast.error(queryError);
		} finally {
			queryLoading = false;
		}
	}
</script>

<div class="space-y-6">
	<!-- Query Form -->
	<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
		<div class="flex items-center gap-2">
			<ScrollText class="h-4 w-4 text-faint" />
			<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
				{m.logs_title()}
			</span>
		</div>
		<div class="mt-3 space-y-4">
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div class="space-y-2">
					<Label>{m.logs_unit()}</Label>
					<Input
						type="text"
						placeholder={m.logs_unit_placeholder()}
						bind:value={unit}
					/>
				</div>
				<div class="space-y-2">
					<Label>{m.logs_lines()}</Label>
					<Input
						type="number"
						bind:value={lines}
						min={1}
						max={10000}
					/>
				</div>
			</div>

			<div class="grid grid-cols-1 md:grid-cols-3 gap-4">
				<div class="space-y-2">
					<Label>{m.logs_since()}</Label>
					<Input
						type="text"
						placeholder={m.logs_since_placeholder()}
						bind:value={since}
					/>
				</div>
				<div class="space-y-2">
					<Label>{m.logs_until()}</Label>
					<Input
						type="text"
						placeholder={m.logs_until_placeholder()}
						bind:value={until}
					/>
				</div>
				<div class="space-y-2">
					<Label>{m.logs_priority()}</Label>
					<Select.Root type="single" value={priority} onValueChange={(v) => priority = v ?? ''}>
						<Select.Trigger>
							{priorities.find(p => p.value === priority)?.label ?? m.logs_priority_placeholder()}
						</Select.Trigger>
						<Select.Content>
							{#each priorities as p}
								<Select.Item value={p.value}>{p.label}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
			</div>

			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div class="space-y-2">
					<Label>{m.logs_grep()}</Label>
					<Input
						type="text"
						placeholder={m.logs_grep_placeholder()}
						bind:value={grep}
					/>
				</div>
				<div class="flex items-end pb-2">
					<label class="flex items-center gap-2 cursor-pointer">
						<Checkbox bind:checked={kernel} />
						<span class="text-sm">{m.logs_kernel()}</span>
					</label>
				</div>
			</div>

			<Button onclick={executeQuery} disabled={queryLoading}>
				<Search class="h-4 w-4 mr-2" />
				{m.logs_execute()}
			</Button>
		</div>
	</section>

	<!-- Results -->
	{#if queryLoading}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="mr-2 h-6 w-6 animate-spin text-muted-foreground" />
			<span class="text-muted-foreground">{m.logs_loading()}</span>
		</div>
	{:else if queryError}
		<div class="rounded-xl border border-hair bg-surface px-4 py-8 shadow-plate">
			<p class="text-center text-crit">{m.logs_error({ error: queryError })}</p>
		</div>
	{:else if queryCompleted && !queryLogs}
		<div class="rounded-xl border border-hair bg-surface px-4 py-8 shadow-plate">
			<p class="text-center text-muted-foreground">{m.logs_no_results()}</p>
		</div>
	{:else if queryLogs}
		<div class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate">
			<pre class="max-h-[600px] overflow-auto bg-sunken p-4 font-mono text-xs break-all whitespace-pre-wrap">{queryLogs}</pre>
		</div>
	{/if}
</div>
