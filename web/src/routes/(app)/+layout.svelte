<script lang="ts">
	// The shell is the app layout, mounted once above the router
	// outlet: the morph pill, stage rail, windows, and terminal drawer all live
	// here and survive every navigation; section pages render
	// in the outlet. Auth/config gating stays in +layout.ts; this
	// component assumes an authenticated session.
	//
	// This route file owns authStore and passes the
	// permission-filtered nav tables to the chrome as plain props. Chrome
	// modules import no client — guard-enforced.
	import { onMount } from 'svelte';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { goto, afterNavigate } from '$app/navigation';
	import { toggleMode } from 'mode-watcher';
	import { Moon, Sun, SquareTerminal, Settings, LogOut } from '@lucide/svelte';
	import { authStore } from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import * as Avatar from '$lib/components/ui/avatar';
	import GlobalSearch from '$lib/components/global-search.svelte';
	import {
		shell,
		toggleTerminal,
		notifyNavigated,
		stagedByKind,
		setShellBounds,
		setShellPath
	} from '$lib/shell/shell.svelte';
	import { PRIMARY_SECTIONS, OVERFLOW_GROUPS, filterNav, filterGroups } from '$lib/shell/nav';
	import MorphBar from '$lib/components/shell/morph-bar.svelte';
	import StageRail from '$lib/components/shell/stage-rail.svelte';
	import PersistentTerminalDrawer from '$lib/components/terminal/persistent-terminal-drawer.svelte';
	import DeviceWindow from '$lib/components/devices/device-window.svelte';
	import Panel from '$lib/components/shell/panel.svelte';
	import OnboardingHost from '$lib/components/onboarding/onboarding-host.svelte';

	let { children } = $props();

	// Navigation is permission-gated; admins see everything.
	const has = (p: string) => authStore.isAdmin || authStore.hasPermission(p);
	const sections = $derived(filterNav(PRIMARY_SECTIONS, has));
	const overflow = $derived(filterGroups(OVERFLOW_GROUPS, has));

	// The app-relative path, base-path stripped — this is the form surfaces
	// declare as their context `route`, so the store can compare the two and tell
	// "my owner is still mounted" from "my owner is three pages behind me".
	const appPath = $derived(page.url.pathname.slice(base.length) || '/');
	$effect(() => setShellPath(appPath));

	const openPanels = $derived(shell.panels.filter((p) => !p.minimized));
	// Reserve a right gutter while the stage rail shows something, so section
	// content doesn't slide under it on typical laptop widths.
	const stageActive = $derived(
		stagedByKind().length > 0 ||
			shell.drafts.length > 0 ||
			(shell.terminal.sessions.length > 0 && !shell.terminal.open)
	);

	const userInitials = $derived(
		authStore.user?.email
			?.split('@')[0]
			?.slice(0, 2)
			?.toUpperCase() ?? '??'
	);

	async function handleLogout() {
		await authStore.logout();
		goto(`${base}/login`);
	}

	afterNavigate(() => notifyNavigated());

	onMount(() => {
		const onKey = (e: KeyboardEvent) => {
			if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
				e.preventDefault();
				shell.paletteOpen = !shell.paletteOpen;
			}
		};
		const onResize = () => setShellBounds(window.innerWidth, window.innerHeight);
		onResize();
		window.addEventListener('keydown', onKey);
		window.addEventListener('resize', onResize);
		return () => {
			window.removeEventListener('keydown', onKey);
			window.removeEventListener('resize', onResize);
		};
	});
</script>

