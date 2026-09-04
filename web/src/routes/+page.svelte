<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { Permission, type User } from '$contract/cadestro/v1/control_pb';
	import Access from '$lib/components/console/Access.svelte';
	import Actions from '$lib/components/console/Actions.svelte';
	import Assignments from '$lib/components/console/Assignments.svelte';
	import Audit from '$lib/components/console/Audit.svelte';
	import Devices from '$lib/components/console/Devices.svelte';
	import Enrollment from '$lib/components/console/Enrollment.svelte';
	import Groups from '$lib/components/console/Groups.svelte';
	import IdentityProviders from '$lib/components/console/IdentityProviders.svelte';
	import { api, errorMessage, logout } from '$lib/api';
	import { readSession } from '$lib/session';

	type Section = 'devices' | 'enrollment' | 'actions' | 'groups' | 'assignments' | 'access' | 'identity-providers' | 'audit';

	let currentUser = $state<User>();
	let loading = $state(true);
	let error = $state('');
	let refreshKey = $state(0);

	function can(permission: Permission): boolean {
		return currentUser?.permissions.includes(permission) ?? false;
	}

	function canAny(...permissions: Permission[]): boolean {
		return permissions.some(can);
	}

	const sections = $derived([
		{ id: 'devices' as const, label: 'Devices', visible: can(Permission.LIST_DEVICES) },
		{ id: 'enrollment' as const, label: 'Enrollment', visible: canAny(Permission.CREATE_TOKEN, Permission.LIST_TOKENS) },
		{ id: 'actions' as const, label: 'Actions', visible: canAny(Permission.CREATE_ACTION, Permission.LIST_ACTIONS) },
		{ id: 'groups' as const, label: 'Groups', visible: canAny(Permission.CREATE_DEVICE_GROUP, Permission.LIST_DEVICE_GROUPS) },
		{ id: 'assignments' as const, label: 'Assignments', visible: canAny(Permission.CREATE_ASSIGNMENT, Permission.LIST_ASSIGNMENTS) },
		{ id: 'access' as const, label: 'Access', visible: canAny(Permission.LIST_USERS, Permission.LIST_ROLES, Permission.LIST_PERMISSIONS, Permission.CREATE_ROLE, Permission.UPDATE_ROLE, Permission.DELETE_ROLE, Permission.ASSIGN_ROLE_TO_USER, Permission.REVOKE_ROLE_FROM_USER, Permission.REVOKE_USER_SESSIONS) },
		{ id: 'identity-providers' as const, label: 'Identity providers', visible: canAny(Permission.CREATE_IDENTITY_PROVIDER, Permission.LIST_IDENTITY_PROVIDERS) },
		{ id: 'audit' as const, label: 'Audit', visible: can(Permission.LIST_AUDIT_EVENTS) }
	].filter((section) => section.visible));
	const requestedSection = $derived(page.url.searchParams.get('section') as Section | null);
	const activeSection = $derived(sections.some((section) => section.id === requestedSection) ? requestedSection : sections[0]?.id);

	function sectionHref(section: Section): string {
		const target = new URL(page.url);
		target.search = '';
		target.searchParams.set('section', section);
		return `${target.pathname}${target.search}`;
	}

	async function loadCurrentUser() {
		loading = true;
		error = '';
		if (!readSession()) {
			await goto('/login');
			return;
		}
		try {
			currentUser = (await api.getCurrentUser({})).user;
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	onMount(loadCurrentUser);

	async function signOut() {
		try {
			await logout();
		} finally {
			await goto('/login');
		}
	}
</script>

<header>
	<div><strong>Cadestro</strong><span>Linux management core</span></div>
	<nav aria-label="Session actions">
		{#if currentUser}<span>{currentUser.displayName || currentUser.email}</span>{/if}
		<button type="button" class="quiet" onclick={() => refreshKey += 1} disabled={loading}>Refresh section</button>
		<button type="button" class="quiet" onclick={signOut}>Sign out</button>
	</nav>
</header>

<main class="console">
	{#if loading}
		<section class="card" aria-busy="true"><p role="status">Loading your account…</p></section>
	{:else if error}
		<section class="card"><p class="error banner" role="alert">{error}</p><button type="button" onclick={loadCurrentUser}>Retry</button></section>
	{:else if currentUser}
		<nav class="section-nav" aria-label="Administration sections">
			{#each sections as section}
				<a href={sectionHref(section.id)} aria-current={activeSection === section.id ? 'page' : undefined}>{section.label}</a>
			{/each}
		</nav>
		<div class="stack">
			{#key `${page.url.search}:${refreshKey}`}
				{#if activeSection === 'devices'}
					<Devices {can} />
				{:else if activeSection === 'enrollment'}
					<Enrollment {can} />
				{:else if activeSection === 'actions'}
					<Actions {can} />
				{:else if activeSection === 'groups'}
					<Groups {can} />
				{:else if activeSection === 'assignments'}
					<Assignments {can} />
				{:else if activeSection === 'access'}
					<Access {can} {currentUser} />
				{:else if activeSection === 'identity-providers'}
					<IdentityProviders {can} />
				{:else if activeSection === 'audit'}
					<Audit />
				{:else}
					<section class="card"><p>Your role has no console sections.</p></section>
				{/if}
			{/key}
		</div>
	{/if}
</main>
