<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient, fetchAllPages, type Assignment, type Device, type DeviceGroup, type User, type UserGroup } from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import AssignTargetDialog from '$lib/components/assign-target-dialog.svelte';
	import { Plus, Monitor, Users, Trash2, UserRound, UsersRound } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { AssignmentMode, AssignmentSourceType, AssignmentTargetType } from '$contract/cadestro/v1/common_pb';
	import { getLocalizedError } from '$lib/errors';

	interface Props {
		sourceType: AssignmentSourceType;
		sourceId: string;
		title: string;
		subtitle: string;
		assignTitle: string;
		assignDescription: string;

		assignOpen?: boolean;
	}

	let {
		sourceType,
		sourceId,
		title,
		subtitle,
		assignTitle,
		assignDescription,
		assignOpen = $bindable(false)
	}: Props = $props();

	let assignments = $state<Assignment[]>([]);
	let devices = $state<Device[]>([]);
	let deviceGroups = $state<DeviceGroup[]>([]);
	let allUsers = $state<User[]>([]);
	let allUserGroups = $state<UserGroup[]>([]);
	let assignDialogOpen = $state(false);

	$effect(() => {
		if (!assignOpen) return;
		assignOpen = false;
		loadTargets();
		assignDialogOpen = true;
	});

	const availableDevices = $derived(
		devices.filter((d) => !assignments.some((a) => a.targetType === AssignmentTargetType.DEVICE && a.targetId?.value === d.id?.value))
	);

	const availableGroups = $derived(
		deviceGroups.filter((g) => !assignments.some((a) => a.targetType === AssignmentTargetType.DEVICE_GROUP && a.targetId?.value === g.id?.value))
	);

	const availableUsers = $derived(
		allUsers.filter((u) => !assignments.some((a) => a.targetType === AssignmentTargetType.USER && a.targetId?.value === u.id?.value))
	);

	const availableUserGroups = $derived(
		allUserGroups.filter((g) => !assignments.some((a) => a.targetType === AssignmentTargetType.USER_GROUP && a.targetId?.value === g.id?.value))
	);

	let loadSeq = 0;

	$effect(() => {
		if (sourceId) {
			loadAssignments();
		}
	});

	async function loadAssignments() {
		const seq = ++loadSeq;
		const requestedSourceId = sourceId;
		try {
			const response = await apiClient.listAssignments(50, '', sourceType, requestedSourceId);

			if (seq !== loadSeq || requestedSourceId !== sourceId) return;
			assignments = response.assignments;
		} catch (error) {
			console.error('Failed to load assignments', error);
		}
	}

	async function loadTargets() {
		try {

			const [devicesRes, groupsRes, usersAll, userGroupsAll] = await Promise.all([
				apiClient.listDevices(),
				apiClient.listDeviceGroups(),
				fetchAllPages<User>(async (size, token) => {
					const r = await apiClient.listUsers(size, token);
					return { items: r.users, nextPageToken: r.nextPageToken };
				}),
				fetchAllPages<UserGroup>(async (size, token) => {
					const r = await apiClient.listUserGroups(size, token);
					return { items: r.groups, nextPageToken: r.nextPageToken };
				})
			]);
			devices = devicesRes.devices;
			deviceGroups = groupsRes.groups;
			allUsers = usersAll;
			allUserGroups = userGroupsAll;
		} catch (error) {
			console.error('Failed to load targets', error);
			toast.error(getLocalizedError(error));
		}
	}

	async function createAssignments(targets: { targetType: AssignmentTargetType; targetId: string }[], mode: AssignmentMode) {
		if (targets.length === 0) return;
		try {
			await apiClient.batchCreateAssignments(sourceType, sourceId, targets, mode);
			toast.success(m.assignments_created({ count: targets.length }));
			assignDialogOpen = false;
			await loadAssignments();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error('Failed to create assignments', error);
		}
	}

	async function deleteAssignment(assignmentId: string) {
		try {
			await apiClient.deleteAssignment(assignmentId);
			toast.success(m.assignments_removed());
			await loadAssignments();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error('Failed to delete assignment', error);
		}
	}

	function getTargetName(assignment: Assignment): string {
		return assignment.targetName || (assignment.targetId?.value ?? '').slice(0, 8);
	}

	function getTargetIcon(targetType: AssignmentTargetType) {
		switch (targetType) {
			case AssignmentTargetType.DEVICE: return Monitor;
			case AssignmentTargetType.DEVICE_GROUP: return Users;
			case AssignmentTargetType.USER: return UserRound;
			case AssignmentTargetType.USER_GROUP: return UsersRound;
			default: return Monitor;
		}
	}

	function getTargetLabel(targetType: AssignmentTargetType): string {
		switch (targetType) {
			case AssignmentTargetType.DEVICE: return m.assignments_target_device();
			case AssignmentTargetType.DEVICE_GROUP: return m.assignments_target_group();
			case AssignmentTargetType.USER: return m.assignments_target_user();
			case AssignmentTargetType.USER_GROUP: return m.assignments_target_user_group();
			default: return String(targetType);
		}
	}

	function getModeBadgeVariant(mode: AssignmentMode): 'default' | 'secondary' | 'destructive' {
		switch (mode) {
			case AssignmentMode.AVAILABLE: return 'secondary';
			case AssignmentMode.UNINSTALL: return 'destructive';
			case AssignmentMode.EXCLUDED: return 'destructive';
			default: return 'default';
		}
	}

	function getModeLabel(mode: AssignmentMode): string {
		switch (mode) {
			case AssignmentMode.AVAILABLE: return m.assignments_mode_available_short();
			case AssignmentMode.EXCLUDED: return m.assignments_mode_excluded_short();
			case AssignmentMode.UNINSTALL: return m.assignments_mode_uninstall_short();
			default: return m.assignments_mode_required_short();
		}
	}
