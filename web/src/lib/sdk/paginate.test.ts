

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

		let calls = 0;
		const fetcher = async () => {
			calls++;
			return { items: [], nextPageToken: 'never-empty' };
		};
		await fetchAllPages(fetcher, { maxItems: 100, pageSize: 10 });

		expect(calls).toBeLessThanOrEqual(12);
	});
});
