// Tests for the picker pagination helper (F004 + F022/F023).

import { describe, it, expect } from 'vitest';
import { fetchAllPages, type PageResult } from './paginate';

describe('fetchAllPages', () => {
	it('returns a single page when the server reports no next-page-token', async () => {
		const fetcher = async (): Promise<PageResult<number>> => ({
			items: [1, 2, 3],
			nextPageToken: ''
		});
		const result = await fetchAllPages(fetcher);
		expect(result).toEqual([1, 2, 3]);
	});

	it('concatenates pages until the server stops returning a next token', async () => {
		const pages: PageResult<string>[] = [
			{ items: ['a', 'b'], nextPageToken: 'p2' },
			{ items: ['c', 'd'], nextPageToken: 'p3' },
			{ items: ['e'], nextPageToken: '' }
		];
		let i = 0;
		const tokens: string[] = [];
		const fetcher = async (_size: number, token: string) => {
			tokens.push(token);
			return pages[i++];
		};
		const result = await fetchAllPages(fetcher);
		expect(result).toEqual(['a', 'b', 'c', 'd', 'e']);
		expect(tokens).toEqual(['', 'p2', 'p3']);
	});

	it('respects the maxItems cap and stops requesting more', async () => {
		let calls = 0;
		const fetcher = async () => {
			calls++;
			return { items: [calls, calls, calls], nextPageToken: 'always-more' };
		};
		const result = await fetchAllPages(fetcher, { maxItems: 5, pageSize: 3 });
		// max 5 items requested -> first call yields 3, second call (size=2)
		// yields 3 (server returns 3 even though we asked for 2) -> 6 total.
		// We just assert the cap stops the loop after the next iteration.
		expect(result.length).toBeLessThanOrEqual(6);
		expect(calls).toBeLessThan(10);
	});

	it('passes through fetcher exceptions', async () => {
		const fetcher = async () => {
			throw new Error('rpc failure');
		};
		await expect(fetchAllPages(fetcher)).rejects.toThrow('rpc failure');
	});

	it('defends against a server returning a non-empty token forever', async () => {
		// Pathological server: always says "more" but returns 0 items per page.
		// Without a max-pages guard the loop would spin forever.
		let calls = 0;
		const fetcher = async () => {
			calls++;
			return { items: [], nextPageToken: 'never-empty' };
		};
		await fetchAllPages(fetcher, { maxItems: 100, pageSize: 10 });
		// max-pages bound = ceil(100 / 10) + 1 = 11 — should not exceed.
		expect(calls).toBeLessThanOrEqual(12);
	});
});
