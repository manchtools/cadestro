<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';
	import { apiClient, fetchAllPages, type ManagedAction } from '$lib/sdk';
	import { ActionType } from '$sdk/powermanage/v1/actions_pb';
	import { ActionCreateForm, getActionTypeLabel } from '$lib/components/actions';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import { Plus, Search, ArrowLeft, Clock } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;
		deviceId: string;
		ondispatched?: () => void;
	}

	let { open = $bindable(), deviceId, ondispatched }: Props = $props();

	let scriptActions = $state<ManagedAction[]>([]);
	let loading = $state(false);
	let selectedActionId = $state<string | null>(null);
	let dispatching = $state(false);
	let showCreateForm = $state(false);
	let searchQuery = $state('');
	// Schedule-for-later state (#57). When scheduleEnabled is true the
	// dispatch is deferred to scheduleAt (datetime-local). The value is
	// kept as a string so the <input type="datetime-local"> binding
	// works directly; we convert to Date on submit.
	let scheduleEnabled = $state(false);
	let scheduleAt = $state(defaultScheduleAt());

	function defaultScheduleAt(): string {
		// Default to "now + 1 hour" so the picker isn't sitting on an
		// already-past timestamp when the operator opens the dialog.
		const t = new Date(Date.now() + 60 * 60 * 1000);
		// trim seconds for the datetime-local control (yyyy-MM-ddTHH:mm)
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())}T${pad(t.getHours())}:${pad(t.getMinutes())}`;
	}

	const filtered = $derived(
		searchQuery
			? scriptActions.filter(
					(a) =>
						a.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
						a.description.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: scriptActions
	);

	$effect(() => {
		if (open) {
			selectedActionId = null;
			showCreateForm = false;
			searchQuery = '';
			scheduleEnabled = false;
			scheduleAt = defaultScheduleAt();
			loadScriptActions();
		}
	});

	function dispatchOptions() {
		if (!scheduleEnabled) return undefined;
		const when = new Date(scheduleAt);
		if (isNaN(when.getTime()) || when.getTime() <= Date.now()) {
			return null; // signal "invalid"
		}
		return { runAt: when };
	}

	async function loadScriptActions() {
		loading = true;
		try {
			// F023: page through all SHELL actions instead of capping at 100.
			scriptActions = await fetchAllPages<ManagedAction>(async (size, token) => {
				const r = await apiClient.listActions(size, token, ActionType.SHELL);
				return { items: r.actions, nextPageToken: r.nextPageToken };
			});
		} catch (error) {
			console.warn('Failed to load script actions', error);
			scriptActions = [];
		} finally {
			loading = false;
		}
	}

	async function dispatchSelected() {
		if (!selectedActionId) return;
		const opts = dispatchOptions();
		if (opts === null) {
			toast.error(m.instant_actions_run_script_schedule_invalid());
			return;
		}
		dispatching = true;
		try {
			await apiClient.dispatchAction(deviceId, selectedActionId, opts);
			toast.success(opts?.runAt ? m.instant_actions_run_script_scheduled() : m.instant_actions_run_script_dispatched());
			open = false;
			ondispatched?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			dispatching = false;
		}
	}

	async function handleCreated(action: ManagedAction) {
		const opts = dispatchOptions();
		if (opts === null) {
			toast.error(m.instant_actions_run_script_schedule_invalid());
			return;
		}
		dispatching = true;
		try {
			await apiClient.dispatchAction(deviceId, action.id, opts);
			toast.success(opts?.runAt ? m.instant_actions_run_script_scheduled() : m.instant_actions_run_script_dispatched());
			open = false;
			ondispatched?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			dispatching = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
		{#if showCreateForm}
			<div class="flex-1 overflow-y-auto p-1 m-2">
				<ActionCreateForm
					compact
					initialType="SHELL"
					onCancel={() => (showCreateForm = false)}
					onCreated={handleCreated}
				/>
			</div>
		{:else}
			<Dialog.Header>
				<Dialog.Title>{m.instant_actions_run_script_title()}</Dialog.Title>
				<Dialog.Description>
					{m.instant_actions_run_script_description()}
				</Dialog.Description>
			</Dialog.Header>

			<div class="flex justify-end mb-2">
				<Button variant="outline" size="sm" onclick={() => (showCreateForm = true)}>
					<Plus class="h-4 w-4 mr-2" />
					{m.instant_actions_run_script_create_new()}
				</Button>
			</div>

			<!-- The dialog's ONE scroll region. The header, the schedule box and the
			     footer keep their natural height; everything that can grow lives in
			     here, so there is never a scroll area nested inside a scroll area
			     for the wheel to get trapped in. -->
			<div data-testid="run-script-list" class="min-h-0 flex-1 overflow-y-auto">
				{#if loading}
					<div class="py-8 text-center">
						<p class="text-muted-foreground">{m.common_loading()}</p>
					</div>
				{:else if scriptActions.length === 0}
					<div class="py-8 text-center">
						<p class="text-muted-foreground">{m.instant_actions_run_script_no_scripts()}</p>
						<Button variant="outline" class="mt-4" onclick={() => (showCreateForm = true)}>
							<Plus class="h-4 w-4 mr-2" />
							{m.instant_actions_run_script_create_new()}
						</Button>
					</div>
				{:else}
					<div class="space-y-3">
						<div class="relative">
							<Search
								class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
							/>
							<Input placeholder={m.common_search()} bind:value={searchQuery} class="pl-9" />
						</div>

						{#if filtered.length === 0}
							<p class="py-6 text-center text-sm text-muted-foreground">
								{m.common_no_results_search()}
							</p>
						{:else}
							<div class="rounded-md border">
								<Table.Root>
									<Table.Header>
										<Table.Row>
											<Table.Head>{m.common_name()}</Table.Head>
											<Table.Head>{m.common_type()}</Table.Head>
										</Table.Row>
									</Table.Header>
									<Table.Body>
										{#each filtered as action (action.id)}
											<Table.Row
												data-state={selectedActionId === action.id ? 'selected' : undefined}
												class="cursor-pointer"
												onclick={() => (selectedActionId = action.id)}
											>
												<Table.Cell>
													<div>
														<span class="font-medium">{action.name}</span>
														{#if action.description}
															<p class="text-xs text-muted-foreground line-clamp-1">
																{action.description}
															</p>
														{/if}
													</div>
												</Table.Cell>
												<Table.Cell>
													{@const isCompliance = action.params.case === 'shell' && action.params.value.isCompliance}
													<Badge variant="outline">{isCompliance ? m.actions_type_option_compliance_check() : getActionTypeLabel(action.type)}</Badge>
												</Table.Cell>
											</Table.Row>
										{/each}
									</Table.Body>
								</Table.Root>
							</div>
						{/if}
					</div>
				{/if}
			</div>

			<div class="mt-4 rounded-md border p-3">
				<div class="flex items-center justify-between gap-3">
					<div class="flex items-center gap-2">
						<Clock class="h-4 w-4 text-muted-foreground" />
						<Label for="schedule-toggle" class="cursor-pointer">{m.instant_actions_run_script_schedule_label()}</Label>
					</div>
					<Switch id="schedule-toggle" bind:checked={scheduleEnabled} />
				</div>
				{#if scheduleEnabled}
					<div class="mt-3 space-y-1">
						<Label for="schedule-at" class="text-xs text-muted-foreground">{m.instant_actions_run_script_schedule_at()}</Label>
						<Input id="schedule-at" type="datetime-local" bind:value={scheduleAt} />
						<p class="text-xs text-muted-foreground">{m.instant_actions_run_script_schedule_help()}</p>
					</div>
				{/if}
			</div>

			<Dialog.Footer class="mt-4">
				<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
				<Button onclick={dispatchSelected} disabled={!selectedActionId || dispatching}>
					{dispatching
						? m.instant_actions_dispatching()
						: scheduleEnabled
							? m.instant_actions_run_script_schedule_run()
							: m.instant_actions_run_script_run_selected()}
				</Button>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
