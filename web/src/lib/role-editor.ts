import { api } from '$lib/api';
import { collectPages, roleChanges } from '$lib/console';
import { Permission, type Role } from '$contract/cadestro/v1/control_pb';

export async function readRole(id: string, can: (permission: Permission) => boolean): Promise<Role> {
 const role = can(Permission.GET_ROLE) ? (await api.getRole({ id: { value: id } })).role : can(Permission.LIST_ROLES) ? (await collectPages(async pageToken => {
  const response = await api.listRoles({ pageSize: 100, pageToken });
  return { items: response.roles, nextPageToken: response.nextPageToken };
 })).find(role => role.id?.value === id) : undefined;
 if (!role) throw new Error('The latest role could not be loaded');
 return role;
}

export async function saveRoleEdits(role: Role, name: string, description: string, permissions: Permission[], can: (permission: Permission) => boolean): Promise<{ role: Role; error: null } | { role: Role | null; error: unknown }> {
 const id = role.id;
 const changes = roleChanges(role, name, description, permissions);
 let updated = role;
 try {
  if (changes.rename) updated = (await api.renameRole({ id, name })).role ?? updated;
  if (changes.describe) updated = (await api.setRoleDescription({ id, description })).role ?? updated;
  for (const permission of changes.grant) updated = (await api.grantRolePermission({ id, permission })).role ?? updated;
  for (const permission of changes.revoke) updated = (await api.revokeRolePermission({ id, permission })).role ?? updated;
  return { role: updated, error: null };
 } catch (error) {
  try { return { role: await readRole(id?.value ?? '', can), error }; }
  catch { return { role: null, error }; }
 }
}
