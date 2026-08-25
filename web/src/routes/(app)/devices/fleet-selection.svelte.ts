

let surface = $state('');
let ids = $state<string[]>([]);

export function getFleetSelection(forSurface: string): string[] {
	return surface === forSurface ? ids : [];
}

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

export function resetFleetSelection() {
	surface = '';
	ids = [];
}
