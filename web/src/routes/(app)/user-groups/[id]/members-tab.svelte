<script lang="ts">

	import { base } from '$app/paths';
	import { Tile, Chip } from '$lib/components/fleet';
	import { Button } from '$lib/components/ui/button';
	import type { UserGroupMember } from '$lib/sdk';
	import { UserPlus, UserMinus } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		members: UserGroupMember[];
		isDynamic: boolean;
		isScimManaged: boolean;
		onadd: () => void;
		onremove: (userId: string) => void;
	}

	let { members, isDynamic, isScimManaged, onadd, onremove }: Props = $props();

	const managed = $derived(isDynamic || isScimManaged);
</script>

<div class="rounded-xl border border-hair bg-surface shadow-plate" data-tour="group-members" data-testid="members-tab">
	<div class="flex items-center justify-between border-b px-3 py-2">
		<span class="font-mono text-[0.62rem] tracking-[0.08em] text-faint uppercase">
			{m.user_group_detail_members()}
		</span>
		{#if isScimManaged}
			<Chip tone="info" label={m.user_groups_scim_managed()} />
		{:else if isDynamic}
			<Chip tone="info" label={m.device_group_detail_auto_managed()} />
		{:else}
			<Button size="sm" variant="outline" onclick={onadd} data-testid="add-member">
				<UserPlus class="mr-1 h-3.5 w-3.5" />
				{m.user_group_detail_add_member()}
			</Button>
		{/if}
	</div>

	{#if isScimManaged}
		<p class="border-b border-hair bg-warn-soft px-3 py-2 text-xs text-warn" data-testid="scim-note">
			{m.user_groups_scim_lifecycle_note()}
		</p>
	{:else if isDynamic}
		<p class="border-b border-hair px-3 py-2 text-xs text-muted-foreground">
			{m.user_group_detail_dynamic_hint()}
		</p>
	{/if}

	{#if members.length === 0}
		<p class="px-3 py-8 text-center text-sm text-muted-foreground">
			{m.user_group_detail_no_members()}
		</p>
	{:else}
		<div class="max-h-[28rem] overflow-y-auto">
			{#each members as member (member.userId?.value ?? '')}
				<div class="flex items-center gap-2.5 border-b border-hair px-3 py-1.5 last:border-b-0">
					<span class="w-3.5 shrink-0"><Tile tone="info" label={member.email} /></span>
					<a href="{base}/users/{member.userId?.value ?? ''}" class="truncate font-mono text-[0.82rem] hover:underline">
						{member.email}
					</a>
					<span class="ml-auto hidden truncate font-mono text-[0.68rem] text-faint sm:inline">
						{member.userId?.value ?? ''}
					</span>
					{#if !managed}
						<Button
							variant="ghost"
							size="icon-sm"
							class="shrink-0 text-muted-foreground hover:text-destructive"
							aria-label={m.user_groups_remove_member()}
							onclick={() => onremove((member.userId?.value ?? ''))}
						>
							<UserMinus class="h-3.5 w-3.5" />
						</Button>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
