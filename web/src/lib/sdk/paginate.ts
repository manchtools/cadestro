// Generic page-through helper for picker components.
//
// F022 / F023: many picker components called list*() with a single magic
// limit (100, 200) and silently truncated tenant data above the cap. The
// audit explicitly recommended either server-side search or full
// pagination. The Search RPC is being adopted incrementally on the list
// pages; pickers stay on list*() but now page through everything up to a
// hard safety cap so we don't accidentally fetch unbounded result sets if
// a server bug returns a non-empty page-token forever.
//
// Caller passes a fetcher closure; the helper repeatedly invokes it with
// the next page token and accumulates the results. Stops on:
//   - empty nextPageToken
//   - reaching the safety cap
//   - the fetcher throwing
//
// The safety cap (default 5000) is generous enough for picker UX —
// at 5000 items the picker is already unusable and the consumer should
// switch to a Search-RPC-driven UI instead.

export interface PageResult<T> {
	items: T[];
	nextPageToken: string;
}

export interface PaginateOptions {
	/** Max items to accumulate before stopping. Default 5000. */
	maxItems?: number;
	/** Per-page request size. Default 100 — the server's maximum allowed
	 *  page_size (the WS13 pagination cap rejects anything larger with
	 *  "page_size must be at most 100"). fetchAllPages follows nextPageToken,
	 *  so a smaller page just means one more round-trip for large tenants. */
	pageSize?: number;
}

/**
 * Page through a list*() RPC until exhausted or the safety cap is hit.
 *
 * @param fetcher  closure that calls `apiClient.listXxx(pageSize, pageToken, ...)`
 *                 and returns `{ items, nextPageToken }`. The caller is
 *                 responsible for projecting the proto response into that
 *                 shape (e.g. `{ items: r.users, nextPageToken: r.nextPageToken }`).
 */
export async function fetchAllPages<T>(
	fetcher: (pageSize: number, pageToken: string) => Promise<PageResult<T>>,
	options: PaginateOptions = {}
): Promise<T[]> {
	const maxItems = options.maxItems ?? 5000;
	const pageSize = options.pageSize ?? 100;

	const all: T[] = [];
	let token = '';
	// Hard upper bound on iterations to defend against a server returning
	// a non-empty token that yields no items. Without this guard a buggy
	// server could spin the picker forever.
	const maxPages = Math.ceil(maxItems / Math.max(1, pageSize)) + 1;
	for (let i = 0; i < maxPages; i++) {
		const remaining = maxItems - all.length;
		if (remaining <= 0) break;
		const requestSize = Math.min(pageSize, remaining);
		const page = await fetcher(requestSize, token);
		all.push(...page.items);
		if (!page.nextPageToken) break;
		token = page.nextPageToken;
	}
	return all;
}
