<script lang="ts">
 import { onMount } from 'svelte';
 import { api } from '$lib/api';
 import { GetDeviceComplianceResponseSchema, type GetDeviceComplianceResponse, type ComplianceCheckResult } from '$contract/cadestro/v1/control_pb';
 import { ComplianceStatus } from '$contract/cadestro/v1/common_pb';
 import { toast } from 'svelte-sonner';
 import { getLocalizedError } from '$lib/errors';
 import { formatDate as formatTimestampDateTime } from '$lib/console';
 import * as Table from '$lib/components/ui/table';
 import * as Sheet from '$lib/components/ui/sheet';
 import { Chip, type FleetTone } from '$lib/components/fleet';
 import { ShieldCheck, ShieldAlert, ShieldQuestion, Zap } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
 let { deviceId }: { deviceId: string } = $props();
 let policyStatus = $state<GetDeviceComplianceResponse | null>(null);
 let loading = $state(true);
 let selected = $state<ComplianceCheckResult | null>(null);
 const checks = $derived(policyStatus?.checks ?? []);
 const hasData = $derived(checks.length > 0);
 function getStatusIcon(status: ComplianceStatus) { return status === ComplianceStatus.COMPLIANT ? ShieldCheck : status === ComplianceStatus.NON_COMPLIANT ? ShieldAlert : ShieldQuestion; }
 function getStatusTone(status: ComplianceStatus): FleetTone { return status === ComplianceStatus.COMPLIANT ? 'ok' : status === ComplianceStatus.NON_COMPLIANT ? 'crit' : 'idle'; }
 function getStatusLabel(status: ComplianceStatus) { return ComplianceStatus[status].replaceAll('_', ' '); }
 onMount(async () => { try { policyStatus = await api.getDeviceCompliance({ deviceId: { value: deviceId } }); } catch(error) { toast.error(getLocalizedError(error)); } finally { loading = false; } });
</script>
<div class="space-y-4">
	{#if loading}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<div class="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
		</div>
	{:else if hasData && policyStatus}

		{@const StatusIcon = getStatusIcon(policyStatus.status)}
		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
			<div class="flex items-center gap-2">
				<StatusIcon class="h-4 w-4 text-faint" />
				<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.compliance_title()}
				</span>
			</div>
			<div class="mt-3 flex items-center gap-4">
				<Chip
					tone={getStatusTone(policyStatus.status)}
					label={getStatusLabel(policyStatus.status)}
				/>
				<span class="text-sm text-muted-foreground">
					{m.compliance_checks_passing({
						passing: String(checks.filter((c) => c.status === ComplianceStatus.COMPLIANT).length),
						total: String(checks.length)
					})}
				</span>
			</div>
		</section>

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

							<Table.Head class="w-[120px]">{m.compliance_status()}</Table.Head>
							<Table.Head class="w-[180px]">{m.compliance_checked_at()}</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each checks as check}
							<Table.Row class="cursor-pointer" onclick={() => { selected = check; }}>
								<Table.Cell class="font-medium text-sm">{check.actionName}</Table.Cell>
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

<Sheet.Root open={!!selected} onOpenChange={open => { if (!open) selected = null; }}><Sheet.Content><Sheet.Header><Sheet.Title>{selected?.actionName}</Sheet.Title><Sheet.Description>{selected ? getStatusLabel(selected.status) : ''}</Sheet.Description></Sheet.Header>{#if selected?.detectionOutput}<div class="rounded-md bg-sunken p-3 font-mono text-xs"><pre class="whitespace-pre-wrap">{selected.detectionOutput.stdout}</pre><pre class="whitespace-pre-wrap text-destructive">{selected.detectionOutput.stderr}</pre><p>{m.compliance_result_exit_code({ code: selected.detectionOutput.exitCode })}</p></div>{/if}</Sheet.Content></Sheet.Root>
