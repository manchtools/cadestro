<script lang="ts" module>
	import * as m from '$lib/paraglide/messages';
	import type { MaintenanceWindow, MaintenanceWindowEntry } from '$contract/cadestro/v1/common_pb';

	export type MaintenanceWindowEntryInput = {
		days: string[];
		allow: string;
	};

	const DAYS: Array<{ token: string; label: string }> = [
		{ token: 'mon', label: m.maintenance_window_day_mon() },
		{ token: 'tue', label: m.maintenance_window_day_tue() },
		{ token: 'wed', label: m.maintenance_window_day_wed() },
		{ token: 'thu', label: m.maintenance_window_day_thu() },
		{ token: 'fri', label: m.maintenance_window_day_fri() },
		{ token: 'sat', label: m.maintenance_window_day_sat() },
		{ token: 'sun', label: m.maintenance_window_day_sun() }
	];

	export function formatEntry(e: MaintenanceWindowEntryInput): string {
		// Day list reuses the canonical token order so a window stays
		// readable regardless of insertion order — no surprise reshuffles
		// when the user re-opens the editor.
		const days = DAYS.filter((d) => e.days.includes(d.token))
			.map((d) => d.label)
			.join(', ');
		return `${days} · ${e.allow}`;
	}

	export function entriesFromWindow(w?: MaintenanceWindow | null): MaintenanceWindowEntryInput[] {
		if (!w || !w.schedule) return [];
		return w.schedule.map((e: MaintenanceWindowEntry) => ({
			days: [...(e.days ?? [])],
			allow: e.allow ?? ''
		}));
	}

	export { DAYS };
</script>

<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Trash2 } from '@lucide/svelte';

	interface Props {
		open: boolean;
		entries: MaintenanceWindowEntryInput[];
		title: string;
		description: string;
		onsave: (entries: MaintenanceWindowEntryInput[]) => void;
	}

	let { open = $bindable(), entries, title, description, onsave }: Props = $props();

	// Edit a working copy so cancel reverts cleanly. Reset whenever
	// the dialog re-opens — this is the only Svelte $effect that
	// depends on `open` to avoid the replaceState-during-hydration
	// trap noted in MEMORY.
	let working: MaintenanceWindowEntryInput[] = $state([]);
	let newDays: Set<string> = $state(new Set());
	let newStart = $state('22:00');
	let newEnd = $state('06:00');
	let error = $state('');

	$effect(() => {
		if (open) {
			working = entries.map((e) => ({ days: [...e.days], allow: e.allow }));
			newDays = new Set();
			newStart = '22:00';
			newEnd = '06:00';
			error = '';
		}
	});

	function isValidClock(s: string): boolean {
		// HH:MM with H 00-23, M 00-59. The browser <input type="time">
		// already normalises these, but we still validate before save
		// in case the user pastes/edits manually.
		return /^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(s);
	}

	function addEntry() {
		error = '';
		if (newDays.size === 0) {
			error = m.maintenance_window_error_no_days();
			return;
		}
		if (!isValidClock(newStart) || !isValidClock(newEnd)) {
			error = m.maintenance_window_error_bad_time();
			return;
		}
		if (newStart === newEnd) {
			error = m.maintenance_window_error_zero_length();
			return;
		}
		const days = DAYS.filter((d) => newDays.has(d.token)).map((d) => d.token);
		working = [...working, { days, allow: `${newStart}-${newEnd}` }];
		newDays = new Set();
	}

	function removeEntry(idx: number) {
		working = working.filter((_, i) => i !== idx);
	}

	function toggleDay(token: string) {
		const next = new Set(newDays);
		if (next.has(token)) next.delete(token);
		else next.add(token);
		newDays = next;
	}

	function clearAll() {
		working = [];
	}

	function handleSave() {
		onsave(working);
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{title}</Dialog.Title>
			<Dialog.Description>
				{description}
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4">
			<div>
				<Label class="text-muted-foreground">{m.maintenance_window_current_label()}</Label>
				{#if working.length === 0}
					<p class="text-sm text-muted-foreground italic mt-1">
						{m.maintenance_window_empty_state()}
					</p>
				{:else}
					<ul class="space-y-1 mt-1">
						{#each working as entry, idx}
							<li class="flex items-center justify-between gap-2 rounded-md border p-2">
								<span class="text-sm">{formatEntry(entry)}</span>
								<Button
									variant="ghost"
									size="icon"
									aria-label={m.maintenance_window_remove_entry()}
									onclick={() => removeEntry(idx)}
								>
									<Trash2 class="h-4 w-4" />
								</Button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			<div class="space-y-2 border-t pt-4">
				<Label>{m.maintenance_window_add_entry_label()}</Label>
				<div class="flex flex-wrap gap-2">
					{#each DAYS as day}
						<label class="flex items-center gap-1 cursor-pointer">
							<Checkbox
								checked={newDays.has(day.token)}
								onCheckedChange={() => toggleDay(day.token)}
							/>
							<span class="text-sm">{day.label}</span>
						</label>
					{/each}
				</div>
				<div class="flex items-center gap-2">
					<div class="flex-1">
						<Label for="window-start" class="text-xs text-muted-foreground">
							{m.maintenance_window_start()}
						</Label>
						<Input id="window-start" type="time" bind:value={newStart} />
					</div>
					<div class="flex-1">
						<Label for="window-end" class="text-xs text-muted-foreground">
							{m.maintenance_window_end()}
						</Label>
						<Input id="window-end" type="time" bind:value={newEnd} />
					</div>
					<Button class="self-end" variant="secondary" onclick={addEntry}>
						{m.maintenance_window_add()}
					</Button>
				</div>
				{#if error}
					<p class="text-sm text-destructive">{error}</p>
				{/if}
				<p class="text-xs text-muted-foreground">
					{m.maintenance_window_local_time_hint()}
				</p>
			</div>
		</div>

		<Dialog.Footer class="gap-2">
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button variant="ghost" onclick={clearAll} disabled={working.length === 0}>
				{m.maintenance_window_clear_all()}
			</Button>
			<Button onclick={handleSave}>{m.common_save()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
