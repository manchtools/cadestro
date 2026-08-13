<script lang="ts">
	// Per-FormKey dispatch component. Both action-create-form.svelte and
	// edit-params-dialog.svelte previously inlined the 19-arm ladder
	// `{#if actionType === 'PACKAGE'}<PackageParamsForm/>{:else if …}…{/if}`.
	// This single component owns the ladder; the orchestrators just render
	// <ActionParamsFormDispatch formKey={…} bind:params={paramsByKey[…]} … />.
	//
	// `params` is the bindable form-state object for the selected FormKey.
	// `errors` / `onclearerror` are forwarded to whichever per-type
	// ParamsForm component lives under this FormKey.
	import {
		PackageParamsForm,
		ShellParamsForm,
		ServiceParamsForm,
		FileParamsForm,
		AppParamsForm,
		FlatpakParamsForm,
		UpdateParamsForm,
		RepositoryParamsForm,
		DirectoryParamsForm,
		UserParamsForm,
		SshParamsForm,
		SshdParamsForm,
		AdminPolicyParamsForm,
		LpsParamsForm,
		EncryptionParamsForm,
		GroupParamsForm,
		WifiParamsForm,
		AgentUpdateParamsForm
	} from '../forms';
	import type { FormKey } from '../registry';

	interface Props {
		formKey: FormKey;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		params: any;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { formKey, params = $bindable(), errors, onclearerror }: Props = $props();
</script>

{#if formKey === 'PACKAGE'}
	<PackageParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'SHELL'}
	<ShellParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'COMPLIANCE_CHECK'}
	<ShellParamsForm bind:params complianceOnly={true} {errors} {onclearerror} />
{:else if formKey === 'SERVICE'}
	<ServiceParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'FILE'}
	<FileParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'APP'}
	<AppParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'FLATPAK'}
	<FlatpakParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'UPDATE'}
	<UpdateParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'REPOSITORY'}
	<RepositoryParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'DIRECTORY'}
	<DirectoryParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'USER'}
	<UserParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'SSH'}
	<SshParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'SSHD'}
	<SshdParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'ADMIN_POLICY'}
	<AdminPolicyParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'LPS'}
	<LpsParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'ENCRYPTION'}
	<EncryptionParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'GROUP'}
	<GroupParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'WIFI'}
	<WifiParamsForm bind:params {errors} {onclearerror} />
{:else if formKey === 'AGENT_UPDATE'}
	<AgentUpdateParamsForm bind:params {errors} {onclearerror} />
{/if}
