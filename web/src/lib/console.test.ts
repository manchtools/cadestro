import { describe, expect, it } from 'vitest';
import type { Role } from '$contract/cadestro/v1/control_pb';
import { Permission } from '$contract/cadestro/v1/control_pb';
import { collectPages, roleChanges } from './console';

describe('collectPages', () => {
	it('collects every page in order using opaque cursors', async () => {
		const tokens: string[] = [];
		const items = await collectPages(async (pageToken) => {
			tokens.push(pageToken);
			if (!pageToken) return { items: ['first'], nextPageToken: 'opaque/one' };
			return { items: ['second'], nextPageToken: '' };
		});

		expect(tokens).toEqual(['', 'opaque/one']);
		expect(items).toEqual(['first', 'second']);
	});

	it('rejects a repeated cursor instead of loading forever', async () => {
		await expect(collectPages(async () => ({ items: [], nextPageToken: 'same' }))).rejects.toThrow('repeated page cursor');
	});
});

describe('roleChanges', () => {
	const role = {
		name: 'Operators',
		description: 'Original',
		permissions: [Permission.LIST_DEVICES]
	} as Role;

	it('computes named mutations from the latest server baseline', () => {
		expect(roleChanges(role, 'Fleet operators', 'Updated', [Permission.LIST_DEVICES, Permission.GET_DEVICE])).toEqual({
			rename: true,
			describe: true,
			grant: [Permission.GET_DEVICE],
			revoke: []
		});
	});

	it('skips mutations already applied before a partial failure', () => {
		const refreshed = {
			...role,
			name: 'Fleet operators',
			permissions: [Permission.LIST_DEVICES, Permission.GET_DEVICE]
		} as Role;

		expect(roleChanges(refreshed, 'Fleet operators', 'Updated', [Permission.LIST_DEVICES, Permission.GET_DEVICE])).toEqual({
			rename: false,
			describe: true,
			grant: [],
			revoke: []
		});
	});
});
