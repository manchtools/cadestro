import { z } from 'zod';
import * as m from '$lib/paraglide/messages';

export const createDeviceGroupSchema = z.object({
	name: z.string().min(1, m.validation_name_required()),
	description: z.string(),
	isDynamic: z.boolean(),
	dynamicQuery: z.string()
});
