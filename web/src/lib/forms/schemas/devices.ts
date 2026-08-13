import { z } from 'zod';
import * as m from '$lib/paraglide/messages';

export const addLabelSchema = z.object({
	key: z.string().min(1, m.validation_label_key_required()),
	value: z.string().min(1, m.validation_label_value_required())
});
