

export interface CarriedSelection {
	deviceIds: string[];

	label: string;
}

let carried = $state<CarriedSelection | null>(null);

export function setCarried(sel: CarriedSelection | null) {
	carried = sel;
}

export function getCarried(): CarriedSelection | null {
	return carried;
}
