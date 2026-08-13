// Fleet selection lives at module scope, exactly like the pill mode it drives:
// the operator selects on the fleet surface, walks somewhere else, and both the
// pill AND the tile outlines are still there on return. Component-local state
// would lose the outlines on the first unmount.
//
// Keyed by surface so /devices and /my-devices never share a set — they are two
// different fleets and a stale cross-surface selection would carry the wrong
// device ids into /assign.

let surface = $state('');
let ids = $state<string[]>([]);

export function getFleetSelection(forSurface: string): string[] {
	return surface === forSurface ? ids : [];
}

/** Which surface currently owns the selection ('' = nobody). A surface only
 *  tears the pill down when it is the owner, so walking from /devices to
 *  /my-devices does not silently discard the /devices selection. */
export function fleetSelectionSurface(): string {
	return surface;
}

export function setFleetSelection(forSurface: string, next: string[]) {
	surface = forSurface;
	ids = next;
}

export function clearFleetSelection(forSurface: string) {
	if (surface === forSurface) ids = [];
}

/** Test seam. */
export function resetFleetSelection() {
	surface = '';
	ids = [];
}
