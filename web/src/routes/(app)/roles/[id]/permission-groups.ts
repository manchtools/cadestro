import type { PermissionInfo } from '$lib/sdk';

export interface PermissionGroup {
	name: string;
	permissions: PermissionInfo[];
}

// Domain grouping for the permission matrix, DISCOVERED from the permission
// list ListPermissions returns — never a hardcoded domain table, so a
// permission added server-side shows up without a web change. The server
// labels each permission with a UI group; when it sends none, the key's own
// prefix (the part before the ':' qualifier) stands in, so a row can never
// fall out of the matrix. Groups keep first-seen order, which is the server
// registry's own order.
export function groupPermissions(permissions: PermissionInfo[]): PermissionGroup[] {
	const by = new Map<string, PermissionInfo[]>();
	for (const p of permissions) {
		const name = p.group.trim() || p.key.split(':')[0] || p.key;
		const bucket = by.get(name);
		if (bucket) bucket.push(p);
		else by.set(name, [p]);
	}
	return [...by.entries()].map(([name, perms]) => ({ name, permissions: perms }));
}
