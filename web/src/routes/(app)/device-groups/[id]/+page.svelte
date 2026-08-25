<script lang="ts">
	// Device group detail — entity header card + tabs with counts.
	//
	// Identity (name, description) is a committable surface, so it commits from
	// the context pill instead of two one-shot dialogs. The interval editors stay
	// dialogs: they are one-shot pickers with no draft to carry.
	//
	// The pill is ALSO this group's action bar: it is held for the whole visit, so
	// Delete and the maintenance window have a home that does not depend on having
	// typed something first. `dirty` still tells the truth about the identity
	// buffer, which is what turns Save/Stash/Cancel on and what decides whether
	// leaving parks a draft.
	import { onMount, onDestroy, untrack } from 'svelte';
	import { getLocalizedError } from '$lib/errors';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, fetchAllPages, DeviceStatus, type DeviceGroup, type Device } from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';
	import { createFormValidation } from '$lib/forms';
	import { editNameSchema } from '$lib/forms/schemas/common';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Tile, Chip, Stat } from '$lib/components/fleet';
	import ItemTablePicker from '$lib/components/item-table-picker.svelte';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import SyncIntervalDialog, { formatSyncInterval } from '$lib/components/sync-interval-dialog.svelte';
	import InventoryIntervalDialog, {
		formatInventoryInterval
	} from '$lib/components/inventory-interval-dialog.svelte';
	import MaintenanceWindowDialog, {
		entriesFromWindow,
		type MaintenanceWindowEntryInput
	} from '$lib/components/maintenance-window-dialog.svelte';
	import DynamicRuleEditor, {
		type RulePreviewRow,
		ruleSubtext
	} from '$lib/components/dynamic-rule-editor.svelte';
	import FutureScopeDialog from '$lib/components/future-scope-dialog.svelte';
	import type { QueryEditorState } from '$lib/components/query-builder.svelte';
	import MembersTab from './members-tab.svelte';
	import { create } from '@bufbuild/protobuf';
	import {
		MaintenanceWindowSchema,
		MaintenanceWindowEntrySchema
	} from '$contract/cadestro/v1/common_pb';
	import {
		shell,
		enterContext,
		updateContext,
		exitContext,
		leaveContext,
		removeDraft,
		claimDraft,
		draftIdFor,
		type ContextState
	} from '$lib/shell/shell.svelte';
	import { ArrowLeft, RefreshCw, Pencil, Play, Clock } from '@lucide/svelte';

	let group = $state<DeviceGroup | null>(null);
	let memberDevices = $state<Array<{ deviceId: string; hostname: string; agentVersion: string }>>([]);
	let memberDeviceIds = $state<string[]>([]);
	let allDevices = $state<Device[]>([]);
	let loading = $state(true);
	let evaluating = $state(false);
	let activeTab = $state('members');

	let deleteDialogOpen = $state(false);
	let addDeviceDialogOpen = $state(false);
	let selectedDeviceIds = $state<string[]>([]);
	let syncIntervalDialogOpen = $state(false);
	let inventoryIntervalDialogOpen = $state(false);
	let maintenanceWindowDialogOpen = $state(false);

	// Identity draft — the pill owns its commit.
	let editingIdentity = $state(false);
	let savingIdentity = $state(false);
	let draftName = $state('');
	let draftDescription = $state('');
	const nameValidation = createFormValidation(editNameSchema);

	const groupId = $derived(page.params.id ?? '');
	// ONE context for the whole group. The Rule tab used to publish a second one,
	// and with a single pill slot that meant two competing Saves: renaming a group
	// AND editing its rule took two separate save events, and whichever surface
	// held the bar decided which half of the work the button would commit.
	const groupContextId = $derived(`device-group:${groupId}`);
	const deviceById = $derived(new Map(allDevices.map((d) => [d.id, d])));
	const availableDevices = $derived(allDevices.filter((d) => !memberDeviceIds.includes(d.id)));
	// Gated on `editingIdentity`: the pill is held even when the fields are shut,
	// and a group the operator only looked at must never park a draft on the stage.
	const identityDirty = $derived(
		editingIdentity &&
			group !== null &&
			(draftName !== group.name || draftDescription !== group.description)
	);
	const identityNameValid = $derived(draftName.trim().length > 0);

	// The rule's edit buffer and its live validation, reported up by the editor.
	let draftQuery = $state('');
	let ruleState = $state<QueryEditorState>({
		text: '',
		valid: false,
		count: null,
		error: m.query_incomplete(),
		validating: false
	});
	let ruleConfirmOpen = $state(false);
	/** The stored rule as last seen, so a reload cannot clobber an edit. */
	let lastSavedQuery = '';
	const savedQuery = $derived(group?.dynamicQuery ?? '');
	const ruleDirty = $derived(group !== null && draftQuery !== savedQuery);
	/** A half-typed chip blocks the commit even when it has not moved the compiled
	 *  string yet — an incomplete condition silently drops out of the compile, so
	 *  gating on dirtiness alone would let Save look armed over a rule the operator
	 *  is still mid-way through writing. */
	const ruleValid = $derived(ruleState.valid === true);

	/** Labels shown inline on a preview row before it runs out of width. */
	const PREVIEW_LABEL_CAP = 3;

	const previewRows = $derived<RulePreviewRow[]>(
		memberDevices.map((row) => {
			const device = deviceById.get(row.deviceId);
			const all = Object.entries(device?.labels ?? {}).map(([k, v]) => `${k}=${v}`);
			return {
				id: row.deviceId,
				primary: row.hostname,
				attributes: all.slice(0, PREVIEW_LABEL_CAP),
				// Say what the cap left off: a three-label device and a nine-label
				// one must not read identically.
				hiddenAttributes: Math.max(0, all.length - PREVIEW_LABEL_CAP),
				tone:
					device?.status === DeviceStatus.ONLINE
						? 'ok'
						: device?.status === DeviceStatus.OFFLINE
							? 'crit'
							: 'idle'
			};
		})
	);

	onMount(() => {
		if (groupId) loadData();
	});

	// If the page goes away mid-edit the pill must not keep pointing at a group
	// that is no longer on screen — and must not DISCARD the unsaved identity.
	// Auto-stash-on-navigate parks it on the stage instead; a commit/cancel/stash
	// already cleared `owns`, so this only fires on a genuine leave.
	onDestroy(() => {
		if (owns) {
			owns = false;
			leaveContext(groupContextId);
		}
	});

	async function loadData() {
		loading = true;
		try {
			const response = await apiClient.getDeviceGroup(groupId);
			group = response.group ?? null;
			memberDeviceIds = response.deviceIds ?? [];
			memberDevices = response.devices ?? [];
			if (group) {
				if (!editingIdentity) {
					draftName = group.name;
					draftDescription = group.description;
				}
				// The rule rebases on a fresh read unless the operator is mid-edit,
				// exactly like the identity fields above it.
				// Rebase the rule buffer on the STORED rule whenever the stored rule
				// itself moved (first read, or a save landing) — never on an unrelated
				// reload, which would eat an edit in progress. Imperative and ordered,
				// so the parked-draft restore below still wins.
				if (group.dynamicQuery !== lastSavedQuery) {
					lastSavedQuery = group.dynamicQuery;
					draftQuery = group.dynamicQuery;
				}
				// …then take back a draft this page parked on the stage. The buffer is
				// component state, so it did NOT survive the unmount: the stash
				// snapshotted it onto the card and this is where it comes back.
				const parked = claimDraft(groupContextId) as GroupDraft | undefined;
				if (parked) {
					draftName = parked.name;
					draftDescription = parked.description;
					if (parked.query !== undefined) draftQuery = parked.query;
					editingIdentity = true;
				}
			}
			// F022 pattern: page through the fleet so the picker and the member
			// attributes are complete, not capped at the first page.
			allDevices = await fetchAllPages<Device>(async (size, token) => {
				const r = await apiClient.listDevices(size, token);
				return { items: r.devices, nextPageToken: r.nextPageToken };
			});
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	// ── the pill is this group's action bar ──────────────────────────────────
	/** This page's home — a stashed draft restores by navigating back to it. */
	const contextRoute = $derived(`/device-groups/${groupId}`);

	/** What Stash has to carry. These buffers are component state, so an unmount
	 *  destroys them: the card must hold them itself. The rule rides along — it is
	 *  the same context now, and a parked card that dropped the query would lose
	 *  half the operator's work. */
	interface GroupDraft {
		name: string;
		description: string;
		query: string;
	}

	// Plain `let`, not `$state`: the effect writes it, so a tracked read would
	// make the effect depend on its own write.
	let owns = false;
	/** The operator set this context aside on purpose: do not take the bar
	 *  back when the slot frees, or Stash would undo itself. */
	let stashParked = false;

	function startIdentityEdit() {
		draftName = group?.name ?? '';
		draftDescription = group?.description ?? '';
		nameValidation.clearErrors();
		editingIdentity = true;
	}

	function revertIdentityEdit() {
		editingIdentity = false;
		draftName = group?.name ?? '';
		draftDescription = group?.description ?? '';
	}

	function contextState(): ContextState {
		return {
			id: groupContextId,
			route: contextRoute,
			title: draftName || (group?.name ?? ''),
			// Either half counts. One bar, one Save, whichever tab is open.
			dirty: identityDirty || ruleDirty,
			valid: identityNameValid && ruleValid,
			// Converting a static group is not a "save" — name what it does.
			commitLabel:
				ruleDirty && group && !group.isDynamic ? m.query_commit_convert() : m.common_save(),
			// Every context explains itself; a greyed Save with no reason is a dead
			// button. The rule's caption is the shared one, so the count and the
			// compiled query read identically wherever they appear.
			subtext: !identityNameValid
				? m.validation_name_required()
				: ruleDirty || !ruleValid
					? ruleSubtext(ruleState, 'device').text
					: undefined,
			subtextTone: (!identityNameValid
				? 'warn'
				: ruleDirty || !ruleValid
					? ruleSubtext(ruleState, 'device').tone
					: 'neutral') as 'neutral' | 'warn',
			onCommit: () => {
				// The store already exited context; a FAILED save re-acquires it below,
				// so the operator never loses the buffer with the commit.
				owns = false;
				// A standing rule is a future-scope decision, so it keeps its real
				// acknowledgement — but it now gates the ONE commit, not a second one.
				if (ruleDirty) ruleConfirmOpen = true;
				else void saveGroup();
			},
			onCancel: () => {
				owns = false;
				revertIdentityEdit();
				draftQuery = savedQuery;
			},
			// Stash releases the pill deliberately. The effect wakes when the slot
			// frees, so without a remembered intent it would re-acquire instantly
			// and the stash would never take.
			onStash: () => {
				stashParked = true;
				owns = false;
			},
			onRestore: () => {
				stashParked = false;
				owns = true;
			},
			stashPayload: (): GroupDraft => ({
				name: draftName,
				description: draftDescription,
				query: draftQuery
			}),
			stashSubtitle: m.device_groups_edit_identity(),
			// The group's own actions live in the pill, not as a trash glyph tucked
			// into the header card and an Edit button a tab away. Both keep their
			// dialogs — the pill is a shorter route to the gate, not a way past it.
			extraActions: [
				{
					id: 'window',
					label: m.device_group_detail_window_label(),
					onRun: () => (maintenanceWindowDialogOpen = true)
				},
				{
					id: 'delete',
					label: m.device_groups_delete_group(),
					tone: 'danger' as const,
					onRun: () => (deleteDialogOpen = true)
				}
			]
		};
	}

	function acquire() {
		owns = true;
		stashParked = false;
		// Resuming edits supersedes any card this context parked on the stage: the
		// live buffer is newer than the snapshot on the card.
		removeDraft(draftIdFor(groupContextId));
		enterContext(contextState());
	}

	function release() {
		owns = false;
		// Only tear down our own context — another surface may have taken over.
		if (shell.pill.context?.id === groupContextId) exitContext();
	}

	$effect(() => {
		// Read every reactive input HERE. `savingIdentity` parks the pill for the
		// round trip: an in-flight commit has no second commit.
		const active = group !== null && !savingIdentity;
		// Tracked, but deliberately NOT a gate. The Rule tab publishes its own
		// context and there is one pill, so whoever holds it keeps it — this only
		// wakes the effect on a tab switch, so the bar comes back when the rule
		// editor lets go. Gating on the tab instead is what made the pill reset to
		// nav the moment the operator opened the query, and patching our state onto
		// the rule editor's context is what greyed out its Save.
		void activeTab;
		// Tracked: this is what wakes the effect when whoever else held the bar
		// lets go. The rule editor exits its context the moment the query matches
		// the stored one again, and nothing else here would change — so the pill
		// stayed empty until the next keystroke.
		const holder = shell.pill.context?.id ?? null;
		// The WHOLE state, not a three-field subset. Once the rule joined this
		// context, a patch that carried only the identity fields left the pill
		// blind to it: Save stayed disabled over a valid rule edit, and the rule's
		// caption never reached the bar. Building the full state here also keeps
		// every reactive input tracked in one place.
		const patch = contextState();
		// …and write to the store UNTRACKED. The store helpers read
		// `shell.pill.context` themselves, so a tracked call would make this effect
		// depend on the pill it just wrote — and Stash, which clears the context,
		// would be undone by an instant re-acquire.
		untrack(() => {
			// Held is read from the STORE: another surface on this page (the Rule
			// tab's editor) may have taken the single context slot, and a stale
			// local flag made us patch OUR state onto ITS context — which set
			// dirty:false on a dirty query and greyed out its Save.
			const held = holder === groupContextId;
			if (!active) {
				if (held) release();
				return;
			}
			if (held) updateContext(patch);
			// Never stomp a context somebody else is holding — the Rule tab's editor
			// takes this slot the moment its query is dirty, and patching our state
			// onto it set dirty:false on a dirty query and greyed out its Save.
			else if (holder === null && !stashParked) acquire();
		});
	});

	/** The group's ONE commit: name, description and rule in a single save.
	 *
	 *  It used to take two — the Rule tab published its own context, so whichever
	 *  surface held the single pill slot committed only its own half and the
	 *  operator had to go back for the rest.
	 *
	 *  Order matters: identity first, so a rule that fails validation server-side
	 *  does not silently discard a rename the operator already saw applied. */
	async function saveGroup() {
		if (!group) return;
		const name = draftName.trim();
		const description = draftDescription.trim();
		const query = draftQuery;
		const wantsRule = ruleDirty;
		savingIdentity = true;
		try {
			await nameValidation.handleSubmit({ name }, async () => {
				try {
					if (name !== group!.name) {
						group = (await apiClient.renameDeviceGroup(groupId, name)) ?? group;
						toast.success(m.device_group_detail_name_updated());
					}
					if (description !== group!.description) {
						group = (await apiClient.updateDeviceGroupDescription(groupId, description)) ?? group;
						toast.success(m.device_group_detail_desc_updated());
					}
					if (wantsRule) {
						group = (await apiClient.updateDeviceGroupQuery(groupId, true, query)) ?? group;
						toast.success(m.device_group_detail_query_updated());
					}
					editingIdentity = false;
					await loadData();
				} catch (error) {
					toast.error(getLocalizedError(error));
					console.error(error);
				}
			});
		} finally {
			savingIdentity = false;
		}
	}

	// ── mutations ────────────────────────────────────────────────────────────
	async function deleteGroup() {
		try {
			await apiClient.deleteDeviceGroup(groupId);
			toast.success(m.device_groups_deleted());
			goto('/device-groups');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function addDevices() {
		if (selectedDeviceIds.length === 0) return;
		try {
			await apiClient.addDeviceToGroup(groupId, selectedDeviceIds);
			addDeviceDialogOpen = false;
			selectedDeviceIds = [];
			toast.success(m.device_group_detail_device_added());
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function removeDevice(deviceId: string) {
		try {
			await apiClient.removeDeviceFromGroup(groupId, deviceId);
			toast.success(m.device_group_detail_device_removed());
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function evaluateGroup() {
		evaluating = true;
		try {
			const result = await apiClient.evaluateDynamicGroup(groupId);
			toast.success(
				m.device_group_detail_evaluated({
					added: result.devicesAdded,
					removed: result.devicesRemoved
				})
			);
			await loadData();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			evaluating = false;
		}
	}

	async function updateSyncInterval(minutes: number) {
		try {
			group = (await apiClient.setDeviceGroupSyncInterval(groupId, minutes)) ?? null;
			toast.success(m.device_group_detail_sync_updated());
			syncIntervalDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateInventoryInterval(minutes: number) {
		try {
			group = (await apiClient.setDeviceGroupInventoryInterval(groupId, minutes)) ?? null;
			toast.success(m.device_group_detail_inventory_updated());
			inventoryIntervalDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateMaintenanceWindow(entries: MaintenanceWindowEntryInput[]) {
		try {
			// An empty entry list clears the group's contribution to the
			// device-side union; pass undefined so the wire shape drops the
			// schedule entirely.
			const window =
				entries.length === 0
					? undefined
					: create(MaintenanceWindowSchema, {
							schedule: entries.map((e) =>
								create(MaintenanceWindowEntrySchema, { days: e.days, allow: e.allow })
							)
						});
			group = (await apiClient.setDeviceGroupMaintenanceWindow(groupId, window)) ?? null;
			toast.success(m.device_group_detail_window_updated());
			maintenanceWindowDialogOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}
</script>

<PageShell contentClass="space-y-4">
	{#snippet header()}
		<div class="flex items-center gap-2">
			<Button variant="ghost" size="icon" aria-label={m.common_back()} onclick={() => history.back()}>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<!-- The ENTITY, not the section. This said "Device Groups" while the pill
			     held the group's name, so the two disagreed about where you were. -->
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-2xl font-bold">{group?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{groupId}</p>
			</div>
			<div class="ml-auto flex gap-2">
				{#if group?.isDynamic}
					<Button variant="outline" size="sm" onclick={evaluateGroup} disabled={evaluating}>
						<span class="mr-2 h-4 w-4" class:animate-spin={evaluating}><Play class="h-4 w-4" /></span>
						{m.device_group_detail_re_evaluate()}
					</Button>
				{/if}
				<Button variant="outline" size="sm" onclick={loadData} disabled={loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={loading}><RefreshCw class="h-4 w-4" /></span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	{#if loading && !group}
		<div class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate">
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if group}
		<!-- entity header card -->
		<div
			class="rounded-xl border border-hair bg-surface p-4 shadow-plate"
			data-tour="group-header"
			data-testid="group-header"
		>
			<div class="flex flex-wrap items-start gap-3">
				<span class="mt-1 w-4 shrink-0"><Tile tone={group.isDynamic ? 'info' : 'idle'} label={group.name} /></span>
				<div class="min-w-0 flex-1">
					{#if editingIdentity}
						<div class="space-y-1.5" data-testid="identity-edit">
							<!-- "The pill commits this" is a standing note about the whole
							     editor, so it goes first — trailing it after the fields told
							     the operator where Save lives only once they had stopped
							     looking for it. -->
							<p class="text-xs text-muted-foreground">{m.device_groups_identity_pill_hint()}</p>
							<Input
								bind:value={draftName}
								aria-label={m.common_name()}
								aria-invalid={!!nameValidation.errors.name}
								class="h-8 font-mono text-sm"
							/>
							<FieldError error={nameValidation.errors.name} />
							<Textarea
								bind:value={draftDescription}
								aria-label={m.common_description()}
								rows={2}
								placeholder={m.device_groups_desc_placeholder()}
								class="text-sm"
							/>
						</div>
					{:else}
						<div class="flex items-center gap-2">
							<h2 class="truncate font-mono text-lg font-semibold">{group.name}</h2>
							<Button
								variant="ghost"
								size="icon-sm"
								aria-label={m.device_groups_edit_identity()}
								onclick={startIdentityEdit}
							>
								<Pencil class="h-3.5 w-3.5" />
							</Button>
						</div>
						<p class="font-mono text-xs text-faint">{group.id}</p>
						<p class="mt-1 text-sm text-muted-foreground">
							{group.description || m.common_no_description()}
						</p>
					{/if}
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<Chip
						tone={group.isDynamic ? 'info' : 'idle'}
						label={group.isDynamic ? m.device_groups_dynamic() : m.device_groups_static()}
					/>
					<Stat
						tone={group.isDynamic ? 'info' : 'ok'}
						value={group.memberCount}
						label={m.device_group_detail_devices()}
					/>
					<!-- No trash glyph here: Delete acts on the whole group, so it is a
					     pill action. The confirm dialog is still the gate. -->
				</div>
			</div>
		</div>

		<Tabs.Root value={activeTab} onValueChange={(v) => (activeTab = v)}>
			<Tabs.List>
				<Tabs.Trigger value="members">
					{m.device_groups_members_tab({ count: group.memberCount })}
				</Tabs.Trigger>
				<Tabs.Trigger value="rule">{m.query_tab_rule()}</Tabs.Trigger>
				<Tabs.Trigger value="schedules">{m.device_groups_tab_schedules()}</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="members" class="mt-3">
				<MembersTab
					members={memberDevices}
					devices={deviceById}
					isDynamic={group.isDynamic}
					canAdd={availableDevices.length > 0}
					onadd={() => {
						selectedDeviceIds = [];
						addDeviceDialogOpen = true;
					}}
					onremove={removeDevice}
				/>
			</Tabs.Content>

			<Tabs.Content value="rule" class="mt-3">
				<DynamicRuleEditor
					kind="device"
					savedQuery={group.dynamicQuery}
					bind:draft={draftQuery}
					isDynamic={group.isDynamic}
					rows={previewRows}
					total={group.memberCount}
					onstate={(state) => (ruleState = state)}
				/>
			</Tabs.Content>

			<!-- Two cadence FIELDS of the group, edited where their current value is
			     read. The maintenance window is not here: it is a group-wide policy
			     that unions into every member device, so it moved to the pill with
			     Delete — a value row with no control would have been a hollow card. -->
			<Tabs.Content value="schedules" class="mt-3">
				<div class="divide-y rounded-xl border border-hair bg-surface shadow-plate" data-testid="schedules-tab">
					<div class="flex flex-wrap items-center gap-3 p-3">
						<div class="min-w-0 flex-1">
							<p class="text-sm font-medium">{m.sync_label()}</p>
							<p class="text-xs text-muted-foreground">{m.device_group_detail_sync_hint()}</p>
						</div>
						<Chip tone="idle">
							<Clock class="h-3 w-3" />{formatSyncInterval(group.syncIntervalMinutes)}
						</Chip>
						<Button variant="outline" size="sm" onclick={() => (syncIntervalDialogOpen = true)}>
							{m.common_edit()}
						</Button>
					</div>
					<div class="flex flex-wrap items-center gap-3 p-3">
						<div class="min-w-0 flex-1">
							<p class="text-sm font-medium">{m.inventory_interval_label()}</p>
						</div>
						<Chip tone="idle">
							<Clock class="h-3 w-3" />{formatInventoryInterval(group.inventoryIntervalMinutes)}
						</Chip>
						<Button variant="outline" size="sm" onclick={() => (inventoryIntervalDialogOpen = true)}>
							{m.common_edit()}
						</Button>
					</div>
				</div>
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</PageShell>

<!-- The future-scope acknowledgement gates the group's ONE commit: a standing
     rule decides membership from here on, and a banner you can scroll past is
     not an acknowledgement. Cancelling keeps the buffer and the pill. -->
<FutureScopeDialog
	bind:open={ruleConfirmOpen}
	queryText={ruleState.text}
	count={ruleState.count}
	kind="device"
	converting={!(group?.isDynamic ?? false)}
	currentMembers={group?.memberCount ?? 0}
	onconfirm={() => {
		ruleConfirmOpen = false;
		void saveGroup();
	}}
	oncancel={() => (ruleConfirmOpen = false)}
/>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.device_groups_delete_dialog_title()}
	description={m.device_groups_delete_dialog_description({ name: group?.name ?? '' })}
	onconfirm={deleteGroup}
/>

<Dialog.Root bind:open={addDeviceDialogOpen}>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{m.device_group_detail_add_dialog_title()}</Dialog.Title>
			<Dialog.Description>{m.device_group_detail_add_dialog_description()}</Dialog.Description>
		</Dialog.Header>

		<ItemTablePicker
			items={availableDevices.map((d) => ({ id: d.id, hostname: d.hostname, status: d.status }))}
			bind:selected={selectedDeviceIds}
			searchPlaceholder={m.picker_search_devices()}
			emptyMessage={m.picker_no_devices()}
			searchFilter={(item, query) =>
				item.hostname.toLowerCase().includes(query.toLowerCase()) ||
				item.id.toLowerCase().includes(query.toLowerCase())}
		>
			{#snippet headerRow()}
				<Table.Head>{m.devices_table_hostname()}</Table.Head>
			{/snippet}
			{#snippet itemRow(item)}
				<Table.Cell><span class="font-mono text-sm">{item.hostname}</span></Table.Cell>
			{/snippet}
		</ItemTablePicker>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (addDeviceDialogOpen = false)}>{m.common_cancel()}</Button>
			<Button onclick={addDevices} disabled={selectedDeviceIds.length === 0}>{m.common_add()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<SyncIntervalDialog
	bind:open={syncIntervalDialogOpen}
	currentMinutes={group?.syncIntervalMinutes ?? 0}
	title={m.device_group_detail_sync_dialog_title()}
	description={m.device_group_detail_sync_dialog_description()}
	onsave={updateSyncInterval}
/>

<InventoryIntervalDialog
	bind:open={inventoryIntervalDialogOpen}
	currentMinutes={group?.inventoryIntervalMinutes ?? 0}
	title={m.device_group_detail_inventory_dialog_title()}
	description={m.device_group_detail_inventory_dialog_description()}
	onsave={updateInventoryInterval}
/>

<MaintenanceWindowDialog
	bind:open={maintenanceWindowDialogOpen}
	entries={entriesFromWindow(group?.maintenanceWindow)}
	title={m.device_group_detail_window_dialog_title()}
	description={m.device_group_detail_window_dialog_description()}
	onsave={updateMaintenanceWindow}
/>
