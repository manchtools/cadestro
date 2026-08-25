<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient, type User } from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	interface Props {
		open: boolean;
		user: User;
		onsave: (updated: User) => void;
	}

	let { open = $bindable(), user, onsave }: Props = $props();

	let displayName = $state('');
	let givenName = $state('');
	let familyName = $state('');
	let preferredUsername = $state('');
	let picture = $state('');
	let locale = $state('');
	let saving = $state(false);

	$effect(() => {
		if (open) {
			displayName = user.displayName;
			givenName = user.givenName;
			familyName = user.familyName;
			preferredUsername = user.preferredUsername;
			picture = user.picture;
			locale = user.locale;
		}
	});

	async function save() {
		saving = true;
		try {
			const updated = await apiClient.updateUserProfile((user.id?.value ?? ''), {
				displayName,
				givenName,
				familyName,
				preferredUsername,
				picture,
				locale
			});
			if (updated) {
				toast.success(m.user_detail_profile_updated());
				open = false;
				onsave(updated);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.user_detail_edit_profile_title()}</Dialog.Title>
			<Dialog.Description>{m.user_detail_edit_profile_description()}</Dialog.Description>
		</Dialog.Header>
		<form onsubmit={(e) => { e.preventDefault(); save(); }} class="space-y-4">
			<div class="space-y-2">
				<Label for="displayName">{m.users_display_name()}</Label>
				<Input id="displayName" bind:value={displayName} />
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div class="space-y-2">
					<Label for="givenName">{m.users_given_name()}</Label>
					<Input id="givenName" bind:value={givenName} />
				</div>
				<div class="space-y-2">
					<Label for="familyName">{m.users_family_name()}</Label>
					<Input id="familyName" bind:value={familyName} />
				</div>
			</div>
			<div class="space-y-2">
				<Label for="preferredUsername">{m.users_preferred_username()}</Label>
				<Input id="preferredUsername" bind:value={preferredUsername} />
			</div>
			<div class="space-y-2">
				<Label for="picture">{m.users_picture()}</Label>
				<Input id="picture" type="url" bind:value={picture} />
			</div>
			<div class="space-y-2">
				<Label for="locale">{m.users_locale()}</Label>
				<Input id="locale" bind:value={locale} placeholder="en, de, fr, ..." />
			</div>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (open = false)}>
					{m.common_cancel()}
				</Button>
				<Button type="submit" disabled={saving}>
					{saving ? m.common_saving() : m.common_save()}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
