<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient, type ManagedAction } from '$lib/sdk';
	import { setUserLoaders, apiUserLoaders } from './forms/user-loader-context.svelte';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Save } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import {
		ActionScheduleForm,
		defaultScheduleForm,
		scheduleFormToProto
	} from '$lib/components/actions';
	import { ACTION_REGISTRY, formKeyFromActionType, type FormKey } from './registry';
	import type { FormStateByKey } from './forms/types';
	import { createFormBundle } from './action-params/form-bundle.svelte';
	import ActionParamsFormDispatch from './action-params/ActionParamsFormDispatch.svelte';

	interface Props {
		open: boolean;
		action: ManagedAction | null;
		onsaved: (action: ManagedAction) => void;
	}

	let { open = $bindable(), action, onsaved }: Props = $props();

	setUserLoaders(apiUserLoaders);

	let timeoutSeconds = $state(300);
	let desiredState = $state<string>('0');
	let saving = $state(false);

	const bundle = createFormBundle();
	let scheduleParams = $state(defaultScheduleForm());

	const formKey = $derived.by((): FormKey | null => {
		if (!action) return null;
		const k = formKeyFromActionType(action.type);
		if (!k) return null;

		if (k === 'SHELL' && bundle.params.SHELL?.isCompliance) return 'COMPLIANCE_CHECK';
		return k;
	});

	$effect(() => {
		if (open && action) {
			timeoutSeconds = action.timeoutSeconds || 300;
			loadParamsFromAction();
			bundle.clearAllErrors();
		}
	});

	function loadParamsFromAction() {
		if (!action) return;

		desiredState = String(action.desiredState ?? 0);

		if (action.params) {
			const k = formKeyFromActionType(action.type);
			if (k) {
				const adapter = ACTION_REGISTRY[k];

				if (action.params.case === adapter.paramsCase) {
					bundle.set(k, adapter.protoToForm(action.params.value) as FormStateByKey[typeof k]);
				}
			}
		}

		if (action.schedule) {
			scheduleParams = {
				cron: action.schedule.cron || '',
				intervalHours: action.schedule.intervalHours || 8,
				runOnAssign: action.schedule.runOnAssign ?? true,
				skipIfUnchanged: action.schedule.skipIfUnchanged || false
			};
		}
	}

	function validateTypeParams(): boolean {
		if (!formKey) return false;
		return bundle.validate(formKey);
	}

	async function saveParams() {
		if (!action) return;

		const typeValid = validateTypeParams();
		if (!typeValid) return;
		if (!formKey) {
			toast.error(m.actions_invalid_type());
			return;
		}

		saving = true;
		try {
			const adapter = ACTION_REGISTRY[formKey];

			const stateKey: FormKey = formKey === 'COMPLIANCE_CHECK' ? 'SHELL' : formKey;
			const params = {
				case: adapter.paramsCase,
				value: adapter.formToProto(bundle.params[stateKey])
			} as Parameters<typeof apiClient.updateActionParams>[0]['params'];

			const updated =
				(await apiClient.updateActionParams({
					id: action.id,
					desiredState: supportsAbsent(action.type) ? parseInt(desiredState) : 0,
					timeoutSeconds,
					schedule: scheduleFormToProto(scheduleParams),
					params
				})) ?? null;

			open = false;
			toast.success(m.action_detail_params_updated());
			if (updated) {
				onsaved(updated);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Content class="max-w-2xl max-h-[90vh] overflow-y-auto">
		<AlertDialog.Header>
			<AlertDialog.Title>{m.action_detail_edit_parameters()}</AlertDialog.Title>
		</AlertDialog.Header>
		<div class="py-4 space-y-4">
			<div class="space-y-2">
				<Label for="timeout">{m.action_detail_timeout_label()}</Label>
				<Input id="timeout" type="number" min="1" max="3600" bind:value={timeoutSeconds} />
			</div>

			{#if supportsAbsent(action?.type)}
				<div class="space-y-2">
					<Label>{m.action_detail_desired_state()}</Label>
					<Select.Root type="single" bind:value={desiredState}>
						<Select.Trigger class="w-full">
							{desiredState === '0' ? m.desired_state_present() : m.desired_state_absent()}
						</Select.Trigger>
						<Select.Content>
							<Select.Item value="0">{m.desired_state_present()}</Select.Item>
							<Select.Item value="1">{m.desired_state_absent()}</Select.Item>
						</Select.Content>
					</Select.Root>
					<p class="text-sm text-muted-foreground">{m.action_detail_desired_state_description()}</p>
				</div>
			{/if}

			<hr />

			{#if formKey}
				{@const stateKey = formKey === 'COMPLIANCE_CHECK' ? 'SHELL' : formKey}
				<ActionParamsFormDispatch
					formKey={formKey}
					bind:params={bundle.params[stateKey]}
					errors={bundle.validations[formKey].errors}
					onclearerror={(f) => bundle.clearFieldError(formKey, f)}
				/>
			{/if}

			<hr />

			<div class="space-y-2">
				<h4 class="text-sm font-medium">{m.action_detail_schedule_title()}</h4>
				<ActionScheduleForm bind:params={scheduleParams} />
			</div>
		</div>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={saveParams} disabled={saving}>
				<Save class="mr-2 h-4 w-4" />
				{saving ? m.common_saving() : m.common_save()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<script lang="ts" module>
	import { ActionType as AT } from '$contract/cadestro/v1/actions_pb';
	import { getLocalizedError } from '$lib/errors';

	export function supportsAbsent(type: AT | undefined): boolean {
		return type !== AT.SHELL && type !== AT.UPDATE && type !== AT.AGENT_UPDATE;
	}
</script>
