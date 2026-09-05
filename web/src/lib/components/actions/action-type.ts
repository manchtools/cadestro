import { Package, RefreshCw, Terminal, ShieldCheck } from '@lucide/svelte';
import type { ManagedAction } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';
export const TILE_VALUES = ['PACKAGE', 'UPDATE', 'SHELL', 'COMPLIANCE_CHECK'] as const;
export type ActionChoice = typeof TILE_VALUES[number];
export function getActionTypeInfoByValue(value: string) {
 switch (value.toUpperCase()) {
 case 'PACKAGE': return { icon: Package, label: m.actions_type_package(), description: m.actions_type_package_description() };
 case 'UPDATE': return { icon: RefreshCw, label: m.actions_type_update(), description: m.actions_type_update_description() };
 case 'COMPLIANCE_CHECK': case 'COMPLIANCE': return { icon: ShieldCheck, label: m.actions_type_compliance_check(), description: m.actions_type_compliance_check_description() };
 default: return { icon: Terminal, label: m.actions_type_shell(), description: m.actions_type_shell_description() };
 }
}
export function actionChoice(action: ManagedAction): ActionChoice {
 if (action.params.case === 'package') return 'PACKAGE';
 if (action.params.case === 'update') return 'UPDATE';
 return action.params.case === 'shell' && action.params.value.isCompliance ? 'COMPLIANCE_CHECK' : 'SHELL';
}
