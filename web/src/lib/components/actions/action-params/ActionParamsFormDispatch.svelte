<script lang="ts">

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
	import type {
		PackageFormState,
		ShellFormState,
		ServiceFormState,
		FileFormState,
		AppFormState,
		FlatpakFormState,
		UpdateFormState,
		RepositoryFormState,
		DirectoryFormState,
		UserFormState,
		SshFormState,
		SshdFormState,
		AdminPolicyFormState,
		LpsFormState,
		EncryptionFormState,
		GroupFormState,
		WifiFormState,
		AgentUpdateFormState,
		FormState
	} from '../forms/types';

	interface Props {
		formKey: FormKey;
		params: FormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { formKey, params = $bindable(), errors, onclearerror }: Props = $props();
</script>

{#if formKey === 'PACKAGE'}
	<PackageParamsForm bind:params={params as PackageFormState} {errors} {onclearerror} />
{:else if formKey === 'SHELL'}
	<ShellParamsForm bind:params={params as ShellFormState} {errors} {onclearerror} />
{:else if formKey === 'COMPLIANCE_CHECK'}
	<ShellParamsForm bind:params={params as ShellFormState} complianceOnly={true} {errors} {onclearerror} />
{:else if formKey === 'SERVICE'}
	<ServiceParamsForm bind:params={params as ServiceFormState} {errors} {onclearerror} />
{:else if formKey === 'FILE'}
	<FileParamsForm bind:params={params as FileFormState} {errors} {onclearerror} />
{:else if formKey === 'APP'}
	<AppParamsForm bind:params={params as AppFormState} {errors} {onclearerror} />
{:else if formKey === 'FLATPAK'}
	<FlatpakParamsForm bind:params={params as FlatpakFormState} {errors} {onclearerror} />
{:else if formKey === 'UPDATE'}
	<UpdateParamsForm bind:params={params as UpdateFormState} {errors} {onclearerror} />
{:else if formKey === 'REPOSITORY'}
	<RepositoryParamsForm bind:params={params as RepositoryFormState} {errors} {onclearerror} />
{:else if formKey === 'DIRECTORY'}
	<DirectoryParamsForm bind:params={params as DirectoryFormState} {errors} {onclearerror} />
{:else if formKey === 'USER'}
	<UserParamsForm bind:params={params as UserFormState} {errors} {onclearerror} />
{:else if formKey === 'SSH'}
	<SshParamsForm bind:params={params as SshFormState} {errors} {onclearerror} />
{:else if formKey === 'SSHD'}
	<SshdParamsForm bind:params={params as SshdFormState} {errors} {onclearerror} />
{:else if formKey === 'ADMIN_POLICY'}
	<AdminPolicyParamsForm bind:params={params as AdminPolicyFormState} {errors} {onclearerror} />
{:else if formKey === 'LPS'}
	<LpsParamsForm bind:params={params as LpsFormState} {errors} {onclearerror} />
{:else if formKey === 'ENCRYPTION'}
	<EncryptionParamsForm bind:params={params as EncryptionFormState} {errors} {onclearerror} />
{:else if formKey === 'GROUP'}
	<GroupParamsForm bind:params={params as GroupFormState} {errors} {onclearerror} />
{:else if formKey === 'WIFI'}
	<WifiParamsForm bind:params={params as WifiFormState} {errors} {onclearerror} />
{:else if formKey === 'AGENT_UPDATE'}
	<AgentUpdateParamsForm bind:params={params as AgentUpdateFormState} {errors} {onclearerror} />
{/if}
