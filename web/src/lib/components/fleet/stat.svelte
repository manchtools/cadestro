<script lang="ts">

	import { TONE_FILL, type FleetTone } from './tone';

	let { tone = 'ok', value, label }: { tone?: FleetTone; value: number | string; label: string } = $props();
</script>

<span
	data-testid="fleet-stat"
	data-tone={tone}
	class="inline-flex items-center gap-1.5 rounded-[9px] border bg-sunken px-2.5 py-1.5 text-sm"
>
	<span class="h-2 w-2 shrink-0 rounded-full {TONE_FILL[tone]}"></span>
	<b class="text-[0.95rem] font-semibold tabular-nums">{value}</b>
	<span class="text-muted-foreground">{label}</span>
</span>
