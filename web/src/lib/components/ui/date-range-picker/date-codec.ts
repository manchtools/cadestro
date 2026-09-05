import { parseDate, type DateValue } from '@internationalized/date';
import type { Codec } from '$lib/url-state';
export const dateCodec: Codec<DateValue | undefined> = {
 parse(value) { if (!value) return undefined; try { return parseDate(value); } catch { return undefined; } },
 serialize: value => value?.toString() ?? null
};
