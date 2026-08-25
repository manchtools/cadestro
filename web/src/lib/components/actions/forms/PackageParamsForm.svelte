<script lang="ts">
	import { untrack } from 'svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { PackageFormState } from './types';

	interface Props {
		params: PackageFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();

	let usePerManagerNames = $state(false);
	let seededParams: PackageFormState | null = null;

	function seedMode() {
		usePerManagerNames =
			!!params.aptName || !!params.dnfName || !!params.pacmanName || !!params.zypperName;
	}

	$effect(() => {
		if (params === seededParams) return;
		seededParams = params;
		untrack(seedMode);
	});

	function handleModeChange(enabled: boolean) {
		usePerManagerNames = enabled;
		if (enabled) {
			if (params.name) {
				params.aptName = params.aptName || params.name;
				params.dnfName = params.dnfName || params.name;
				params.pacmanName = params.pacmanName || params.name;
				params.zypperName = params.zypperName || params.name;
			}
			params.name = '';
		} else {
			params.name = params.aptName || params.dnfName || params.pacmanName || params.zypperName || '';
			params.aptName = '';
			params.dnfName = '';
			params.pacmanName = '';
			params.zypperName = '';
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="perManagerNames">{m.actions_params_per_manager()}</Label>
			<p class="text-xs text-muted-foreground">
				{m.actions_params_per_manager_description()}
			</p>
		</div>
		<Switch
			id="perManagerNames"
			checked={usePerManagerNames}
			onCheckedChange={handleModeChange}
		/>
	</div>

	{#if usePerManagerNames}
		<div class="space-y-3 rounded-md border p-3">
			<p class="text-xs text-muted-foreground">
				{m.actions_params_per_manager_hint()}
			</p>
			<div class="grid grid-cols-2 gap-3">
				<div class="space-y-1">
					<Label for="aptName" class="text-xs">{m.actions_params_package_apt()}</Label>
					<Input
						id="aptName"
						placeholder="e.g., firefox"
						bind:value={params.aptName}
						aria-invalid={!!errors?.aptName}
						oninput={() => onclearerror?.('aptName')}
					/>
					<FieldError error={errors?.aptName} />
				</div>
				<div class="space-y-1">
					<Label for="dnfName" class="text-xs">{m.actions_params_package_dnf()}</Label>
					<Input
						id="dnfName"
						placeholder="e.g., firefox"
						bind:value={params.dnfName}
						aria-invalid={!!errors?.dnfName}
						oninput={() => onclearerror?.('dnfName')}
					/>
					<FieldError error={errors?.dnfName} />
				</div>
				<div class="space-y-1">
					<Label for="pacmanName" class="text-xs">{m.actions_params_package_pacman()}</Label>
					<Input
						id="pacmanName"
						placeholder="e.g., firefox"
						bind:value={params.pacmanName}
						aria-invalid={!!errors?.pacmanName}
						oninput={() => onclearerror?.('pacmanName')}
					/>
					<FieldError error={errors?.pacmanName} />
				</div>
				<div class="space-y-1">
					<Label for="zypperName" class="text-xs">{m.actions_params_package_zypper()}</Label>
					<Input
						id="zypperName"
						placeholder="e.g., MozillaFirefox"
						bind:value={params.zypperName}
						aria-invalid={!!errors?.zypperName}
						oninput={() => onclearerror?.('zypperName')}
					/>
					<FieldError error={errors?.zypperName} />
				</div>
			</div>
		</div>
	{:else}
		<div class="space-y-1.5">
			<Label for="packageName">{m.actions_params_package_name()}</Label>
			<Input
				id="packageName"
				placeholder="e.g., firefox"
				bind:value={params.name}
				required
				aria-invalid={!!errors?.name}
				oninput={() => onclearerror?.('name')}
			/>
			<FieldError error={errors?.name} />
		</div>
	{/if}

	<div class="space-y-1.5 sm:max-w-64">
		<Label for="packageVersion">{m.actions_params_package_version()}</Label>
		<Input id="packageVersion" placeholder="e.g., 120.0" bind:value={params.version} />
	</div>
	<div class="flex items-center justify-between">
		<div class="space-y-0.5">
			<Label for="allowDowngrade">{m.actions_params_allow_downgrade()}</Label>
			<p class="text-xs text-muted-foreground">{m.actions_params_allow_downgrade_description()}</p>
		</div>
		<Switch id="allowDowngrade" bind:checked={params.allowDowngrade} />
	</div>
</div>
