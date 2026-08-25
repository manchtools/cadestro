

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ActionScheduleSchema } from '$contract/cadestro/v1/actions_pb';
import * as m from '$lib/paraglide/messages';

import ContainerScheduleCard from './container-schedule-card.svelte';

const onsave = vi.fn<(next: unknown) => Promise<void>>();

function storedSchedule() {
	return create(ActionScheduleSchema, {
		cron: '0 3 * * *',
		intervalHours: 8,
		runOnAssign: true,
		skipIfUnchanged: true
	});
}

function footerButtons(): string[] {
	return [
		...document.querySelectorAll<HTMLElement>('[data-slot="dialog-footer"] button')
	].map((b) => b.textContent?.trim() ?? '');
}

function cronInput(): HTMLInputElement {
	const el = document.querySelector<HTMLInputElement>('input#scheduleCron');
	if (!el) throw new Error('the schedule dialog never rendered its cron field');
	return el;
}

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

function doneButton(): HTMLButtonElement {
	const found = [
		...document.querySelectorAll<HTMLButtonElement>('[data-slot="dialog-footer"] button')
	].find((b) => b.textContent?.trim() === m.common_done());
	if (!found) throw new Error('the schedule dialog has no Done button');
	return found;
}

function mountOpen() {
	return render(ContainerScheduleCard, {
		props: { schedule: storedSchedule(), editOpen: true, onsave }
	});
}

beforeEach(() => {
	vi.clearAllMocks();
	onsave.mockResolvedValue(undefined);
});

describe('the container schedule dialog offers ONE way out', () => {
	it('dismisses with a single Done — never a competing Cancel/Save pair', async () => {
		mountOpen();
		await vi.waitFor(() => expect(document.querySelector('[data-slot="dialog-footer"]')).not.toBeNull());

		expect(footerButtons()).toEqual([m.common_done()]);

		expect(footerButtons()).not.toContain(m.common_save());
		expect(footerButtons()).not.toContain(m.common_cancel());

		expect(document.body.textContent).toContain(m.container_schedule_saved_on_close());
	});

	it('states the schedule, and the statement itself is the way in', async () => {
		render(ContainerScheduleCard, { props: { schedule: storedSchedule(), onsave } });

		await vi.waitFor(() =>
			expect(document.body.textContent).toContain(m.container_schedule_title())
		);

		expect(document.querySelector('[data-slot="dialog-footer"]')).toBeNull();
		const triggers = [...document.querySelectorAll('button')];
		expect(triggers.map((b) => b.getAttribute('data-testid'))).toEqual([
			'schedule-summary-edit'
		]);

		triggers[0].click();
		await vi.waitFor(() =>
			expect(document.querySelector('[data-slot="dialog-footer"]')).not.toBeNull()
		);
		expect(footerButtons()).toEqual([m.common_done()]);
	});
});

describe('dismiss is the commit', () => {
	it('writes the edited schedule once, through the page’s existing RPC', async () => {
		mountOpen();
		await vi.waitFor(() => expect(cronInput().value).toBe('0 3 * * *'));

		type(cronInput(), '0 */8 * * *');
		doneButton().click();

		await vi.waitFor(() => expect(onsave).toHaveBeenCalledTimes(1));
		expect(onsave.mock.calls[0][0]).toMatchObject({ cron: '0 */8 * * *' });

		await vi.waitFor(() => expect(document.querySelector('[data-slot="dialog-footer"]')).toBeNull());
	});

	it('writes nothing when the operator only looked', async () => {
		mountOpen();
		await vi.waitFor(() => expect(cronInput().value).toBe('0 3 * * *'));

		doneButton().click();

		await vi.waitFor(() => expect(document.querySelector('[data-slot="dialog-footer"]')).toBeNull());

		expect(onsave).not.toHaveBeenCalled();
	});

	it('commits an edit dismissed with Esc too — one path, not two', async () => {
		mountOpen();
		await vi.waitFor(() => expect(cronInput().value).toBe('0 3 * * *'));

		type(cronInput(), '0 0 * * 0');

		document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

		await vi.waitFor(() => expect(document.querySelector('[data-slot="dialog-footer"]')).toBeNull());
		await vi.waitFor(() => expect(onsave).toHaveBeenCalledTimes(1));
		expect(onsave.mock.calls[0][0]).toMatchObject({ cron: '0 0 * * 0' });
	});
});
