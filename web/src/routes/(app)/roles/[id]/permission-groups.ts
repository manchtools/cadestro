import { Permission } from '$contract/cadestro/v1/control_pb';
export const availablePermissions = Object.values(Permission).filter((value): value is Permission => typeof value === 'number' && value !== Permission.UNSPECIFIED);
export function groupPermissions(permissions: Permission[]) {
 const groups = new Map<string, Permission[]>();
 for (const permission of permissions) {
  const name = Permission[permission].replace(/^(GET|LIST|CREATE|UPDATE|DELETE|RENAME|REVOKE|ASSIGN|ADD|REMOVE)_/, '').replace(/_TO_.*|_FROM_.*|_FOR_.*/, '');
  const group = groups.get(name) ?? []; group.push(permission); groups.set(name, group);
 }
 return [...groups].map(([name, permissions]) => ({ name: name.replaceAll('_', ' '), permissions }));
}
