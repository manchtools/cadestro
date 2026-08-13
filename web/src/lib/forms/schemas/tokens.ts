import { z } from 'zod';
import * as m from '$lib/paraglide/messages';

export const createTokenSchema = z.object({
	name: z.string().min(1, m.validation_token_name_required()),
	oneTime: z.boolean(),
	maxUses: z.number().int().min(0),
	expiresInDays: z.number().int().min(0)
});
