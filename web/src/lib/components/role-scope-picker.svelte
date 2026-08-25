<script lang="ts">
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import * as Popover from '$lib/components/ui/popover';
	import * as Command from '$lib/components/ui/command';
	import { Check, ChevronsUpDown, Server, Users } from '@lucide/svelte';
	import { cn } from '$lib/utils';
	import * as m from '$lib/paraglide/messages';

	interface GroupOption {
		id: string;
		name: string;
		kind: 'device' | 'user';
	}

	let {
		groups,
		scopeId = $bindable(''),
		activeKind,
		label,
		allOptionLabel,
		hint,
		disabled = false
	}: {
		groups: GroupOption[];
		scopeId: string;

		activeKind: 'device' | 'user';
		label: string;
		allOptionLabel: string;
		hint: string;
		disabled?: boolean;
	} = $props();

	let open = $state(false);

	const buttonLabel = $derived(
		disabled || scopeId === ''
			? allOptionLabel
			: (groups.find((g) => g.id === scopeId)?.name ?? allOptionLabel)
	);

	const disabledNote = $derived(
		activeKind === 'device' ? m.roles_scope_only_device() : m.roles_scope_only_user()
	);

	function select(id: string) {
		scopeId = id;
		open = false;
	}
</script>

<div class="space-y-2">
	<Label id="role-scope-label">{label}</Label>
	<Popover.Root bind:open>
		<Popover.Trigger {disabled}>
			<Button
				variant="outline"
				role="combobox"
				aria-expanded={open}
				aria-labelledby="role-scope-label"
				aria-describedby="role-scope-hint"
				{disabled}
				title={disabled ? hint : undefined}
				class="w-full justify-between font-normal"
			>
				<span class="truncate">{buttonLabel}</span>
				<ChevronsUpDown class="ml-2 size-4 shrink-0 opacity-50" />
			</Button>
		</Popover.Trigger>
		<Popover.Content class="w-(--bits-popover-anchor-width) p-0" align="start">
			<Command.Root>
				<Command.Input placeholder={m.roles_scope_search()} />
				<Command.List>
					<Command.Empty>{m.roles_scope_no_group()}</Command.Empty>
					<Command.Item value={allOptionLabel} onSelect={() => select('')}>
						<Check class={cn('mr-2 size-4', scopeId === '' ? 'opacity-100' : 'opacity-0')} />
						{allOptionLabel}
					</Command.Item>
					{#each groups as g (g.id)}
						{@const ok = g.kind === activeKind}
						<Command.Item value={g.name} disabled={!ok} onSelect={() => ok && select(g.id)}>
							<Check class={cn('mr-2 size-4', scopeId === g.id ? 'opacity-100' : 'opacity-0')} />
							{#if g.kind === 'device'}
								<Server class="text-faint mr-1.5 size-3.5" />
							{:else}
								<Users class="text-faint mr-1.5 size-3.5" />
							{/if}
							<span class="truncate">{g.name}</span>
							{#if !ok}
								<span class="text-muted-foreground ml-auto pl-2 text-xs whitespace-nowrap">
									{disabledNote}
								</span>
							{/if}
						</Command.Item>
					{/each}
				</Command.List>
			</Command.Root>
		</Popover.Content>
	</Popover.Root>
	<p id="role-scope-hint" class="text-muted-foreground text-xs">{hint}</p>
</div>
