<script lang="ts">
	import PackageParamsForm from './PackageParamsForm.svelte';
	import type { PackageFormState } from './types';

	let { params = $bindable() }: { params: PackageFormState } = $props();
</script>

<PackageParamsForm bind:params />
