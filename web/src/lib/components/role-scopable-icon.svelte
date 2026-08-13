<script lang="ts">
	import { Server, Users } from '@lucide/svelte';
	import { RoleGrantScopeKind } from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';

	// #7: a small marker shown next to a role that CAN be scoped at assignment.
	// Presence = scopable; the icon + tooltip convey the kind (device vs user
	// group). A non-scopable (org-wide-only) role renders nothing.
	let { scopeKind }: { scopeKind: RoleGrantScopeKind | null } = $props();
</script>

{#if scopeKind === RoleGrantScopeKind.DEVICE_GROUP}
	<span class="inline-flex" title={m.roles_scopable_device()} aria-label={m.roles_scopable_device()}>
		<Server class="text-faint size-3.5" aria-hidden="true" />
	</span>
{:else if scopeKind === RoleGrantScopeKind.USER_GROUP}
	<span class="inline-flex" title={m.roles_scopable_user()} aria-label={m.roles_scopable_user()}>
		<Users class="text-faint size-3.5" aria-hidden="true" />
	</span>
{/if}
