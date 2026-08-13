<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';
	import {
		apiClient,
		type ManagedAction,
		type ActionSet,
		type Definition,
		type Assignment
	} from '$lib/sdk';
	import { ActionType } from '$sdk/powermanage/v1/actions_pb';
	import { AssignmentMode, AssignmentSourceType, AssignmentTargetType } from '$sdk/powermanage/v1/common_pb';
	import { getActionTypeInfo } from '$lib/components/actions';
	import ActionDetailSheet, { openActionSheet } from '$lib/components/actions/action-detail-sheet.svelte';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Chip } from '$lib/components/fleet';
	import type { FleetTone } from '$lib/components/fleet/tone';
	import { Button } from '$lib/components/ui/button';
	import { ShieldCheck, Layers, BookOpen, Zap, Search, Play } from '@lucide/svelte';
	import { Input } from '$lib/components/ui/input';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		deviceId: string;
	}

	let { deviceId }: Props = $props();

	let actions = $state<ManagedAction[]>([]);
	let actionSets = $state<ActionSet[]>([]);
	let definitions = $state<Definition[]>([]);
	let directAssignments = $state<Assignment[]>([]);
	let loading = $state(true);

	// Reverse map: actionId → list of parent containers
	type ParentInfo = {
		type: 'action_set';
		id: string;
		name: string;
		definitionId?: string;
		definitionName?: string;
	};
	let actionParentsMap = $state(new Map<string, ParentInfo[]>());

	// Search
	let searchQuery = $state('');

	// Derive currently-shown action ID from ActionDetailSheet shallow routing
	let sheetActionId = $derived(page.state.actionSheet);

	// Map of "sourceType:sourceId" → Assignment for quick lookup.
	// Keyed by the AssignmentSourceType enum value (numeric) joined
	// with the source id; the lookup helper feeds the same enum.
	const directAssignmentMap = $derived.by(() => {
		const map = new Map<string, Assignment>();
		for (const a of directAssignments) {
			map.set(`${a.sourceType}:${a.sourceId}`, a);
		}
		return map;
	});

	function isComplianceCheck(action: ManagedAction): boolean {
		return action.type === ActionType.SHELL && action.params.case === 'shell' && action.params.value.isCompliance;
	}

	// Deduplicated actions by ID, excluding compliance check actions
	const uniqueActions = $derived.by(() => {
		const seen = new Set<string>();
		const result: ManagedAction[] = [];
		for (const action of actions) {
			if (!seen.has(action.id) && !isComplianceCheck(action)) {
				seen.add(action.id);
				result.push(action);
			}
		}
		return result;
	});

	// Filtered actions by search query
	const filteredActions = $derived.by(() => {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return uniqueActions;
		return uniqueActions.filter((action) => {
			const typeInfo = getActionTypeInfo(action.type);
			return (
				action.name.toLowerCase().includes(q) ||
				typeInfo.label.toLowerCase().includes(q)
			);
		});
	});

	const isEmpty = $derived(
		actions.length === 0 && actionSets.length === 0 && definitions.length === 0
	);

	onMount(() => {
		loadData();
	});

	async function loadData() {
		loading = true;
		try {
			const [assignmentsResponse, directResponse] = await Promise.all([
				apiClient.getDeviceAssignments(deviceId),
				apiClient.listAssignments(100, '', AssignmentSourceType.UNSPECIFIED, '', AssignmentTargetType.DEVICE, deviceId)
			]);
			actions = assignmentsResponse.actions;
			actionSets = assignmentsResponse.actionSets;
			definitions = assignmentsResponse.definitions;
			directAssignments = directResponse.assignments;

			// Build parent mapping from enriched response (no extra API calls needed)
			buildParentMapping(
				assignmentsResponse.actionSetDetails ?? [],
				assignmentsResponse.definitionDetails ?? []
			);
		} catch (error) {
			console.error('Failed to load policies:', error);
		} finally {
			loading = false;
		}
	}

	function buildParentMapping(
		setDetails: Array<{ set?: ActionSet; members: Array<{ actionId: string }> }>,
		defDetails: Array<{ definition?: Definition; members: Array<{ actionSetId: string }> }>
	) {
		const map = new Map<string, ParentInfo[]>();

		// Build a map of setId → definition info
		const setToDefMap = new Map<string, { defId: string; defName: string }>();
		for (const defDetail of defDetails) {
			if (!defDetail.definition) continue;
			for (const member of defDetail.members) {
				setToDefMap.set(member.actionSetId, {
					defId: defDetail.definition.id,
					defName: defDetail.definition.name
				});
			}
		}

		// Build action → parent mapping from action set details
		for (const setDetail of setDetails) {
			if (!setDetail.set) continue;
			const defInfo = setToDefMap.get(setDetail.set.id);
			for (const member of setDetail.members) {
				const existing = map.get(member.actionId) || [];
				existing.push({
					type: 'action_set',
					id: setDetail.set.id,
					name: setDetail.set.name,
					definitionId: defInfo?.defId,
					definitionName: defInfo?.defName
				});
				map.set(member.actionId, existing);
			}
		}

		actionParentsMap = map;
	}

	function getDirectAssignment(
		sourceType: AssignmentSourceType,
		sourceId: string
	): Assignment | undefined {
		return directAssignmentMap.get(`${sourceType}:${sourceId}`);
	}

	// The assignment mode is the state this device is being held in, so it rides
	// a toned chip: removal/exclusion reads critical, "available" is a neutral
	// offer, and the default (required) is the enforced-and-healthy case.
	function getModeTone(mode: AssignmentMode): FleetTone {
		switch (mode) {
			case AssignmentMode.AVAILABLE:
				return 'idle';
			case AssignmentMode.UNINSTALL:
			case AssignmentMode.EXCLUDED:
				return 'crit';
			default:
				return 'info';
		}
	}

	function getModeLabel(mode: AssignmentMode): string {
		switch (mode) {
			case AssignmentMode.AVAILABLE:
				return m.assignments_mode_available_short();
			case AssignmentMode.EXCLUDED:
				return m.assignments_mode_excluded_short();
			case AssignmentMode.UNINSTALL:
				return m.assignments_mode_uninstall_short();
			default:
				return m.assignments_mode_required_short();
		}
	}

	function getParents(actionId: string): ParentInfo[] {
		return actionParentsMap.get(actionId) || [];
	}

	// Tracks which row is currently dispatching so the matching Play
	// button can show a spinner / stay disabled. Keyed by
	// "sourceType:sourceId" — same shape as directAssignmentMap.
	let dispatchingKey = $state<string | null>(null);

	async function dispatchFor(
		sourceType: AssignmentSourceType,
		sourceId: string,
		displayName: string
	) {
		const key = `${sourceType}:${sourceId}`;
		dispatchingKey = key;
		try {
			switch (sourceType) {
				case AssignmentSourceType.ACTION:
					await apiClient.dispatchAction(deviceId, sourceId);
					break;
				case AssignmentSourceType.ACTION_SET:
					await apiClient.dispatchActionSet(deviceId, sourceId);
					break;
				case AssignmentSourceType.DEFINITION:
					await apiClient.dispatchDefinition(deviceId, sourceId);
					break;
				default:
					toast.error(m.policies_dispatch_unsupported({ name: displayName }));
					return;
			}
			toast.success(m.policies_dispatch_dispatched({ name: displayName }));
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			dispatchingKey = null;
		}
	}
