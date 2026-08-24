<script lang="ts">
	import { onMount } from 'svelte';
	import { getLocalizedError } from '$lib/errors';
	import { toast } from 'svelte-sonner';
	import { page } from '$app/state';
	import { apiClient, formatTimestampDateTime } from '$lib/sdk';
	import { ComplianceStatus, type CommandOutput } from '$contract/cadestro/v1/common_pb';
	import type { Timestamp } from '@bufbuild/protobuf/wkt';
	import type { GetDeviceCompliancePolicyStatusResponse } from '$contract/cadestro/v1/control_pb';
	import ActionDetailSheet, { openActionSheet } from '$lib/components/actions/action-detail-sheet.svelte';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Chip } from '$lib/components/fleet';
	import type { FleetTone } from '$lib/components/fleet/tone';
	import { ShieldCheck, ShieldAlert, ShieldQuestion, Clock, Zap } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import CompliancePolicyDetailSheet, { openCompliancePolicySheet } from '$lib/components/compliance-policy-detail-sheet.svelte';

	interface Props {
		deviceId: string;
	}

	let { deviceId }: Props = $props();

	let policyStatus = $state<GetDeviceCompliancePolicyStatusResponse | null>(null);
	let loading = $state(true);

	// Derive currently-shown action ID from ActionDetailSheet shallow routing
	let sheetActionId = $derived(page.state.actionSheet);

	type PolicyParent = { policyId: string; policyName: string };

	type MergedCheck = {
		actionId: string;
		actionName: string;
		status: ComplianceStatus;
		checkedAt?: Timestamp;
		detectionOutput?: CommandOutput;
		gracePeriodHours: number;
		policies: PolicyParent[];
	};

	// Flatten and deduplicate: each action appears once with all parent policies
	const mergedChecks = $derived.by(() => {
		if (!policyStatus) return [];
		const actionMap = new Map<string, MergedCheck>();
		for (const policy of policyStatus.policies) {
			for (const rule of policy.rules) {
				const existing = actionMap.get(rule.actionId);
				if (existing) {
					existing.policies.push({ policyId: policy.policyId, policyName: policy.policyName });
				} else {
					actionMap.set(rule.actionId, {
						actionId: rule.actionId,
						actionName: rule.actionName,
						status: rule.status,
						checkedAt: rule.checkedAt,
						detectionOutput: rule.detectionOutput,
						gracePeriodHours: rule.gracePeriodHours,
						policies: [{ policyId: policy.policyId, policyName: policy.policyName }]
					});
				}
			}
		}
		return Array.from(actionMap.values());
	});

	onMount(() => {
		loadCompliance();
	});

	async function loadCompliance() {
		loading = true;
		try {
			policyStatus = await apiClient.getDeviceCompliancePolicyStatus(deviceId);
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	function getSheetCheck(actionId: string): MergedCheck | undefined {
		return mergedChecks.find((c) => c.actionId === actionId);
	}

	function getStatusIcon(status: ComplianceStatus) {
		switch (status) {
			case ComplianceStatus.COMPLIANT:
				return ShieldCheck;
			case ComplianceStatus.NON_COMPLIANT:
				return ShieldAlert;
			case ComplianceStatus.IN_GRACE_PERIOD:
				return Clock;
			default:
				return ShieldQuestion;
		}
	}

	function getStatusLabel(status: ComplianceStatus): string {
		switch (status) {
			case ComplianceStatus.COMPLIANT:
				return m.compliance_status_compliant();
			case ComplianceStatus.NON_COMPLIANT:
				return m.compliance_status_non_compliant();
			case ComplianceStatus.IN_GRACE_PERIOD:
				return m.compliance_status_in_grace_period();
			default:
				return m.compliance_status_unknown();
		}
	}

	// Same buckets the compliance-policy detail page paints its device rows
	// with, so one device never reads as two different states across surfaces.
	function getStatusTone(status: ComplianceStatus): FleetTone {
		switch (status) {
			case ComplianceStatus.COMPLIANT:
				return 'ok';
			case ComplianceStatus.NON_COMPLIANT:
				return 'crit';
			case ComplianceStatus.IN_GRACE_PERIOD:
				return 'warn';
			default:
				return 'idle';
		}
	}

	const checks = $derived(mergedChecks);
	const hasData = $derived(checks.length > 0);
</script>

<div class="space-y-4">
	{#if loading}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<div class="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
		</div>
	{:else if hasData && policyStatus}
		<!-- Summary -->
		{@const StatusIcon = getStatusIcon(policyStatus.overallStatus)}
		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
			<div class="flex items-center gap-2">
				<StatusIcon class="h-4 w-4 text-faint" />
				<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.compliance_title()}
				</span>
			</div>
			<div class="mt-3 flex items-center gap-4">
				<Chip
					tone={getStatusTone(policyStatus.overallStatus)}
					label={getStatusLabel(policyStatus.overallStatus)}
				/>
				<span class="text-sm text-muted-foreground">
					{m.compliance_checks_passing({
						passing: String(checks.filter((c) => c.status === ComplianceStatus.COMPLIANT).length),
						total: String(checks.length)
					})}
				</span>
			</div>
		</section>

		<!-- Compliance Checks Table -->
		<section class="overflow-hidden rounded-xl border border-hair bg-surface shadow-plate">
			<div class="flex items-center gap-2 border-b border-hair px-4 py-3">
				<Zap class="h-4 w-4 text-faint" />
				<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.compliance_policy_detail_rules()}
				</span>
			</div>
			<div>
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>{m.common_name()}</Table.Head>
							<Table.Head>{m.nav_compliance_policies()}</Table.Head>
							<Table.Head class="w-[120px]">{m.compliance_status()}</Table.Head>
							<Table.Head class="w-[180px]">{m.compliance_checked_at()}</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each checks as check}
							<Table.Row class="cursor-pointer" onclick={() => openActionSheet(check.actionId)}>
								<Table.Cell class="font-medium text-sm">{check.actionName}</Table.Cell>
								<Table.Cell class="text-sm">
									{#if check.policies.length === 1}
										<button
											type="button"
											class="text-primary hover:underline"
											onclick={(e) => {
												e.stopPropagation();
												openCompliancePolicySheet(check.policies[0].policyId);
											}}
										>
											{check.policies[0].policyName}
										</button>
									{:else if check.policies.length > 1}
										<button
											type="button"
											class="text-primary hover:underline"
											onclick={(e) => {
												e.stopPropagation();
												openCompliancePolicySheet(check.policies[0].policyId);
											}}
										>
											{check.policies[0].policyName}
										</button>
										<Badge variant="outline" class="ml-1 text-xs">+{check.policies.length - 1}</Badge>
									{/if}
								</Table.Cell>
								<Table.Cell>
									<Chip tone={getStatusTone(check.status)} label={getStatusLabel(check.status)} />
								</Table.Cell>
								<Table.Cell class="text-sm text-muted-foreground">
									{#if check.checkedAt}
										{formatTimestampDateTime(check.checkedAt)}
									{:else}
										—
									{/if}
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			</div>
		</section>
	{:else}
		<div
			class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface py-12 text-center shadow-plate"
		>
			<ShieldQuestion class="mb-2 h-8 w-8 text-muted-foreground" />
			<p class="text-muted-foreground">{m.compliance_no_checks()}</p>
		</div>
	{/if}
</div>

<!-- Action Detail Sheet (shallow routing) with compliance-specific children -->
<ActionDetailSheet>
	{#if sheetActionId}
		{@const check = getSheetCheck(sheetActionId)}
		{#if check}
			<!-- Compliance Status -->
			<div class="space-y-3">
				<h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">{m.compliance_status()}</h3>
				<div class="space-y-2">
					<div class="flex items-center gap-2">
						<Chip tone={getStatusTone(check.status)} label={getStatusLabel(check.status)} />
						{#if check.gracePeriodHours > 0}
							<span class="flex items-center gap-1 text-xs text-muted-foreground">
								<Clock class="h-3 w-3" />
								{m.compliance_policy_detail_grace_period_hours({ count: check.gracePeriodHours })}
							</span>
						{/if}
					</div>
					{#if check.checkedAt}
						<p class="text-xs text-muted-foreground">
							{m.compliance_policy_last_checked()}: {formatTimestampDateTime(check.checkedAt)}
						</p>
					{/if}
				</div>
			</div>

			<!-- Detection output -->
			{#if check.detectionOutput}
				<hr />
				<div class="space-y-3">
					<h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
						{m.compliance_detection_output()}
					</h3>
					<div class="rounded-md bg-sunken p-3 font-mono text-xs">
						{#if check.detectionOutput.stdout}
							<pre class="whitespace-pre-wrap">{check.detectionOutput.stdout}</pre>
						{/if}
						{#if check.detectionOutput.stderr}
							<pre class="whitespace-pre-wrap text-destructive">{check.detectionOutput.stderr}</pre>
						{/if}
						<div class="mt-1 text-muted-foreground">
							{m.compliance_result_exit_code({ code: check.detectionOutput.exitCode })}
						</div>
					</div>
				</div>
			{/if}

			<!-- Part of (parent policies) -->
			{#if check.policies.length > 0}
				<hr />
				<div class="space-y-3">
					<h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
						{m.policies_part_of()}
					</h3>
					<div class="space-y-1">
						{#each check.policies as policy}
							<button
								type="button"
								class="flex w-full items-center gap-2 rounded-md p-2 text-left text-sm transition-colors hover:bg-muted"
								onclick={() => openCompliancePolicySheet(policy.policyId)}
							>
								<ShieldCheck class="h-4 w-4 text-muted-foreground shrink-0" />
								<span class="truncate">{policy.policyName}</span>
								<Badge variant="outline" class="text-xs shrink-0 ml-auto">
									{m.nav_compliance_policies()}
								</Badge>
							</button>
						{/each}
					</div>
				</div>
			{/if}
		{/if}
	{/if}
</ActionDetailSheet>

<!-- Compliance Policy Detail Sheet (shallow routing) -->
<CompliancePolicyDetailSheet />
