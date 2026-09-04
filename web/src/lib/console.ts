import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';
import type { Permission, Role } from '$contract/cadestro/v1/control_pb';

export type PageResult<T> = {
	items: T[];
	nextPageToken: string;
};

export async function collectPages<T>(loadPage: (pageToken: string) => Promise<PageResult<T>>): Promise<T[]> {
	const items: T[] = [];
	const seen = new Set<string>();
	let pageToken = '';
	do {
		if (seen.has(pageToken)) throw new Error('The server returned a repeated page cursor');
		seen.add(pageToken);
		const page = await loadPage(pageToken);
		items.push(...page.items);
		pageToken = page.nextPageToken;
	} while (pageToken);
	return items;
}

export function roleChanges(role: Role, name: string, description: string, permissions: Permission[]) {
	return {
		rename: role.name !== name,
		describe: role.description !== description,
		grant: permissions.filter((permission) => !role.permissions.includes(permission)),
		revoke: role.permissions.filter((permission) => !permissions.includes(permission))
	};
}

export function cursorHref(url: URL, key: string, value: string, clear: string[] = []): string {
	const target = new URL(url);
	for (const name of clear) target.searchParams.delete(name);
	if (value) target.searchParams.set(key, value);
	else target.searchParams.delete(key);
	return `${target.pathname}${target.search}`;
}

export function formatDate(value: Timestamp | undefined): string {
	return value ? timestampDate(value).toLocaleString() : 'Never';
}
