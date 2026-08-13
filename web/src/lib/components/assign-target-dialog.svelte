<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Select from '$lib/components/ui/select';
	import AssignTargetPicker from '$lib/components/assign-target-picker.svelte';
	import * as m from '$lib/paraglide/messages';
	import { AssignmentMode, AssignmentTargetType, type DeviceStatus } from '$sdk/powermanage/v1/common_pb';

	interface Props {
		open: boolean;
		title: string;
		description: string;
		devices: { id: string; hostname: string; status: DeviceStatus }[];
		deviceGroups: { id: string; name: string; description?: string }[];
		users: { id: string; email: string }[];
		userGroups: { id: string; name: string; description?: string }[];
		onassign: (
			targets: { targetType: AssignmentTargetType; targetId: string }[],
			mode: AssignmentMode
		) => void;
	}

	let { open = $bindable(), title, description, devices, deviceGroups, users, userGroups, onassign }: Props =
		$props();

	let assignmentMode = $state('0');
	let selectedDeviceIds = $state<string[]>([]);
	let selectedGroupIds = $state<string[]>([]);
	let selectedUserIds = $state<string[]>([]);
	let selectedUserGroupIds = $state<string[]>([]);

	$effect(() => {
		if (open) {
			assignmentMode = '0';
			selectedDeviceIds = [];
			selectedGroupIds = [];
			selectedUserIds = [];
			selectedUserGroupIds = [];
		}
	});

	function handleAssign() {
		const targets: { targetType: AssignmentTargetType; targetId: string }[] = [
			...selectedDeviceIds.map((id) => ({
				targetType: AssignmentTargetType.DEVICE,
				targetId: id
			})),
			...selectedGroupIds.map((id) => ({
				targetType: AssignmentTargetType.DEVICE_GROUP,
				targetId: id
			})),
			...selectedUserIds.map((id) => ({
				targetType: AssignmentTargetType.USER,
				targetId: id
			})),
			...selectedUserGroupIds.map((id) => ({
				targetType: AssignmentTargetType.USER_GROUP,
				targetId: id
			}))
		];
		if (targets.length === 0) return;
		onassign(targets, parseInt(assignmentMode) as AssignmentMode);
	}

	const hasSelection = $derived(
		selectedDeviceIds.length > 0 ||
		selectedGroupIds.length > 0 ||
		selectedUserIds.length > 0 ||
		selectedUserGroupIds.length > 0
	);
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{title}</Dialog.Title>
			<Dialog.Description>
				{description}
			</Dialog.Description>
		</Dialog.Header>
		<div class="py-4 space-y-4">
			<div class="space-y-2">
				<Label>{m.assignments_mode()}</Label>
				<Select.Root type="single" bind:value={assignmentMode}>
					<Select.Trigger>
						{assignmentMode === '0'
							? m.assignments_mode_required()
							: assignmentMode === '1'
								? m.assignments_mode_available()
								: assignmentMode === '2'
									? m.assignments_mode_excluded()
									: m.assignments_mode_uninstall()}
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="0">{m.assignments_mode_required()}</Select.Item>
						<Select.Item value="1">{m.assignments_mode_available()}</Select.Item>
						<Select.Item value="2">{m.assignments_mode_excluded()}</Select.Item>
						<Select.Item value="3">{m.assignments_mode_uninstall()}</Select.Item>
					</Select.Content>
				</Select.Root>
			</div>
			<AssignTargetPicker
				{devices}
				{deviceGroups}
				{users}
				{userGroups}
				bind:selectedDeviceIds
				bind:selectedGroupIds
				bind:selectedUserIds
				bind:selectedUserGroupIds
			/>
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button
				onclick={handleAssign}
				disabled={!hasSelection}
				>{m.common_assign()}</Button
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
