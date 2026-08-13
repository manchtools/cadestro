// Carried selection: devices picked on the fleet surface ride the pill into
// the assign surface (design draft B2 — "the pill carried it here").
// Written by the fleet page, consumed by /assign; survives navigation like
// all shell state. Holds only ids + a display label — never RPC data.
export interface CarriedSelection {
	deviceIds: string[];
	/** Short display label for pill/stage surfaces, e.g. "12 devices". */
	label: string;
}

let carried = $state<CarriedSelection | null>(null);

export function setCarried(sel: CarriedSelection | null) {
	carried = sel;
}

export function getCarried(): CarriedSelection | null {
	return carried;
}
