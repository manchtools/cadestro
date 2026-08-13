import { z } from 'zod';
import * as m from '$lib/paraglide/messages';

export const setupSchema = z.object({
	serverUrl: z.string().min(1, m.validation_server_url_required()).url(m.validation_server_url_invalid())
});
