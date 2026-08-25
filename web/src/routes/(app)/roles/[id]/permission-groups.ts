import type { PermissionInfo } from '$lib/sdk';

export interface PermissionGroup {
	name: string;
	permissions: PermissionInfo[];
}

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