</script>

<Card.Root>
	<Card.Header>

		<Card.Title>{title}</Card.Title>
		<Card.Description>{subtitle}</Card.Description>
	</Card.Header>
	<Card.Content>
		{#if assignments.length === 0}
			<p class="text-muted-foreground text-sm text-center py-4">{m.assignments_no_assignments()}</p>
		{:else}
			<div class="space-y-2">
				{#each assignments as assignment}
					{@const Icon = getTargetIcon(assignment.targetType)}
					<div class="flex items-center justify-between p-2 rounded-md hover:bg-muted">
						<div class="flex items-center gap-2">
							<Icon class="h-4 w-4 text-muted-foreground" />
							<span class="font-medium">{getTargetName(assignment)}</span>
							<Badge variant="outline" class="text-xs">
								{getTargetLabel(assignment.targetType)}
							</Badge>
							<Badge variant={getModeBadgeVariant(assignment.mode)} class="text-xs">
								{getModeLabel(assignment.mode)}
							</Badge>
						</div>
						<Button variant="ghost" size="icon" onclick={() => deleteAssignment((assignment.id?.value ?? ''))}>
							<Trash2 class="h-4 w-4 text-destructive" />
						</Button>
					</div>
				{/each}
			</div>
		{/if}
	</Card.Content>
</Card.Root>

<AssignTargetDialog
	bind:open={assignDialogOpen}
	title={assignTitle}
	description={assignDescription}
	devices={availableDevices.map((device) => ({ ...device, id: device.id?.value ?? '' }))}
	deviceGroups={availableGroups.map((group) => ({ ...group, id: group.id?.value ?? '' }))}
	users={availableUsers.map((user) => ({ ...user, id: user.id?.value ?? '' }))}
	userGroups={availableUserGroups.map((group) => ({ ...group, id: group.id?.value ?? '' }))}
	onassign={createAssignments}
/>
