

import { test, expect, preparePage, gotoAndSettle, recordRpc } from './fixtures';
import { wrappedID } from '../showcase/dummy';
import type { Page } from '@playwright/test';

type SearchReq = {
	scope?: string;
	query?: string;
	pageSize?: number;
	pageToken?: string;
	sortField?: string;
	sortDirection?: string;
	tagFilters?: Record<string, string>;
};

function lastSearch(calls: SearchReq[]): SearchReq {
	return calls[calls.length - 1] ?? {};
}

test.describe('list search', () => {
	test('typing in the search box emits a Search with the query', async ({ page }) => {
		await preparePage(page, 'light');
		const search = recordRpc<SearchReq>(page, 'Search');
		await gotoAndSettle(page, '/actions?zoom=list', '[data-testid="row-list-row"]');
		await page.getByRole('button', { name: 'Open search' }).click();
		await page.getByTestId('global-search').getByRole('combobox').fill('edge');
		await expect.poll(() => lastSearch(search).query, { timeout: 5000 }).toBe('edge');
		expect(lastSearch(search).scope).toBe('SEARCH_SCOPE_ACTIONS');
	});
});

test.describe('list sort', () => {
	test('clicking a column header sorts by that field; clicking again flips direction', async ({ page }) => {
		await preparePage(page, 'light');
		const search = recordRpc<SearchReq>(page, 'Search');
		await gotoAndSettle(page, '/action-sets?zoom=list', '[data-testid="row-list-row"]');

		await page.getByTestId('row-list-sort').getByRole('button', { name: 'Name' }).click();
		await expect.poll(() => lastSearch(search).sortField, { timeout: 5000 }).toBe('SORT_FIELD_NAME');
		expect(lastSearch(search).sortDirection).toBe('SORT_DIRECTION_ASC');

		await page.getByTestId('row-list-sort').getByRole('button', { name: 'Name' }).click();
		await expect
			.poll(() => lastSearch(search).sortDirection, { timeout: 5000 })
			.toBe('SORT_DIRECTION_DESC');
		expect(lastSearch(search).sortField).toBe('SORT_FIELD_NAME');
	});
});

test.describe('empty-relation filters (#325)', () => {
	const cases: Array<{ name: string; path: string; label: string; field: string }> = [
		{ name: 'action-sets · no assigned actions', path: '/action-sets?zoom=list', label: 'No assigned actions', field: 'member_count' },
		{ name: 'definitions · no assigned action sets', path: '/definitions?zoom=list', label: 'No assigned action sets', field: 'member_count' },
		{ name: 'compliance · no rules assigned', path: '/compliance-policies?zoom=list', label: 'No rules assigned', field: 'rule_count' },
	];

	for (const c of cases) {
		test(`${c.name} → tagFilters.${c.field} = "0"`, async ({ page }) => {
			await preparePage(page, 'light');
			const search = recordRpc<SearchReq>(page, 'Search');
			await gotoAndSettle(page, c.path, '[data-testid="row-list-row"]');
			await page.getByText(c.label).click();
			await expect
				.poll(() => lastSearch(search).tagFilters?.[c.field], { timeout: 5000 })
				.toBe('0');
		});
	}
});

test.describe('list filters', () => {
	test('actions · "not in action set" → tagFilters.assigned = "false"', async ({ page }) => {
		await preparePage(page, 'light');
		const search = recordRpc<SearchReq>(page, 'Search');
		await gotoAndSettle(page, '/actions?zoom=list', '[data-testid="row-list-row"]');
		await page.getByText('Not in action set').click();
		await expect
			.poll(() => lastSearch(search).tagFilters?.assigned, { timeout: 5000 })
			.toBe('false');
	});
});

test.describe('list pagination', () => {
	test('the next-page control advances the page token (offset)', async ({ page }) => {
		await preparePage(page, 'light');

		await page.route('**/cadestro.v1.ControlService/Search', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					results: Array.from({ length: 25 }, (_, i) => ({
						id: wrappedID(`01J6XYZSHOWCASEDEVICE${String(i).padStart(4, '0')}`),
						name: `host-${i}`,
						scope: 5,
						fields: { hostname: `host-${i}`, last_seen_at: '0', registered_at: '0', compliance_status: '1' },
					})),
					nextPageToken: '',
					totalCount: 200,
				}),
			});
		});
		const search = recordRpc<SearchReq>(page, 'Search');
		await gotoAndSettle(page, '/actions?zoom=list', '[data-testid="row-list-row"]');

		await page.getByText(/Page 1 of/).locator('..').getByRole('button').last().click();
		await expect.poll(() => lastSearch(search).pageToken, { timeout: 5000 }).toBe('25');
	});
});

test.describe('list navigation', () => {
	test('clicking a row opens its detail page', async ({ page }: { page: Page }) => {
		await preparePage(page, 'light');
		await gotoAndSettle(page, '/action-sets?zoom=list', '[data-testid="row-list-row"]');

		await page.getByTestId('row-list-link').first().click();
		await expect(page).toHaveURL(/\/action-sets\/01J6XYZSHOWCASESET00001/);
	});
});
