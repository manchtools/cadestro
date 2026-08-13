import { z } from 'zod';
import * as m from '$lib/paraglide/messages';

export const editNameSchema = z.object({
	name: z.string().min(1, m.validation_name_required())
});

export const editDescriptionSchema = z.object({
	description: z.string()
});

export const nameDescriptionSchema = z.object({
	name: z.string().min(1, m.validation_name_required()),
	description: z.string()
});
