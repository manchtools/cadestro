<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { shell, enterContext, exitContext } from '$lib/shell/shell.svelte';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		authStore,
		type User,
		type Role,
		type UserGroup,
		type RoleGrant,
		RoleGrantScopeKind,
		formatTimestamp
	} from '$lib/sdk';
	import RoleAssignDialog from '$lib/components/role-assign-dialog.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Chip } from '$lib/components/fleet';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Switch } from '$lib/components/ui/switch';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Dialog from '$lib/components/ui/dialog';
	import {
		ArrowLeft,
		RefreshCw,
		Pencil,
		Shield,
		Ban,
		Check,
		UserPlus,
		UsersRound,
		Unlink,
		Globe,
		Key,
		Terminal,
		Plus,
		X
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import EditProfileDialog from './edit-profile-dialog.svelte';
	import { getLocalizedError } from '$lib/errors';

	let user = $state<User | null>(null);
	let userGroups = $state<UserGroup[]>([]);
	let loading = $state(true);
	let activeTab = $state('profile');
	let editProfileOpen = $state(false);
	let eraseDialogOpen = $state(false);
	let roleDialogOpen = $state(false);
	let unlinkConfirmOpen = $state(false);
	let unlinkingLinkId = $state('');
	let editUsernameOpen = $state(false);
	let editingUsername = $state('');
	let addSshKeyOpen = $state(false);
	let newSshKeyValue = $state('');
	let newSshKeyComment = $state('');
	let removeSshKeyConfirmOpen = $state(false);
	let removingSshKeyId = $state('');

	const userId = $derived(page.params.id ?? '');

	const displayName = $derived.by(() => {
		if (!user) return '';
		if (user.displayName) return user.displayName;
		if (user.givenName || user.familyName) return `${user.givenName} ${user.familyName}`.trim();
		return user.email;
	});

	// Header avatar fallback: initials from whatever identity the user actually
	// has — no generated art, no placeholder image.
	const initials = $derived.by(() => {
		const source = displayName || user?.email || '';
		const words = source.split(/[\s._@-]+/).filter(Boolean);
		const letters = words.slice(0, 2).map((w) => w[0]);
		return (letters.join('') || source.slice(0, 2)).toUpperCase();
	});

	const isSelf = $derived(!!user && user.id === authStore.user?.id);

	// The user's OWN action. Nothing on this page holds an editable draft — every
	// field commits from its own dialog — so the context is permanently clean and
	// exists solely as this user's action bar, the same shape the action detail
	// page uses for action types with no params form. Erase is offered only for
	// somebody else; a clean context shows no Stash/Cancel and a disabled commit,
	// so it reads as the one thing on offer, in crit with a trash glyph.
	const contextId = $derived(`user:${userId}`);
	$effect(() => {
		const u = user;
		const self = isSelf;
		if (!u) return;
		untrack(() =>
			enterContext({
				id: contextId,
				title: displayName || u.email,
				dirty: false,
				valid: true,
				commitLabel: m.common_save(),
				onCommit: () => {},
				extraActions: self
					? []
					: [
							{
								id: 'erase',
								label: m.users_erase_action(),
								tone: 'danger',
								onRun: () => (eraseDialogOpen = true)
							}
						]
			})
		);
	});
	onDestroy(() => {
		if (shell.pill.context?.id === contextId) exitContext();
	});

	// Role ids hidden from the assign picker: held GLOBALLY (unscoped) or
	// inherited. A role held only at device-group scopes stays selectable so a
	// second scope can be added (#7); roleGrants carries each grant's scope.
	const roleAssignExcludeIds = $derived.by(() => {
		if (!user) return [];
		const unscoped = (user.roleGrants ?? [])
			.filter((g) => g.scopeKind === RoleGrantScopeKind.UNSPECIFIED && g.role)
			.map((g) => g.role!.id);
		return [...unscoped, ...inheritedRoles.map((ir) => ir.role.id)];
	});

	// Roles inherited via user group membership (not directly assigned)
	const inheritedRoles = $derived.by(() => {
		if (!user) return [];
		const directIds = new Set(user.roleGrants.flatMap((grant) => (grant.role ? [grant.role.id] : [])));
		const result: { role: Role; groupName: string }[] = [];
		const seen = new Set<string>();
		for (const group of userGroups) {
			for (const grant of group.roleGrants) {
				if (grant.role && !directIds.has(grant.role.id) && !seen.has(grant.role.id)) {
					seen.add(grant.role.id);
					result.push({ role: grant.role, groupName: group.name });
				}
			}
		}
		return result;
	});

	const isAdmin = $derived(
		(user?.roleGrants.some((grant) => grant.role?.name === 'Admin') ?? false) ||
			inheritedRoles.some((ir) => ir.role.name === 'Admin')
	);

	const roleCount = $derived((user?.roleGrants.length ?? 0) + inheritedRoles.length);

	onMount(() => {
		if (userId) {
			loadUser();
		}
	});

	async function loadUser() {
		if (!userId) return;
		loading = true;
		try {
			user = (await apiClient.getUser(userId)) ?? null;
			try {
				const groupsResponse = await apiClient.listUserGroupsForUser(userId);
				userGroups = groupsResponse.groups;
			} catch (error) {
				console.error('Failed to load user groups', error);
				userGroups = [];
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	function handleProfileUpdate(updated: User) {
		user = { ...updated, identityLinks: user?.identityLinks ?? [] };
	}

	async function toggleDisabled() {
		if (!user) return;
		try {
			const updated = await apiClient.setUserDisabled(user.id, !user.disabled);
			if (updated) {
				user = { ...updated, identityLinks: user.identityLinks };
				toast.success(updated.disabled ? m.users_disabled() : m.users_enabled());
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	// The server refuses SCIM-managed subjects with `scim_managed_resource`; the
	// page stays put because only a resolved erase navigates away.
	async function eraseUser() {
		if (!user) return;
		try {
			await apiClient.eraseJITUser(user.id);
			toast.success(m.users_erased());
			goto('/users');
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			eraseDialogOpen = false;
		}
	}

	async function revokeRoleGrant(grant: RoleGrant) {
		if (!user || !grant.role) return;
		try {
			await apiClient.revokeRoleFromUser(user.id, grant.role.id, grant.scopeKind, grant.scopeId);
			toast.success(m.roles_revoked());
			await loadUser();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function unlinkIdentity() {
		if (!unlinkingLinkId) return;
		try {
			await apiClient.unlinkIdentity(unlinkingLinkId);
			unlinkConfirmOpen = false;
			unlinkingLinkId = '';
			toast.success(m.users_identity_unlinked());
			await loadUser();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function updateLinuxUsername() {
		if (!user || !editingUsername.trim()) return;
		try {
			const updated = await apiClient.updateUserLinuxUsername(user.id, editingUsername.trim());
			if (updated) {
				user = { ...updated, identityLinks: user.identityLinks };
				toast.success(m.user_detail_linux_username_updated());
			}
			editUsernameOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function toggleSshSetting(
		field: 'sshAccessEnabled' | 'sshAllowPubkey' | 'sshAllowPassword',
		value: boolean
	) {
		if (!user) return;
		const settings = {
			sshAccessEnabled: user.sshAccessEnabled,
			sshAllowPubkey: user.sshAllowPubkey,
			sshAllowPassword: user.sshAllowPassword,
			[field]: value
		};
		try {
			const updated = await apiClient.updateUserSshSettings(user.id, settings);
			if (updated) {
				user = { ...updated, identityLinks: user.identityLinks };
				toast.success(m.user_detail_ssh_settings_updated());
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function addSshKey() {
		if (!user || !newSshKeyValue.trim()) return;
		try {
			await apiClient.addUserSshKey(user.id, newSshKeyValue.trim(), newSshKeyComment.trim());
			toast.success(m.user_detail_ssh_key_added());
			addSshKeyOpen = false;
			newSshKeyValue = '';
			newSshKeyComment = '';
			await loadUser();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function removeSshKey() {
		if (!user || !removingSshKeyId) return;
		try {
			await apiClient.removeUserSshKey(user.id, removingSshKeyId);
			toast.success(m.user_detail_ssh_key_removed());
			removeSshKeyConfirmOpen = false;
			removingSshKeyId = '';
			await loadUser();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}
</script>

{#snippet field(label: string, value: string, mono = false)}
	<span class="text-xs text-muted-foreground">{label}</span>
	<span class="text-sm {mono ? 'font-mono' : ''}">{value}</span>
{/snippet}

{#snippet sectionLabel(text: string)}
	<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{text}</span>
{/snippet}

<div class="min-w-0 flex-1 overflow-x-hidden overflow-y-auto p-4 md:p-6">
	<div class="min-w-0 space-y-4">
		<div class="flex items-center gap-2">
			<Button
				variant="ghost"
				size="icon"
				aria-label={m.common_back()}
				onclick={() => history.back()}
			>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<Button variant="outline" size="sm" class="ml-auto" onclick={loadUser} disabled={loading}>
				<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
					<RefreshCw class="h-4 w-4" />
				</span>
				{m.common_refresh()}
			</Button>
		</div>

		{#if loading && !user}
			<div class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate">
				<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if user}
			<!-- entity header card -->
			<section
				data-tour="user-header"
				data-testid="user-header"
				class="flex flex-wrap items-center gap-4 rounded-xl border border-hair bg-surface p-4 shadow-plate"
			>
				{#if user.picture}
					<img
						src={user.picture}
						alt={displayName}
						class="h-14 w-14 shrink-0 rounded-full object-cover"
					/>
				{:else}
					<span
						aria-hidden="true"
						class="grid h-14 w-14 shrink-0 place-items-center rounded-full bg-accent-soft text-lg font-semibold text-accent-ink"
					>
						{initials}
					</span>
				{/if}
				<div class="min-w-0 flex-1">
					<div class="flex flex-wrap items-center gap-2">
						<h1 class="truncate text-2xl font-bold">{displayName}</h1>
						{#if isAdmin}
							<Chip tone="info" label={m.users_role_admin()} />
						{/if}
						{#if user.disabled}
							<Chip tone="crit" label={m.common_disabled()} />
						{:else}
							<Chip tone="ok" label={m.common_active()} />
						{/if}
						{#if user.identityLinks?.length}
							<span title={user.identityLinks.map((l) => l.providerName).join(', ')}>
								<Chip tone="idle" label={m.users_sso_chip()} />
							</span>
						{/if}
						{#if isSelf}
							<Chip tone="idle" label={m.users_you()} />
						{/if}
					</div>
					<p class="truncate text-sm text-muted-foreground">{user.email}</p>
					<p class="truncate font-mono text-[0.68rem] text-faint" title={m.users_header_id()}>
						{user.id}
					</p>
				</div>
				<div class="flex flex-wrap gap-2">
					<Button variant="outline" size="sm" onclick={() => (editProfileOpen = true)}>
						<Pencil class="mr-2 h-4 w-4" />
						{m.user_detail_edit_profile()}
					</Button>
					{#if !isSelf}
						<Button variant="outline" size="sm" onclick={toggleDisabled}>
							{#if user.disabled}
								<Check class="mr-2 h-4 w-4" />
								{m.common_enable()}
							{:else}
								<Ban class="mr-2 h-4 w-4" />
								{m.common_disable()}
							{/if}
						</Button>
					{/if}
				</div>
			</section>

			<Tabs.Root bind:value={activeTab} class="min-w-0">
				<Tabs.List>
					<Tabs.Trigger value="profile">{m.users_tab_profile()}</Tabs.Trigger>
					<Tabs.Trigger value="identity-links">
						{m.users_tab_identity_links({ count: user.identityLinks?.length ?? 0 })}
					</Tabs.Trigger>
					<Tabs.Trigger value="ssh-keys">
						{m.users_tab_ssh_keys({ count: user.sshPublicKeys?.length ?? 0 })}
					</Tabs.Trigger>
					<Tabs.Trigger value="roles">{m.users_tab_roles({ count: roleCount })}</Tabs.Trigger>
					<Tabs.Trigger value="groups">
						{m.users_tab_groups({ count: userGroups.length })}
					</Tabs.Trigger>
				</Tabs.List>

				<!-- ── Profile ───────────────────────────────────────────────── -->
				<Tabs.Content value="profile" class="space-y-4">
					<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
						{@render sectionLabel(m.user_detail_profile())}
						<div class="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2">
							{@render field(m.users_table_email(), user.email)}
							{#if user.displayName}{@render field(m.users_display_name(), user.displayName)}{/if}
							{#if user.givenName}{@render field(m.users_given_name(), user.givenName)}{/if}
							{#if user.familyName}{@render field(m.users_family_name(), user.familyName)}{/if}
							{#if user.preferredUsername}
								{@render field(m.users_preferred_username(), user.preferredUsername, true)}
							{/if}
							{#if user.locale}{@render field(m.users_locale(), user.locale, true)}{/if}
							{@render field(m.users_table_created(), formatTimestamp(user.createdAt), true)}
							{@render field(m.users_table_last_login(), formatTimestamp(user.lastLoginAt), true)}
						</div>
					</section>

					<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
						<div class="flex items-center justify-between gap-3">
							<span class="flex items-center gap-2">
								<Terminal class="h-4 w-4 text-faint" />
								{@render sectionLabel(m.user_detail_linux_identity())}
							</span>
							{#if authStore.hasPermission('UpdateUserLinuxUsername')}
								<Button
									variant="outline"
									size="sm"
									onclick={() => {
										editingUsername = user?.linuxUsername ?? '';
										editUsernameOpen = true;
									}}
								>
									<Pencil class="mr-2 h-4 w-4" />
									{m.user_detail_edit_linux_username()}
								</Button>
							{/if}
						</div>
						<div class="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2">
							{@render field(m.user_detail_linux_username(), user.linuxUsername || '—', true)}
							{@render field(m.user_detail_linux_uid(), String(user.linuxUid || '—'), true)}
						</div>
						{#if authStore.hasPermission('SetUserProvisioningEnabled')}
							<div class="mt-4 flex items-center justify-between gap-4 border-t border-hair pt-4">
								<div>
									<Label>{m.user_detail_provisioning_enabled()}</Label>
									<p class="text-xs text-muted-foreground">
										{m.user_detail_provisioning_enabled_description()}
									</p>
								</div>
								<Switch
									checked={user.userProvisioningEnabled}
									onCheckedChange={async (v) => {
										try {
											const res = await apiClient.setUserProvisioningEnabled(user!.id, v);
											if (res.user) user = { ...res.user, identityLinks: user!.identityLinks };
										} catch (error) {
											toast.error(getLocalizedError(error));
										}
									}}
								/>
							</div>
						{/if}
					</section>
				</Tabs.Content>

				<!-- ── Identity links ────────────────────────────────────────── -->
				<Tabs.Content value="identity-links">
					<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
						{@render sectionLabel(m.user_detail_identity_links())}
						{#if !user.identityLinks || user.identityLinks.length === 0}
							<p class="mt-3 text-sm text-muted-foreground">{m.user_detail_no_identity_links()}</p>
						{:else}
							<div class="mt-3 divide-y divide-hair">
								{#each user.identityLinks as link (link.id)}
									<div class="flex items-center justify-between gap-3 py-2.5">
										<div class="min-w-0">
											<p class="flex items-center gap-2 text-sm font-medium">
												<Globe class="h-4 w-4 text-faint" />
												{link.providerName}
												<span class="font-mono text-[0.68rem] text-faint">{link.providerSlug}</span>
											</p>
											<p class="truncate font-mono text-xs text-muted-foreground">
												{link.externalEmail}
											</p>
										</div>
										<Button
											variant="ghost"
											size="sm"
											onclick={() => {
												unlinkingLinkId = link.id;
												unlinkConfirmOpen = true;
											}}
										>
											<Unlink class="mr-2 h-4 w-4" />
											{m.common_delete()}
										</Button>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</Tabs.Content>

				<!-- ── SSH keys ──────────────────────────────────────────────── -->
				<Tabs.Content value="ssh-keys" class="space-y-4">
					<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
						<span class="flex items-center gap-2">
							<Key class="h-4 w-4 text-faint" />
							{@render sectionLabel(m.user_detail_ssh_access())}
						</span>
						<div class="mt-3 space-y-3">
							<div class="flex items-center justify-between gap-4">
								<Label for="ssh-enabled" class="text-sm">{m.user_detail_ssh_access_enabled()}</Label>
								<Switch
									id="ssh-enabled"
									checked={user.sshAccessEnabled}
									onCheckedChange={(v) => toggleSshSetting('sshAccessEnabled', v)}
								/>
							</div>
							<div class="flex items-center justify-between gap-4">
								<Label for="ssh-pubkey" class="text-sm">{m.user_detail_ssh_allow_pubkey()}</Label>
								<Switch
									id="ssh-pubkey"
									checked={user.sshAllowPubkey}
									onCheckedChange={(v) => toggleSshSetting('sshAllowPubkey', v)}
								/>
							</div>
							<div class="flex items-center justify-between gap-4">
								<Label for="ssh-password" class="text-sm">{m.user_detail_ssh_allow_password()}</Label>
								<Switch
									id="ssh-password"
									checked={user.sshAllowPassword}
									onCheckedChange={(v) => toggleSshSetting('sshAllowPassword', v)}
								/>
							</div>
						</div>
					</section>

					<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
						<div class="flex items-center justify-between gap-3">
							<span class="flex items-center gap-2">
								<Key class="h-4 w-4 text-faint" />
								{@render sectionLabel(m.user_detail_ssh_keys())}
							</span>
							<Button
								variant="outline"
								size="sm"
								onclick={() => {
									newSshKeyValue = '';
									newSshKeyComment = '';
									addSshKeyOpen = true;
								}}
							>
								<Plus class="mr-2 h-4 w-4" />
								{m.user_detail_ssh_add_key()}
							</Button>
						</div>
						{#if !user.sshPublicKeys || user.sshPublicKeys.length === 0}
							<p class="mt-3 text-sm text-muted-foreground">{m.user_detail_ssh_no_keys()}</p>
						{:else}
							<div class="mt-3 divide-y divide-hair">
								{#each user.sshPublicKeys as key (key.id)}
									<div class="flex items-center justify-between gap-3 py-2.5">
										<div class="min-w-0 flex-1">
											<p class="truncate rounded-md bg-sunken px-2 py-1 font-mono text-xs">
												{key.publicKey}
											</p>
											{#if key.comment}
												<p class="mt-1 text-xs text-muted-foreground">{key.comment}</p>
											{/if}
										</div>
										<Button
											variant="ghost"
											size="icon"
											aria-label={m.common_delete()}
											onclick={() => {
												removingSshKeyId = key.id;
												removeSshKeyConfirmOpen = true;
											}}
										>
											<X class="h-4 w-4" />
										</Button>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</Tabs.Content>

				<!-- ── Roles ─────────────────────────────────────────────────── -->
				<Tabs.Content value="roles">
					<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
						<div class="flex items-center justify-between gap-3">
							{@render sectionLabel(m.user_detail_roles())}
							<Button variant="outline" size="sm" onclick={() => (roleDialogOpen = true)}>
								<UserPlus class="mr-2 h-4 w-4" />
								{m.roles_assign_to_user()}
							</Button>
						</div>
						{#if (user.roleGrants?.length ?? 0) === 0 && inheritedRoles.length === 0}
							<p class="mt-3 text-sm text-muted-foreground">—</p>
						{:else}
							<div class="mt-3 divide-y divide-hair">
								{#each user.roleGrants ?? [] as grant}
									{#if grant.role}
										<div class="flex items-center justify-between gap-3 py-2.5">
											<div class="min-w-0">
												<div class="flex flex-wrap items-center gap-2">
													<a
														href="{base}/roles/{grant.role.id}"
														class="text-sm font-medium hover:underline"
													>
														{grant.role.name}
													</a>
													{#if grant.scopeKind === RoleGrantScopeKind.DEVICE_GROUP}
														<Chip tone="idle">
															<Terminal class="h-3 w-3" />{grant.scopeName || grant.scopeId}
														</Chip>
													{:else if grant.scopeKind === RoleGrantScopeKind.USER_GROUP}
														<Chip tone="idle">
															<UsersRound class="h-3 w-3" />{grant.scopeName || grant.scopeId}
														</Chip>
													{:else}
														<Chip tone="idle" label={m.roles_scope_org_wide()} />
													{/if}
												</div>
												{#if grant.role.description}
													<p class="text-xs text-muted-foreground">{grant.role.description}</p>
												{/if}
											</div>
											{#if !isSelf && (!grant.role.isSystem || grant.role.name !== 'User')}
												<Button variant="ghost" size="sm" onclick={() => revokeRoleGrant(grant)}>
													<Shield class="mr-2 h-4 w-4" />
													{m.roles_revoke_from_user()}
												</Button>
											{/if}
										</div>
									{/if}
								{/each}
								{#each inheritedRoles as { role, groupName } (role.id)}
									<div class="flex items-center justify-between gap-3 py-2.5">
										<div class="min-w-0">
											<div class="flex flex-wrap items-center gap-2">
												<a href="{base}/roles/{role.id}" class="text-sm font-medium hover:underline">
													{role.name}
												</a>
												<Chip tone="info">
													<UsersRound class="h-3 w-3" />{groupName}
												</Chip>
											</div>
											{#if role.description}
												<p class="text-xs text-muted-foreground">{role.description}</p>
											{/if}
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</Tabs.Content>

				<!-- ── Groups ────────────────────────────────────────────────── -->
				<Tabs.Content value="groups">
					<!-- The tab trigger already names this section, so it carries no
					     second heading of its own. -->
					<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
						{#if userGroups.length === 0}
							<p class="text-sm text-muted-foreground">{m.users_groups_empty()}</p>
						{:else}
							<div class="divide-y divide-hair">
								{#each userGroups as group (group.id)}
									<div class="flex items-center justify-between gap-3 py-2.5">
										<div class="min-w-0">
											<div class="flex flex-wrap items-center gap-2">
												<a
													href="{base}/user-groups/{group.id}"
													class="text-sm font-medium hover:underline"
												>
													{group.name}
												</a>
												{#if group.isScimManaged}
													<Chip tone="info" label={m.users_groups_scim()} />
												{/if}
											</div>
											{#if group.description}
												<p class="truncate text-xs text-muted-foreground">{group.description}</p>
											{/if}
										</div>
										<span class="shrink-0 font-mono text-[0.68rem] text-faint">
											{m.users_groups_members({ count: group.memberCount })}
										</span>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</Tabs.Content>
			</Tabs.Root>
		{/if}
	</div>
</div>

{#if user}
	<EditProfileDialog bind:open={editProfileOpen} {user} onsave={handleProfileUpdate} />
{/if}

<AlertDialog.Root bind:open={eraseDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.users_erase_dialog_title()}</AlertDialog.Title>
			<AlertDialog.Description>
				{m.users_erase_dialog_description({ email: user?.email ?? '' })}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<p class="text-sm text-muted-foreground">{m.users_erase_dialog_scim_note()}</p>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={eraseUser} variant="destructive">
				{m.users_erase_action()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

{#if user}
	<RoleAssignDialog
		bind:open={roleDialogOpen}
		title={m.roles_assign_to_user()}
		subtitle={user.email}
		excludeRoleIds={roleAssignExcludeIds}
		assign={(roleIds, scopeKind, scopeId) =>
			apiClient.assignRoleToUser(user!.id, roleIds, scopeKind, scopeId)}
		onAssigned={loadUser}
	/>
{/if}

<Dialog.Root bind:open={editUsernameOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.user_detail_linux_username()}</Dialog.Title>
		</Dialog.Header>
		<form
			onsubmit={(e) => {
				e.preventDefault();
				updateLinuxUsername();
			}}
			class="space-y-4"
		>
			<div class="space-y-2">
				<Label for="linuxUsername">{m.user_detail_linux_username()}</Label>
				<Input id="linuxUsername" bind:value={editingUsername} maxlength={32} class="font-mono" />
			</div>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (editUsernameOpen = false)}>
					{m.common_cancel()}
				</Button>
				<Button type="submit" disabled={!editingUsername.trim()}>{m.common_save()}</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={addSshKeyOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.user_detail_ssh_add_key_title()}</Dialog.Title>
			<Dialog.Description>{m.user_detail_ssh_add_key_description()}</Dialog.Description>
		</Dialog.Header>
		<form
			onsubmit={(e) => {
				e.preventDefault();
				addSshKey();
			}}
			class="space-y-4"
		>
			<div class="space-y-2">
				<Label for="sshKey">{m.user_detail_ssh_public_key()}</Label>
				<Textarea id="sshKey" bind:value={newSshKeyValue} rows={3} class="font-mono text-sm" />
			</div>
			<div class="space-y-2">
				<Label for="sshComment">{m.user_detail_ssh_comment()}</Label>
				<Input id="sshComment" bind:value={newSshKeyComment} />
			</div>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (addSshKeyOpen = false)}>
					{m.common_cancel()}
				</Button>
				<Button type="submit" disabled={!newSshKeyValue.trim()}>
					{m.user_detail_ssh_add_key()}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<AlertDialog.Root bind:open={removeSshKeyConfirmOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.common_delete()}</AlertDialog.Title>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel
				onclick={() => {
					removeSshKeyConfirmOpen = false;
					removingSshKeyId = '';
				}}
			>
				{m.common_cancel()}
			</AlertDialog.Cancel>
			<AlertDialog.Action onclick={removeSshKey}>{m.common_delete()}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={unlinkConfirmOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.common_delete()}</AlertDialog.Title>
			<AlertDialog.Description>{m.users_identity_unlink_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel
				onclick={() => {
					unlinkConfirmOpen = false;
					unlinkingLinkId = '';
				}}
			>
				{m.common_cancel()}
			</AlertDialog.Cancel>
			<AlertDialog.Action onclick={unlinkIdentity}>{m.common_delete()}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
