export async function addMemberships(ids: string[], add: (id: string) => Promise<unknown>, reload: () => Promise<string[]>): Promise<{ remaining: string[]; ready: boolean; error: unknown | null }> {
 let remaining = [...ids];
 try {
  for (const id of ids) {
   await add(id);
   remaining = remaining.filter(value => value !== id);
  }
  return { remaining, ready: true, error: null };
 } catch (error) {
  try {
   const assigned = new Set(await reload());
   return { remaining: remaining.filter(id => !assigned.has(id)), ready: true, error };
  } catch {
   return { remaining, ready: false, error };
  }
 }
}
