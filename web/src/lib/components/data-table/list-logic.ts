export type SortDir = 'asc' | 'desc';
export type SortState<K extends string = string> = { key: K; dir: SortDir };

export function nextSort<K extends string>(
	current: SortState<K>,
	clicked: K,
	defaultDir: (key: K) => SortDir = () => 'asc'
): SortState<K> {
	if (current.key === clicked) {
		return { key: clicked, dir: current.dir === 'asc' ? 'desc' : 'asc' };
	}
	return { key: clicked, dir: defaultDir(clicked) };
}

export interface PageMath {
	totalPages: number;
	clampedPage: number;
	offset: number;
	showingFrom: number;
	showingTo: number;
}

export function pageMath(total: number, page: number, pageSize: number): PageMath {
	const totalPages = Math.max(1, Math.ceil(total / pageSize));
	const clampedPage = Math.min(Math.max(1, page), totalPages);
	const offset = (clampedPage - 1) * pageSize;
	const showingFrom = total === 0 ? 0 : offset + 1;
	const showingTo = Math.min(clampedPage * pageSize, total);
	return { totalPages, clampedPage, offset, showingFrom, showingTo };
}
