<script lang="ts">
	import { setConsoleContext } from '$lib/console-context.svelte';
	import { goto } from '$app/navigation';
 import { base } from '$app/paths';
 import * as m from '$lib/paraglide/messages';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { LogOut, Moon, RefreshCw, Sun } from '@lucide/svelte';
	import { toggleMode } from 'mode-watcher';
	import { Permission, type User } from '$contract/cadestro/v1/control_pb';
	import { api, errorMessage, logout } from '$lib/api';
	import MorphBar from '$lib/components/shell/morph-bar.svelte';
	import GlobalSearch from '$lib/components/workspace/global-search.svelte';
	import DeviceWindow from '$lib/components/workspace/device-window.svelte';
	import Panel from '$lib/components/shell/panel.svelte';
	import StageRail from '$lib/components/shell/stage-rail.svelte';
	import { readSession } from '$lib/session';
	import { filterGroups, filterNav, OVERFLOW_GROUPS, PRIMARY_SECTIONS } from '$lib/shell/nav';
	import { setShellBounds, setShellPath, shell } from '$lib/shell/shell.svelte';
	import * as Avatar from '$lib/components/ui/avatar';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';

	let { children } = $props();
	let user = $state<User>();
	let loading = $state(true);
	let error = $state('');

	function can(permission: Permission): boolean {
		return user?.permissions.includes(permission) ?? false;
	}

	const sections = $derived(filterNav(PRIMARY_SECTIONS, can));
	const overflow = $derived(filterGroups(OVERFLOW_GROUPS, can));
	const initials = $derived(user?.email.split('@')[0]?.slice(0, 2).toUpperCase() ?? '??');
	const stageActive = $derived(shell.panels.some(panel => panel.minimized) || shell.drafts.length > 0);
	const openPanels = $derived(shell.panels.filter((panel) => !panel.minimized));
	$effect(() => setShellPath(page.url.pathname));
	setConsoleContext({ can, currentUser: () => user });

	async function loadUser() {
		loading = true;
		error = '';
		if (!readSession()) {
			await goto('/login');
			return;
		}
		try {
			user = (await api.getCurrentUser({})).user;
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	async function signOut() {
		try {
			await logout();
		} finally {
			await goto('/login');
		}
	}

	onMount(() => {
		const onKey = (event: KeyboardEvent) => { if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); shell.paletteOpen = !shell.paletteOpen; } };
		window.addEventListener('keydown', onKey);
		const resize = () => setShellBounds(window.innerWidth, window.innerHeight);
		resize();
		window.addEventListener('resize', resize);
		void loadUser();
		return () => { window.removeEventListener('resize', resize); window.removeEventListener('keydown', onKey); };
	});
</script>

<div class="h-screen overflow-hidden bg-background text-foreground">

	<a href="{base}/" class="fixed left-4 top-4 z-40 flex items-center gap-2" aria-label={m.shell_home()}>
		<img src="{base}/icon-light.svg" alt="" width="22" height="22" class="h-[22px] w-[22px] dark:hidden" />
		<img src="{base}/icon-dark.svg" alt="" width="22" height="22" class="hidden h-[22px] w-[22px] dark:block" />
		<span class="hidden font-semibold lg:inline">{m.nav_app_name()}</span>
	</a>

	<div class="fixed right-3 top-3 z-50 flex items-center gap-1.5">
		<button type="button" aria-label={m.shell_toggle_theme()} onclick={toggleMode} class="rounded-full border bg-popover/95 p-2 shadow-pill backdrop-blur hover:bg-accent hover:text-accent-foreground">
			<Sun class="h-4 w-4 dark:hidden" />
			<Moon class="hidden h-4 w-4 dark:block" />
		</button>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button type="button" aria-label={m.shell_user_menu()} class="rounded-full border bg-popover/95 p-1 shadow-pill backdrop-blur hover:bg-accent" {...props}>
						<Avatar.Root class="h-6 w-6">
							<Avatar.Fallback class="text-xs">{initials}</Avatar.Fallback>
						</Avatar.Root>
					</button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content side="bottom" align="end" class="w-52">
				<div class="truncate px-2 py-1.5 text-xs text-muted-foreground">{user?.email ?? m.shell_user_fallback()}</div>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={signOut}>
					<LogOut class="mr-2 h-4 w-4" />
					{m.nav_sign_out()}
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>

 {#if loading}<main class="grid h-full place-items-center"><p role="status">Loading your workspace…</p></main>{:else if error}<main class="grid h-full place-items-center"><div><p role="alert">{error}</p><button type="button" onclick={loadUser}>Retry</button></div></main>{:else if user}
	<MorphBar pathname={page.url.pathname} hrefBase={base} {sections} {overflow}>
		{#snippet searchSurface()}
			<GlobalSearch bind:open={shell.paletteOpen} {sections} {overflow} />
		{/snippet}
	</MorphBar>

	<main
		style="padding-top: calc(0.75rem + var(--pill-block, 3.25rem) + 0.75rem)"
		class="flex h-full min-h-0 flex-col overflow-hidden px-2 pb-2 transition-[padding-right] duration-200 {stageActive ? 'md:pr-56' : ''}"
	>
		{@render children()}
	</main>

	<StageRail />

	{#each openPanels as p (p.id)}
		<Panel panel={p}>
			{#snippet content()}
				{#if p.kind === 'device'}
					<DeviceWindow deviceId={p.refId} />
				{/if}
			{/snippet}
		</Panel>
	{/each}

	{#if shell.drag.slot === 'left'}
		<div data-testid="snap-zone-left" class="pointer-events-none fixed bottom-6 left-3 top-24 z-20 w-[42%] rounded-xl border-2 border-dashed border-primary bg-primary/5"></div>
	{:else if shell.drag.slot === 'right'}
		<div data-testid="snap-zone-right" class="pointer-events-none fixed bottom-6 right-3 top-24 z-20 w-[38%] rounded-xl border-2 border-dashed border-primary bg-primary/5"></div>
	{:else if shell.drag.slot === 'corner'}
		<div data-testid="snap-zone-corner" class="pointer-events-none fixed bottom-6 right-3 z-20 h-56 w-96 rounded-xl border-2 border-dashed border-primary bg-primary/5"></div>
	{/if}

	<div class="sr-only" role="status" aria-live="polite" data-testid="wm-announcement">{shell.announcement}</div>


{/if}
</div>