<div class="h-screen overflow-hidden bg-background text-foreground">
	<!-- brand — passive mark, the pill is the nav -->
	<a href="{base}/" class="fixed left-4 top-4 z-40 flex items-center gap-2" aria-label={m.shell_home()}>
		<img src="{base}/icon-light.svg" alt="" width="22" height="22" class="h-[22px] w-[22px] dark:hidden" />
		<img src="{base}/icon-dark.svg" alt="" width="22" height="22" class="hidden h-[22px] w-[22px] dark:block" />
		<span class="hidden font-semibold lg:inline">{m.nav_app_name()}</span>
	</a>

	<!-- utility cluster — stable chrome, out of the morphing pill -->
	<div class="fixed right-3 top-3 z-50 flex items-center gap-1.5">
		<button type="button" aria-label={m.terminal_title()} onclick={toggleTerminal} disabled={shell.terminal.sessions.length === 0} class="relative rounded-full border bg-popover/95 p-2 shadow-pill backdrop-blur hover:bg-accent hover:text-accent-foreground disabled:cursor-not-allowed disabled:opacity-40" title={shell.terminal.sessions.length ? m.terminal_sessions_title() : m.shell_terminal_empty_hint()}>
			<SquareTerminal class="h-4 w-4" />
			{#if shell.terminal.sessions.length}<span class="absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full border-2 border-background bg-success"></span>{/if}
		</button>
		<button type="button" aria-label={m.shell_toggle_theme()} onclick={toggleMode} class="rounded-full border bg-popover/95 p-2 shadow-pill backdrop-blur hover:bg-accent hover:text-accent-foreground">
			<Sun class="h-4 w-4 dark:hidden" />
			<Moon class="hidden h-4 w-4 dark:block" />
		</button>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button type="button" aria-label={m.shell_user_menu()} class="rounded-full border bg-popover/95 p-1 shadow-pill backdrop-blur hover:bg-accent" {...props}>
						<Avatar.Root class="h-6 w-6">
							<Avatar.Fallback class="text-xs">{userInitials}</Avatar.Fallback>
						</Avatar.Root>
					</button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content side="bottom" align="end" class="w-52">
				<div class="truncate px-2 py-1.5 text-xs text-muted-foreground">{authStore.user?.email ?? m.shell_user_fallback()}</div>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={() => goto(`${base}/settings`)}>
					<Settings class="mr-2 h-4 w-4" />
					{m.nav_settings()}
				</DropdownMenu.Item>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={handleLogout}>
					<LogOut class="mr-2 h-4 w-4" />
					{m.nav_sign_out()}
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>

	<!-- ⌘K morphs the pill and the palette IS the morphed pill's body — one
	     search surface for the page's own list, the fleet, and this tab. The
	     layout owns the client, so the palette is passed in as a snippet; the
	     chrome itself stays API-free (guard-enforced). -->
	<MorphBar pathname={page.url.pathname} hrefBase={base} {sections} {overflow}>
		{#snippet searchSurface()}
			<GlobalSearch bind:open={shell.paletteOpen} {sections} {overflow} />
		{/snippet}
	</MorphBar>

	<!-- section outlet — ONLY this swaps on navigation. Old section pages
	     manage their own scroll inside a flex column, as they did under the
	     sidebar's Inset. -->
	<!-- The chrome is fixed, so the content reserves its MEASURED height rather
	     than a guess: `--pill-block` is published by MorphBar and grows when the
	     subtext caption appears or wraps to two lines — hard-coding the pill's
	     height is what left the caption sitting on top of the first card. The
	     fallback matches the bare pill, for the first frame and for SSR.

	     The reservation must FOLLOW the measurement instantly, never animate it:
	     --pill-block is republished by a ResizeObserver on every transient height
	     of the pill's own 220ms morph, and an animated padding restarts a 200ms
	     transition toward that moving target on every tick — the stepped
	     whole-page shudder reported as pill flicker. Only the stage rail's
	     padding-right (a one-shot discrete change) transitions. -->
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

	<!-- Snap zones render only while a drag is over their region. -->
	{#if shell.drag.slot === 'left'}
		<div data-testid="snap-zone-left" class="pointer-events-none fixed bottom-6 left-3 top-24 z-20 w-[42%] rounded-xl border-2 border-dashed border-primary bg-primary/5"></div>
	{:else if shell.drag.slot === 'right'}
		<div data-testid="snap-zone-right" class="pointer-events-none fixed bottom-6 right-3 top-24 z-20 w-[38%] rounded-xl border-2 border-dashed border-primary bg-primary/5"></div>
	{:else if shell.drag.slot === 'corner'}
		<div data-testid="snap-zone-corner" class="pointer-events-none fixed bottom-6 right-3 z-20 h-56 w-96 rounded-xl border-2 border-dashed border-primary bg-primary/5"></div>
	{/if}

	<!-- Auto-stash announcements are polite and text-only. -->
	<div class="sr-only" role="status" aria-live="polite" data-testid="wm-announcement">{shell.announcement}</div>

	<PersistentTerminalDrawer />

	<OnboardingHost />
</div>
