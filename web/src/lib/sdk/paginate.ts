

export interface PageResult<T> {
	items: T[];
	nextPageToken: string;
}

export interface PaginateOptions {

	maxItems?: number;

	pageSize?: number;
}

export async function fetchAllPages<T>(
	fetcher: (pageSize: number, pageToken: string) => Promise<PageResult<T>>,
	options: PaginateOptions = {}
): Promise<T[]> {
	const maxItems = options.maxItems ?? 5000;
	const pageSize = options.pageSize ?? 100;

	const all: T[] = [];
	let token = '';

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