</script>

<div class="space-y-4">
	{#if loading}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<div
				class="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent"
			></div>
		</div>
	{:else if isEmpty}
		<div
			class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface py-12 text-center shadow-plate"
		>
			<ShieldCheck class="mb-2 h-8 w-8 text-muted-foreground" />
			<p class="text-muted-foreground">{m.policies_no_policies()}</p>
		</div>
	{:else}
		<!-- Definitions -->
		{#if definitions.length > 0}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex items-center gap-2">
					<BookOpen class="h-4 w-4 text-faint" />
					<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
						{m.policies_definitions()}
					</span>
				</div>
				<div class="mt-3">
					<div class="space-y-2">
						{#each definitions as def}
							{@const assignment = getDirectAssignment(AssignmentSourceType.DEFINITION, def.id)}
							{@const dispatchKey = `${AssignmentSourceType.DEFINITION}:${def.id}`}
							<div
								role="button"
								tabindex="0"
								class="flex w-full items-center justify-between rounded-md p-3 text-left transition-colors hover:bg-muted"
								onclick={() => goto(`/definitions/${def.id}`)}
								onkeydown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') goto(`/definitions/${def.id}`);
								}}
							>
								<div class="space-y-0.5">
									<div class="font-medium">{def.name}</div>
									{#if def.description}
										<div class="text-xs text-muted-foreground">
											{def.description}
										</div>
									{/if}
								</div>
								<div class="flex items-center gap-2">
									<span class="text-xs text-muted-foreground"
										>{m.policies_member_count({
											count: String(def.memberCount)
										})}</span
									>
									{#if assignment}
										<Chip tone={getModeTone(assignment.mode)} label={getModeLabel(assignment.mode)} />
										<Badge variant="outline" class="text-xs">
											{m.policies_direct()}
										</Badge>
									{:else}
										<Badge variant="outline" class="text-xs">
											{m.policies_inherited()}
										</Badge>
									{/if}
									<Button
										variant="ghost"
										size="icon"
										class="h-7 w-7"
										title={m.policies_dispatch_button_title()}
										disabled={dispatchingKey !== null}
										onclick={(e) => {
											e.stopPropagation();
											dispatchFor(AssignmentSourceType.DEFINITION, def.id, def.name);
										}}
									>
										<Play class={`h-3.5 w-3.5 ${dispatchingKey === dispatchKey ? "animate-pulse" : ""}`} />
									</Button>
								</div>
							</div>
						{/each}
					</div>
				</div>
			</section>
		{/if}

		<!-- Action Sets -->
		{#if actionSets.length > 0}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex items-center gap-2">
					<Layers class="h-4 w-4 text-faint" />
					<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
						{m.policies_action_sets()}
					</span>
				</div>
				<div class="mt-3">
					<div class="space-y-2">
						{#each actionSets as set}
							{@const assignment = getDirectAssignment(AssignmentSourceType.ACTION_SET, set.id)}
							{@const dispatchKey = `${AssignmentSourceType.ACTION_SET}:${set.id}`}
							<div
								role="button"
								tabindex="0"
								class="flex w-full items-center justify-between rounded-md p-3 text-left transition-colors hover:bg-muted"
								onclick={() => goto(`/action-sets/${set.id}`)}
								onkeydown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') goto(`/action-sets/${set.id}`);
								}}
							>
								<div class="space-y-0.5">
									<div class="font-medium">{set.name}</div>
									{#if set.description}
										<div class="text-xs text-muted-foreground">
											{set.description}
										</div>
									{/if}
								</div>
								<div class="flex items-center gap-2">
									<span class="text-xs text-muted-foreground"
										>{m.policies_member_count({
											count: String(set.memberCount)
										})}</span
									>
									{#if assignment}
										<Chip tone={getModeTone(assignment.mode)} label={getModeLabel(assignment.mode)} />
										<Badge variant="outline" class="text-xs">
											{m.policies_direct()}
										</Badge>
									{:else}
										<Badge variant="outline" class="text-xs">
											{m.policies_inherited()}
										</Badge>
									{/if}
									<Button
										variant="ghost"
										size="icon"
										class="h-7 w-7"
										title={m.policies_dispatch_button_title()}
										disabled={dispatchingKey !== null}
										onclick={(e) => {
											e.stopPropagation();
											dispatchFor(AssignmentSourceType.ACTION_SET, set.id, set.name);
										}}
									>
										<Play class={`h-3.5 w-3.5 ${dispatchingKey === dispatchKey ? "animate-pulse" : ""}`} />
									</Button>
								</div>
							</div>
						{/each}
					</div>
				</div>
			</section>
		{/if}

		<!-- Actions Table -->
		{#if uniqueActions.length > 0}
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex flex-wrap items-center gap-2">
					<Zap class="h-4 w-4 text-faint" />
					<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
						{m.policies_actions()}
					</span>
					<span class="text-sm text-muted-foreground"
						>{m.policies_actions_count({
							count: String(uniqueActions.length)
						})}</span
					>
				</div>
				<div class="relative mt-3">
					<Search class="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
					<Input
						type="search"
						placeholder={m.policies_search_actions()}
						class="pl-9"
						bind:value={searchQuery}
					/>
				</div>
				<div class="mt-3">
					<div class="max-h-[600px] overflow-auto">
						<Table.Root>
							<Table.Header>
								<Table.Row>
									<Table.Head class="w-[180px]">{m.common_type()}</Table.Head>
									<Table.Head>{m.common_name()}</Table.Head>
									<Table.Head class="w-[160px] text-right">{m.common_status()}</Table.Head>
									<Table.Head class="w-[48px]"></Table.Head>
								</Table.Row>
							</Table.Header>
							<Table.Body>
								{#each filteredActions as action}
									{@const typeInfo = getActionTypeInfo(action.type)}
									{@const TypeIcon = typeInfo.icon}
									{@const assignment = getDirectAssignment(AssignmentSourceType.ACTION, action.id)}
									{@const dispatchKey = `${AssignmentSourceType.ACTION}:${action.id}`}
									<Table.Row
										class="cursor-pointer"
										onclick={() => openActionSheet(action.id)}
									>
										<Table.Cell>
											<div class="flex items-center gap-2">
												<TypeIcon class="h-3.5 w-3.5 text-muted-foreground" />
												<span class="text-xs">{typeInfo.label}</span>
											</div>
										</Table.Cell>
										<Table.Cell class="font-medium text-sm">{action.name}</Table.Cell>
										<Table.Cell class="text-right">
											<div class="flex items-center justify-end gap-1.5">
												{#if assignment}
													<Chip tone={getModeTone(assignment.mode)} label={getModeLabel(assignment.mode)} />
													<Badge variant="outline" class="text-xs">
														{m.policies_direct()}
													</Badge>
												{:else}
													<Badge variant="outline" class="text-xs">
														{m.policies_inherited()}
													</Badge>
												{/if}
											</div>
										</Table.Cell>
										<Table.Cell class="text-right">
											<Button
												variant="ghost"
												size="icon"
												class="h-7 w-7"
												title={m.policies_dispatch_button_title()}
												disabled={dispatchingKey !== null}
												onclick={(e) => {
													e.stopPropagation();
													dispatchFor(AssignmentSourceType.ACTION, action.id, action.name);
												}}
											>
												<Play class={`h-3.5 w-3.5 ${dispatchingKey === dispatchKey ? "animate-pulse" : ""}`} />
											</Button>
										</Table.Cell>
									</Table.Row>
								{/each}
							</Table.Body>
						</Table.Root>
					</div>
				</div>
			</section>
		{/if}
	{/if}
</div>

<!-- Action Detail Sheet (shallow routing) -->
<ActionDetailSheet>
	{#if sheetActionId}
		{@const parents = getParents(sheetActionId)}
		{@const assignment = getDirectAssignment(AssignmentSourceType.ACTION, sheetActionId)}
		{#if parents.length > 0 || assignment}
			<div class="space-y-3">
				<h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
					{m.policies_part_of()}
				</h3>
				<div class="space-y-1">
					{#if assignment && parents.length === 0}
						<p class="text-sm text-muted-foreground">
							{m.policies_direct()}
						</p>
					{/if}
					{#each parents as parent}
						<div class="space-y-0">
							{#if parent.definitionId}
								<button
									type="button"
									class="flex w-full items-center gap-2 rounded-md p-2 text-left text-sm transition-colors hover:bg-muted"
									onclick={() => goto(`/definitions/${parent.definitionId}`)}
								>
									<BookOpen
										class="h-4 w-4 text-muted-foreground shrink-0"
									/>
									<span class="truncate">{parent.definitionName}</span>
									<Badge variant="outline" class="text-xs shrink-0 ml-auto">
										{m.policies_definitions()}
									</Badge>
								</button>
								<button
									type="button"
									class="flex w-full items-center gap-2 rounded-md py-1.5 pl-8 pr-2 text-left text-xs text-muted-foreground transition-colors hover:bg-muted"
									onclick={() => goto(`/action-sets/${parent.id}`)}
								>
									<span class="shrink-0">{'\u21B3'}</span>
									<Layers class="h-3.5 w-3.5 shrink-0" />
									<span class="truncate">{parent.name}</span>
									<Badge variant="outline" class="text-xs shrink-0 ml-auto">
										{m.policies_action_sets()}
									</Badge>
								</button>
							{:else}
								<button
									type="button"
									class="flex w-full items-center gap-2 rounded-md p-2 text-left text-sm transition-colors hover:bg-muted"
									onclick={() => goto(`/action-sets/${parent.id}`)}
								>
									<Layers
										class="h-4 w-4 text-muted-foreground shrink-0"
									/>
									<span class="truncate">{parent.name}</span>
									<Badge variant="outline" class="text-xs shrink-0 ml-auto">
										{m.policies_action_sets()}
									</Badge>
								</button>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}
	{/if}
</ActionDetailSheet>
