

export type EmbedMessage =

	| { kind: 'pm.marketplace.hello' }

	| { kind: 'pm.marketplace.init'; subscriptionToken: string | null; parentOrigin: string }

	| { kind: 'pm.marketplace.ready' }
	| {
			kind: 'pm.marketplace.import';
			templateId: string;
			templateType: string;
			templateName: string;
			content: unknown;
	  }
	| { kind: 'pm.marketplace.close' };

export function isEmbedMessage(data: unknown): data is EmbedMessage {
	if (typeof data !== 'object' || data === null) return false;
	const kind = (data as { kind?: unknown }).kind;
	return (
		typeof kind === 'string' &&
		(kind === 'pm.marketplace.hello' ||
			kind === 'pm.marketplace.init' ||
			kind === 'pm.marketplace.ready' ||
			kind === 'pm.marketplace.import' ||
			kind === 'pm.marketplace.close')
	);
}

export function originOf(url: string): string {
	try {
		return new URL(url).origin;
	} catch {
		return '';
	}
}
