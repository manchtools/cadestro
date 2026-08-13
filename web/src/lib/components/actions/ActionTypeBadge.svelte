<script lang="ts">
	import { ActionType } from '$sdk/powermanage/v1/actions_pb';
	import { Badge } from '$lib/components/ui/badge';
	import { getActionTypeInfo, getActionTypeInfoByValue } from './action-type';

	interface Props {
		type: ActionType;
		isCompliance?: boolean;
		showIcon?: boolean;
		class?: string;
	}

	let { type, isCompliance = false, showIcon = true, class: className }: Props = $props();

	const info = $derived(
		isCompliance && type === ActionType.SHELL
			? getActionTypeInfoByValue('COMPLIANCE_CHECK')
			: getActionTypeInfo(type)
	);
</script>

<Badge variant="outline" class={className}>
	{#if showIcon}
		<info.icon class="h-3 w-3 mr-1" />
	{/if}
	{info.label}
</Badge>
