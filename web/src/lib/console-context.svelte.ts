import { createContext } from 'svelte';
import type { Permission, User } from '$contract/cadestro/v1/control_pb';
export const [consoleContext, setConsoleContext] = createContext<{ can: (permission: Permission) => boolean; currentUser: () => User | undefined }>();
