<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient } from '$lib/sdk';
	import { setUserLoaders, apiUserLoaders } from './forms/user-loader-context.svelte';
	import type { ManagedAction } from '$contract/cadestro/v1/control_pb';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Card from '$lib/components/ui/card';
	import * as Select from '$lib/components/ui/select';
	import { FieldError } from '$lib/components/ui/field-error';
	import { ArrowLeft, Save } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { createFormValidation } from '$lib/forms';
	import { actionBasicSchema } from '$lib/forms/schemas/actions';

	import {
		getGroupedActionTypeOptions,
		getActionTypeEnum,
		getActionTypeInfoByValue,
		ActionScheduleForm,
		defaultScheduleForm,
		scheduleFormToProto
	} from '$lib/components/actions';
	import { ACTION_REGISTRY, formKeyFromString, type FormKey } from './registry';
	import { createFormBundle } from './action-params/form-bundle.svelte';
	import ActionParamsFormDispatch from './action-params/ActionParamsFormDispatch.svelte';
	import { bindBuilderContext } from './pipeline/builder-pill.svelte';
	import { getLocalizedError } from '$lib/errors';

	interface Props {
		/** Called when user cancels or wants to go back */
		onCancel: () => void;
		/** Called when action is successfully created */
		onCreated: (action: ManagedAction) => void;
		/** Whether to show compact header (for embedding in dialogs) */
		compact?: boolean;
		/** Pre-select action type, skipping type selection step */
		initialType?: string;
		/**
		 * Route the commit through the shell's context pill instead of an inline
		 * Save button. Opt-in, because the several DIALOGS that embed this form
		 * are modal surfaces with their own footers — only the full-page
		 * /actions/new route hands its commit to the pill.
		 */
		pillCommit?: boolean;
	}

	let { onCancel, onCreated, initialType, pillCommit = false }: Props = $props();

	// Wire the apiClient-backed loaders into the form context so every
	// nested UserPicker / GroupParamsForm resolves them transparently.
	setUserLoaders(apiUserLoaders);

	// Step tracking: 'select-type' or 'configure'
	// `initialType` is intentionally consumed only at component construction;
	// the parent does not change it once the dialog is open.
	// svelte-ignore state_referenced_locally
	let step = $state<'select-type' | 'configure'>(initialType ? 'configure' : 'select-type');

	let name = $state('');
	let description = $state('');
	// svelte-ignore state_referenced_locally
	let actionType = $state<string>(initialType ?? '');
	let desiredState = $state<string>('0');
	let timeoutSeconds = $state(300);
	let saving = $state(false);

	// All per-FormKey state + per-type validation lives in one bundle. The
	// registry drives the per-type defaults and schemas; this component
	// no longer hand-iterates the 19 action types.
	const bundle = createFormBundle();
	let scheduleParams = $state(defaultScheduleForm());

	// Get info for the selected action type
	const selectedTypeInfo = $derived.by(() => {
		if (!actionType) return null;
		return getActionTypeInfoByValue(actionType);
	});

	// FormKey resolved from the orchestrator-level actionType string. This is
	// `null` while the user is on the type-selection step.
	const formKey = $derived.by((): FormKey | null => {
		if (!actionType) return null;
		// COMPLIANCE_CHECK is a synthetic key — it shares the SHELL params shape
		// but uses a stricter compliance schema and toggles the form into
		// compliance-only mode.
		if (actionType === 'COMPLIANCE_CHECK') return 'COMPLIANCE_CHECK';
		return formKeyFromString(actionType);
	});

	// Basic-info validation handle (not type-specific).
	const basicValidation = createFormValidation(actionBasicSchema);

	// Action types that don't model PRESENT/ABSENT — looked up via the
	// registry so adding a new type is a single registry edit.
	function supportsAbsent(typeKey: string): boolean {
		const k = typeKey === 'COMPLIANCE_CHECK' ? 'COMPLIANCE_CHECK' : formKeyFromString(typeKey);
		if (!k) return false;
		return ACTION_REGISTRY[k].supportsAbsent;
	}

	function selectType(type: string) {
		actionType = type;
		step = 'configure';
		basicValidation.clearErrors();
		// LPS uses frequent checks (1h) with run-on-assign
		if (type === 'LPS') {
			scheduleParams = { ...scheduleParams, intervalHours: 1, runOnAssign: true, skipIfUnchanged: true };
		}
		// LUKS uses frequent checks (1h) with run-on-assign
		if (type === 'ENCRYPTION') {
			scheduleParams = { ...scheduleParams, intervalHours: 1, runOnAssign: true, skipIfUnchanged: true };
		}
	}

	function goBackToTypeSelection() {
		if (initialType) {
			onCancel();
			return;
		}
		step = 'select-type';
		basicValidation.clearErrors();
		bundle.clearAllErrors();
	}

	function validateTypeParams(): boolean {
		if (!formKey) return false;
		// COMPLIANCE_CHECK reuses the SHELL state but validates against its
		// stricter schema. We mark the SHELL form-state as compliance before
		// validating so downstream code sees the right shape.
		if (formKey === 'COMPLIANCE_CHECK') {
			bundle.params.SHELL.isCompliance = true;
			return bundle.validate('COMPLIANCE_CHECK');
		}
		return bundle.validate(formKey);
	}

	// Live validity for the pill: the same two schemas `createAction` submits
	// against, evaluated on every keystroke so ⌘S is closed before it is pressed.
	const pillValid = $derived.by(() => {
		if (!formKey) return false;
		const basic = actionBasicSchema.safeParse({
			name: name.trim(),
			description: description.trim(),
			timeoutSeconds
		});
		if (!basic.success) return false;
		const stateKey: FormKey = formKey === 'COMPLIANCE_CHECK' ? 'SHELL' : formKey;
		return ACTION_REGISTRY[formKey].schema.safeParse(bundle.params[stateKey]).success;
	});

	bindBuilderContext('action:new', () =>
		pillCommit && step === 'configure' && !saving
			? {
					title: name.trim() || (selectedTypeInfo?.label ?? m.actions_create()),
					dirty: true,
					valid: pillValid,
					commitLabel: m.common_create(),
					subtext: pillValid ? undefined : m.actions_create_incomplete(),
					subtextTone: pillValid ? ('neutral' as const) : ('warn' as const),
					onCommit: () => void createAction(),
					onCancel: onCancel
				}
			: null
	);

	async function createAction() {
		// Validate basic info and type-specific params simultaneously
		const basicValid = basicValidation.validate({
			name: name.trim(),
			description: description.trim(),
			timeoutSeconds
		});
		const typeValid = validateTypeParams();
		if (!basicValid || !typeValid) return;
		if (!formKey) {
			toast.error(m.actions_invalid_type());
			return;
		}

		saving = true;
		try {
			const adapter = ACTION_REGISTRY[formKey];
			// COMPLIANCE_CHECK and SHELL share the SHELL params bucket; both go
			// through the same form-state object.
			const formStateKey: FormKey = formKey === 'COMPLIANCE_CHECK' ? 'SHELL' : formKey;
			const params = {
				case: adapter.paramsCase,
				value: adapter.formToProto(bundle.params[formStateKey])
			} as Parameters<typeof apiClient.createAction>[0]['params'];

			const requestData = {
				name: name.trim(),
				description: description.trim(),
				type: getActionTypeEnum(actionType),
				desiredState: supportsAbsent(actionType) ? parseInt(desiredState) : 0,
				timeoutSeconds,
				schedule: scheduleFormToProto(scheduleParams),
				params
			};
			const action = await apiClient.createAction(requestData);

			if (action) {
				toast.success(m.actions_created());
				onCreated(action);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

{#if step === 'select-type'}
	<!-- Step 1: Select Action Type -->
	<div class={"flex flex-col h-full gap-6"}>

			<div class="flex items-center gap-4">
				<Button variant="ghost" size="icon" aria-label={m.common_back()} onclick={onCancel}>
					<ArrowLeft class="h-4 w-4" />
				</Button>
				<div class="flex-1">
					<h1 class="text-2xl font-bold">{m.actions_create()}</h1>
					<p class="text-muted-foreground">{m.actions_create_subtitle()}</p>
				</div>
			</div>


		<div class={"flex-1 overflow-y-auto space-y-6 pr-2"}>
			{#each getGroupedActionTypeOptions() as group}
				<div class="space-y-3">
					<div>
						<h3 class="text-base font-semibold">{group.label}</h3>
						<p class="text-sm text-muted-foreground">{group.description}</p>
					</div>
					<div class="grid gap-3 sm:grid-cols-2">
						{#each group.types as option}
							{@const info = getActionTypeInfoByValue(option.value)}
							{@const Icon = info.icon}
							<button
								type="button"
								class="group text-left"
								onclick={() => selectType(option.value)}
							>
								<Card.Root
									class="h-full transition-all hover:border-primary hover:shadow-md cursor-pointer"
								>
									<Card.Header class="pb-2">
										<div class="flex items-center gap-3">
											<div
												class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary group-hover:bg-primary group-hover:text-primary-foreground transition-colors"
											>
												<Icon class="h-4 w-4" />
											</div>
											<Card.Title class="text-sm">{info.label}</Card.Title>
										</div>
									</Card.Header>
									<Card.Content class="pt-0">
										<p class="text-xs text-muted-foreground line-clamp-2">{info.description}</p>
									</Card.Content>
								</Card.Root>
							</button>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	</div>
{:else}
	<!-- Step 2: Configure Action -->
	<div class={"flex flex-col h-full gap-4"}>
		<div class="flex items-center gap-4">
			<Button variant="ghost" size="icon" aria-label={m.common_back()} onclick={goBackToTypeSelection}>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<div class="flex-1 min-w-0">
				<div class="flex items-center gap-2">
					{#if selectedTypeInfo}
						{@const Icon = selectedTypeInfo.icon}
						<div
							class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"
						>
							<Icon class="h-3.5 w-3.5" />
						</div>
					{/if}
					<h2 class="text-lg font-semibold truncate">{m.action_detail_new_action({ type: selectedTypeInfo?.label ?? '' })}</h2>
				</div>
			</div>
			{#if !pillCommit}
				<Button onclick={createAction} disabled={saving} size="sm">
					<Save class="mr-2 h-4 w-4" />
					{saving ? m.common_saving() : m.common_save()}
				</Button>
			{/if}
		</div>

		<div class={"flex-1 overflow-y-auto space-y-4 pr-2"}>
			<Card.Root>
				<Card.Header class="pb-3">
					<Card.Title class="text-base">{m.action_detail_basic_info()}</Card.Title>
				</Card.Header>
				<Card.Content class="space-y-3">
					<div class="space-y-1.5">
						<Label for="name">{m.common_name()}</Label>
						<Input
							id="name"
							placeholder={m.action_detail_name_placeholder()}
							bind:value={name}
							required
							aria-invalid={!!basicValidation.errors.name}
							oninput={() => basicValidation.clearFieldError('name')}
						/>
						<FieldError error={basicValidation.errors.name} />
					</div>
					<div class="space-y-1.5">
						<Label for="description">{m.common_description()}</Label>
						<Textarea
							id="description"
							placeholder={m.action_detail_description_placeholder()}
							bind:value={description}
							rows={2}
						/>
					</div>
					<div class="grid gap-3 sm:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="timeout">{m.action_detail_timeout_label()}</Label>
							<Input
								id="timeout"
								type="number"
								min="1"
								max="3600"
								bind:value={timeoutSeconds}
								aria-invalid={!!basicValidation.errors.timeoutSeconds}
								oninput={() => basicValidation.clearFieldError('timeoutSeconds')}
							/>
							<FieldError error={basicValidation.errors.timeoutSeconds} />
						</div>
						{#if supportsAbsent(actionType)}
							<div class="space-y-1.5">
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
							</div>
						{/if}
					</div>
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Card.Header class="pb-3">
					<Card.Title class="text-base">{m.action_detail_parameters()}</Card.Title>
				</Card.Header>
				<Card.Content class="space-y-3">
					{#if formKey}
						{@const stateKey = formKey === 'COMPLIANCE_CHECK' ? 'SHELL' : formKey}
						<ActionParamsFormDispatch
							formKey={formKey}
							bind:params={bundle.params[stateKey]}
							errors={bundle.validations[formKey].errors}
							onclearerror={(f) => bundle.validations[formKey].clearFieldError(f)}
						/>
					{/if}
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Card.Header class="pb-3">
					<Card.Title class="text-base">{m.action_detail_schedule_title()}</Card.Title>
					<Card.Description class="text-xs">
						{m.action_detail_schedule_description()}
					</Card.Description>
				</Card.Header>
				<Card.Content>
					<ActionScheduleForm
						bind:params={scheduleParams}
					/>
				</Card.Content>
			</Card.Root>
		</div>
	</div>
{/if}
