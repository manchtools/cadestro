import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import InventoryIntervalDialog from './inventory-interval-dialog.svelte';

function trigger(): HTMLButtonElement {
	const element = document.querySelector<HTMLButtonElement>('[data-slot="select-trigger"]');
	if (!element) throw new Error('the inventory interval trigger is missing');
	return element;
}

describe('inventory interval dialog seeding', () => {
	it('seeds on reopen, not when the source value changes while open', async () => {
		const view = await render(InventoryIntervalDialog, {
			props: {
				open: true,
				currentMinutes: 60,
				title: 'Inventory interval',
				description: 'Choose an interval',
				onsave: vi.fn()
			}
		});

		await vi.waitFor(() => expect(trigger().textContent).toContain('1 h'));
		trigger().click();
		await vi.waitFor(() => expect(document.querySelector('[data-slot="select-item"]')).not.toBeNull());
		document.querySelector<HTMLElement>('[data-slot="select-item"][data-value="120"]')?.click();
		await vi.waitFor(() => expect(trigger().textContent).toContain('2 h'));

		await view.rerender({ open: true, currentMinutes: 360 });
		await vi.waitFor(() => expect(trigger().textContent).toContain('2 h'));

		await view.rerender({ open: false, currentMinutes: 360 });
		await view.rerender({ open: true, currentMinutes: 360 });
		await vi.waitFor(() => expect(trigger().textContent).toContain('6 h'));
	});
});
