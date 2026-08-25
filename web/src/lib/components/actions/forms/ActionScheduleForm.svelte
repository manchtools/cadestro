<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';
	import type { ScheduleFormState } from './types';

	interface Props {
		params: ScheduleFormState;
		errors?: Partial<Record<string, string>>;
		onclearerror?: (field: string) => void;
	}

	let { params = $bindable(), errors, onclearerror }: Props = $props();

	const hasCron = $derived(params.cron.trim() !== '');

	const cronDescription = $derived(describeCron(params.cron.trim()));

	const presets = [
		{ label: () => m.actions_schedule_cron_preset_hourly(), cron: '0 * * * *' },
		{ label: () => m.actions_schedule_cron_preset_every_8h(), cron: '0 */8 * * *' },
		{ label: () => m.actions_schedule_cron_preset_daily_midnight(), cron: '0 0 * * *' },
		{ label: () => m.actions_schedule_cron_preset_daily_3am(), cron: '0 3 * * *' },
		{ label: () => m.actions_schedule_cron_preset_weekly_sunday(), cron: '0 3 * * 0' },
		{ label: () => m.actions_schedule_cron_preset_monthly(), cron: '0 3 1 * *' }
	];

	function applyPreset(cron: string) {
		params.cron = cron;
	}

	function describeCron(expr: string): string {
		if (!expr) return '';
		const parts = expr.split(/\s+/);
		if (parts.length !== 5) return '';

		const [minute, hour, dom, month, dow] = parts;

		try {

			if (minute === '*' && hour === '*' && dom === '*' && month === '*' && dow === '*') {
				return 'Runs every minute';
			}

			const minStr = minute === '0' ? ':00' : `:${minute.padStart(2, '0')}`;

			if (minute.startsWith('*/') && hour === '*' && dom === '*' && month === '*' && dow === '*') {
				return `Runs every ${minute.slice(2)} minutes`;
			}

			if (minute !== '*' && hour === '*' && dom === '*' && month === '*' && dow === '*') {
				if (minute.startsWith('*/')) return `Runs every ${minute.slice(2)} minutes`;
				return `Runs every hour at ${minStr}`;
			}

			if (hour.startsWith('*/') && dom === '*' && month === '*' && dow === '*') {
				return `Runs every ${hour.slice(2)} hours at ${minStr}`;
			}

			if (!hour.includes('*') && !hour.includes('/') && dom === '*' && month === '*' && dow === '*') {
				if (hour.includes(',')) {
					return `Runs daily at ${hour.split(',').map((h) => `${h}${minStr}`).join(', ')}`;
				}
				return `Runs daily at ${hour}${minStr}`;
			}

			if (dom === '*' && month === '*' && dow !== '*') {
				const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
				const dayNum = parseInt(dow, 10);
				const dayName = dayNum >= 0 && dayNum <= 6 ? days[dayNum] : dow;
				return `Runs weekly on ${dayName} at ${hour}${minStr}`;
			}

			if (dom !== '*' && month === '*' && dow === '*') {
				const suffix = dom === '1' ? 'st' : dom === '2' ? 'nd' : dom === '3' ? 'rd' : 'th';
				return `Runs monthly on the ${dom}${suffix} at ${hour}${minStr}`;
			}

			return '';
		} catch (err) {
			console.warn('cron summary parse failed', err);
			return '';
		}
	}
</script>

<div class="space-y-4">
	<div class="grid gap-4 md:grid-cols-2">
		<div class="space-y-2">
			<Label for="scheduleCron">{m.actions_schedule_cron()}</Label>
			<Input
				id="scheduleCron"
				placeholder="0 3 * * *"
				bind:value={params.cron}
			/>
			{#if cronDescription}
				<p class="text-xs text-ok">{cronDescription}</p>
			{:else}
				<p class="text-xs text-muted-foreground">
					{m.actions_schedule_cron_description()}
				</p>
			{/if}
			<div class="flex flex-wrap gap-1.5 pt-1">
				<span class="text-xs text-muted-foreground">{m.actions_schedule_cron_presets()}:</span>
				{#each presets as preset}
					<button
						type="button"
						class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs transition-colors hover:bg-accent hover:text-accent-foreground cursor-pointer {params.cron === preset.cron ? 'bg-accent text-accent-foreground border-primary/30' : 'border-transparent'}"
						onclick={() => applyPreset(preset.cron)}
					>
						{preset.label()}
					</button>
				{/each}
			</div>
		</div>
		<div class="space-y-2">
			<Label for="scheduleInterval" class={hasCron ? 'text-muted-foreground' : ''}>{m.actions_schedule_interval()}</Label>
			<Input
				id="scheduleInterval"
				type="number"
				min="1"
				max="8760"
				bind:value={params.intervalHours}
				aria-invalid={!!errors?.intervalHours}
				oninput={() => onclearerror?.('intervalHours')}
				disabled={hasCron}
			/>
			<FieldError error={errors?.intervalHours} />
			<p class="text-xs text-muted-foreground">
				{hasCron ? m.actions_schedule_interval_disabled_by_cron() : m.actions_schedule_interval_default()}
			</p>
		</div>
	</div>
	<div class="flex flex-wrap gap-6">
		<div class="flex items-center gap-3">
			<Switch id="scheduleRunOnAssign" bind:checked={params.runOnAssign} />
			<div class="space-y-0.5">
				<Label for="scheduleRunOnAssign">{m.actions_schedule_run_on_assign()}</Label>
				<p class="text-xs text-muted-foreground">{m.actions_schedule_run_on_assign_description()}</p>
			</div>
		</div>
		<div class="flex items-center gap-3">
			<Switch id="scheduleSkipIfUnchanged" bind:checked={params.skipIfUnchanged} />
			<div class="space-y-0.5">
				<Label for="scheduleSkipIfUnchanged">{m.actions_schedule_skip_if_unchanged()}</Label>
				<p class="text-xs text-muted-foreground">{m.actions_schedule_skip_if_unchanged_description()}</p>
			</div>
		</div>
	</div>
</div>
