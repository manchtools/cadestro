<script lang="ts">
	import * as Popover from '$lib/components/ui/popover';
	import { RangeCalendar } from '$lib/components/ui/range-calendar';
	import { Button } from '$lib/components/ui/button';
	import { CalendarDays, X } from '@lucide/svelte';
	import { cn } from '$lib/utils';
	import {
		type DateValue,
		DateFormatter,
		getLocalTimeZone
	} from '@internationalized/date';
	import type { DateRange } from 'bits-ui';

	let {
		start = $bindable<DateValue | undefined>(undefined),
		end = $bindable<DateValue | undefined>(undefined),
		onChange,
		placeholder = 'Filter by date...',
		class: className
	}: {
		start?: DateValue | undefined;
		end?: DateValue | undefined;
		/** Optional callback fired only on user-interaction range changes
		 *  (calendar select or clear button) — not when start/end are set
		 *  externally via bind:. Intended for URL-as-state syncing where
		 *  the caller needs to distinguish user-driven changes from
		 *  state-set-from-URL to avoid history loops. */
		onChange?: (value: { start?: DateValue; end?: DateValue }) => void;
		placeholder?: string;
		class?: string;
	} = $props();

	let open = $state(false);

	const df = new DateFormatter('en-US', { month: 'short', day: 'numeric', year: 'numeric' });

	// rangeValue is a $derived view of the (start, end) prop pair so that
	// any external change to the props — including initial values seeded
	// from URL state on page load — flows straight to the underlying
	// RangeCalendar. The previous $effect-based sync raced against
	// hydration: the trigger label rendered with the right dates, but
	// the calendar's selected highlights and the popover's "value"
	// remained empty until the next interaction. $derived removes the
	// race entirely.
	const rangeValue = $derived<DateRange | undefined>(
		start || end ? { start, end } : undefined
	);

	function handleRangeChange(val: DateRange | undefined) {
		start = val?.start;
		end = val?.end;
		onChange?.({ start: val?.start, end: val?.end });
	}

	const buttonLabel = $derived.by(() => {
		if (!start && !end) return placeholder;
		const s = start ? df.format(start.toDate(getLocalTimeZone())) : '...';
		const e = end ? df.format(end.toDate(getLocalTimeZone())) : '...';
		return `${s} – ${e}`;
	});

	const hasValue = $derived(!!start || !!end);

	function clear(event: MouseEvent) {
		event.stopPropagation();
		start = undefined;
		end = undefined;
		onChange?.({ start: undefined, end: undefined });
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		<Button
			variant="outline"
			class={cn(
				'justify-between font-normal',
				!hasValue && 'text-muted-foreground',
				className
			)}
		>
			<CalendarDays class="mr-2 h-4 w-4 shrink-0" />
			<span class="truncate">{buttonLabel}</span>
			{#if hasValue}
				<button
					type="button"
					class="ml-2 rounded-full hover:bg-muted p-0.5"
					onclick={clear}
				>
					<X class="h-3 w-3" />
				</button>
			{/if}
		</Button>
	</Popover.Trigger>
	<Popover.Content class="w-auto p-0" align="start">
		<RangeCalendar
			value={rangeValue}
			onValueChange={handleRangeChange}
			numberOfMonths={2}
			weekStartsOn={1}
		/>
	</Popover.Content>
</Popover.Root>
