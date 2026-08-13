// postMessage contract with the marketplace iframe. Mirror of the
// definition inside the marketplace repo — kept in sync by hand for
// now so PM web has a single source of truth it can type against.
//
// Any change here must be reflected in
// power-manage-marketplace/web/src/lib/embed.ts.

export type EmbedMessage =
	// Embed → host. Posted once the embed's message listener is
	// attached; tells the host it's safe to send init. Carries no
	// secrets so the embed targets origin '*'; host filters by
	// event.source.
	| { kind: 'pm.marketplace.hello' }
	// Host → embed. Response to hello.
	| { kind: 'pm.marketplace.init'; subscriptionToken: string | null; parentOrigin: string }
	// Embed → host. Final ack — UI is visible and interactive.
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

/** Extracts the origin (scheme + host + port) from a URL string. */
export function originOf(url: string): string {
	try {
		return new URL(url).origin;
	} catch {
		return '';
	}
}
