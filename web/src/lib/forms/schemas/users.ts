import { z } from 'zod';
import * as m from '$lib/paraglide/messages';

export const updateUserEmailSchema = z.object({
	email: z.string().min(1, m.validation_email_required()).email(m.validation_email_invalid())
});
