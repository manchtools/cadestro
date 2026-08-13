<script lang="ts">
	// One secret column cell. The plaintext lives only in this component's
	// local state and only while the row is revealed: hiding drops it, so
	// re-revealing re-issues the audited reveal RPC instead of replaying a
	// cached value. Nothing here logs or stores the plaintext.
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';
	import { Button } from '$lib/components/ui/button';
	import { Eye, EyeOff, Copy } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		id: string;
		reveal: (id: string) => Promise<string>;
		revealLabel: string;
		hideLabel: string;
		copyLabel: string;
		copiedMessage: string;
	}

	let { id, reveal, revealLabel, hideLabel, copyLabel, copiedMessage }: Props = $props();

	let plaintext = $state<string | null>(null);
	let revealing = $state(false);

	async function toggle() {
		if (plaintext !== null) {
			plaintext = null;
			return;
		}
		revealing = true;
		try {
			plaintext = await reveal(id);
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			revealing = false;
		}
	}

	async function copy() {
		if (plaintext === null) return;
		try {
			await navigator.clipboard.writeText(plaintext);
			toast.success(copiedMessage);
		} catch {
			toast.error(m.common_copy_failed());
		}
	}
</script>

<div class="flex items-center gap-1" data-testid="secret-cell" data-entry-id={id}>
	<code class="text-xs bg-muted px-1.5 py-0.5 rounded">{plaintext ?? '••••••••••••'}</code>
	<Button
		variant="ghost"
		size="icon"
		class="h-6 w-6"
		onclick={toggle}
		disabled={revealing}
		aria-label={plaintext === null ? revealLabel : hideLabel}
		title={plaintext === null ? revealLabel : hideLabel}
	>
		{#if plaintext === null}<Eye class="h-3 w-3" />{:else}<EyeOff class="h-3 w-3" />{/if}
	</Button>
	<Button
		variant="ghost"
		size="icon"
		class="h-6 w-6"
		onclick={copy}
		disabled={plaintext === null}
		aria-label={copyLabel}
		title={copyLabel}
	>
		<Copy class="h-3 w-3" />
	</Button>
</div>
