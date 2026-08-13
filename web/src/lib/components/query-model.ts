// The dynamic-group query model behind the chip editor.
//
// The chips are only a VIEW: the string in this file's `compileQuery` output is
// exactly what ValidateDynamicQuery / UpdateDeviceGroupQuery (and the user-group
// twins) receive, so parse ∘ compile has to round-trip without rewriting an
// operator, a quote style or an `in (…)` list. Everything here is pure so the
// mapping is unit-testable without a DOM.
//
// Supported shape: a flat sequence of conditions and single-level parenthesised
// groups joined by AND/OR — `a == x AND (b == y OR c == z) AND d != w`, the
// concept's own example. Nested groups and NOT are deliberately NOT modelled;
// `parseQuery` returns null for them and the editor falls back to raw text so a
// query it cannot draw is never silently rewritten.

export type Join = 'AND' | 'OR';

/** Sentinel property for "some label I type in", not a real query key. */
export const LABEL_CUSTOM = '__label_custom__';

/** Operators that take no value. */
export const UNARY_OPERATORS = ['exists', 'notExists'];

/** Operators whose value is a parenthesised list, passed through verbatim. */
export const LIST_OPERATORS = ['in', 'notIn'];

export type Cond = {
	property: string;
	/** Only meaningful when `property === LABEL_CUSTOM`. */
	labelKey: string;
	operator: string;
	value: string;
};

export type QueryNode =
	| { kind: 'cond'; cond: Cond }
	| { kind: 'group'; join: Join; conds: Cond[] };

export interface QueryModel {
	nodes: QueryNode[];
	/** `joins[i]` sits between `nodes[i]` and `nodes[i + 1]`. */
	joins: Join[];
}

export function emptyCond(): Cond {
	return { property: '', labelKey: '', operator: 'equals', value: '' };
}

export function emptyModel(): QueryModel {
	return { nodes: [{ kind: 'cond', cond: emptyCond() }], joins: [] };
}

export function isUnary(operator: string): boolean {
	return UNARY_OPERATORS.includes(operator);
}

function isListValue(value: string): boolean {
	const v = value.trim();
	return v.startsWith('(') && v.endsWith(')');
}

/** The property as the server sees it — the label sentinel expands here. */
export function resolveProperty(c: Cond): string {
	if (c.property === LABEL_CUSTOM) {
		return c.labelKey ? `device.labels.${c.labelKey}` : '';
	}
	return c.property;
}

/** A condition the operator has not finished filling in cannot be compiled. */
export function isComplete(c: Cond): boolean {
	if (!resolveProperty(c) || !c.operator) return false;
	if (isUnary(c.operator)) return true;
	return c.value.trim().length > 0;
}

/** The untouched builder: exactly one bare condition still equal to the seed,
 *  and no joins. This state is LEGAL — it compiles to '' and the server parses
 *  the empty query as the always-true rule ("matches all") — so it is the one
 *  unfilled state that does not count as half-typed. Anything touched (a
 *  property, a changed operator, a group, a second node) leaves it. */
export function modelIsEmpty(model: QueryModel): boolean {
	if (model.joins.length !== 0 || model.nodes.length !== 1) return false;
	const node = model.nodes[0];
	if (node.kind !== 'cond') return false;
	const seed = emptyCond();
	return (
		node.cond.property === seed.property &&
		node.cond.labelKey === seed.labelKey &&
		node.cond.operator === seed.operator &&
		node.cond.value === seed.value
	);
}

export function modelComplete(model: QueryModel): boolean {
	// The pristine seed is not "half-typed" — it is the legal match-all rule.
	// Only this exact state passes; any partially filled condition stays
	// incomplete so a narrower rule can never be saved by accident.
	if (modelIsEmpty(model)) return true;
	return model.nodes.every((n) =>
		n.kind === 'cond' ? isComplete(n.cond) : n.conds.length > 0 && n.conds.every(isComplete)
	);
}

export function compileCond(c: Cond): string {
	const prop = resolveProperty(c);
	if (!prop || !c.operator) return '';
	if (isUnary(c.operator)) return `${prop} ${c.operator}`;
	// An `in (…)` list is already query syntax; quoting it would change the query.
	if (LIST_OPERATORS.includes(c.operator) && isListValue(c.value)) {
		return `${prop} ${c.operator} ${c.value.trim()}`;
	}
	const quote = c.value.includes('"') ? "'" : '"';
	return `${prop} ${c.operator} ${quote}${c.value}${quote}`;
}

function compileNode(n: QueryNode): string {
	if (n.kind === 'cond') return compileCond(n.cond);
	const parts = n.conds.map(compileCond).filter(Boolean);
	if (parts.length === 0) return '';
	if (parts.length === 1) return parts[0];
	return `(${parts.join(` ${n.join} `)})`;
}

/** The exact string handed to the RPC. Incomplete conditions drop out — the
 *  editor blocks the commit in that state, so a half-typed chip can never be
 *  saved as a silently narrower rule. */
export function compileQuery(model: QueryModel): string {
	const out: string[] = [];
	for (let i = 0; i < model.nodes.length; i++) {
		const text = compileNode(model.nodes[i]);
		if (!text) continue;
		if (out.length > 0) out.push(model.joins[i - 1] ?? 'AND');
		out.push(text);
	}
	return out.join(' ');
}

// ── parsing ──────────────────────────────────────────────────────────────────

interface SplitResult {
	parts: string[];
	joins: Join[];
}

/** Split on AND/OR that sit at paren depth 0 and outside quotes. */
function splitTopLevel(text: string): SplitResult | null {
	const parts: string[] = [];
	const joins: Join[] = [];
	let depth = 0;
	let quote: string | null = null;
	let start = 0;
	let i = 0;

	while (i < text.length) {
		const ch = text[i];
		if (quote) {
			if (ch === quote) quote = null;
			i++;
			continue;
		}
		if (ch === '"' || ch === "'") {
			quote = ch;
			i++;
			continue;
		}
		if (ch === '(') {
			depth++;
			i++;
			continue;
		}
		if (ch === ')') {
			depth--;
			if (depth < 0) return null;
			i++;
			continue;
		}
		if (depth === 0 && /\s/.test(ch)) {
			const rest = text.slice(i + 1);
			const kw = /^(AND|OR|NOT)\b/i.exec(rest);
			if (kw && /\s|$/.test(rest[kw[0].length] ?? ' ')) {
				const word = kw[1].toUpperCase();
				if (word === 'NOT') return null; // not drawable as chips
				parts.push(text.slice(start, i));
				joins.push(word as Join);
				i = i + 1 + kw[0].length;
				start = i;
				continue;
			}
		}
		i++;
	}
	if (depth !== 0 || quote) return null;
	parts.push(text.slice(start));
	return { parts: parts.map((p) => p.trim()), joins };
}

/** True when the part is one balanced parenthesised expression, `(…)`. */
function isWrappedGroup(part: string): boolean {
	if (!part.startsWith('(') || !part.endsWith(')')) return false;
	let depth = 0;
	let quote: string | null = null;
	for (let i = 0; i < part.length; i++) {
		const ch = part[i];
		if (quote) {
			if (ch === quote) quote = null;
			continue;
		}
		if (ch === '"' || ch === "'") quote = ch;
		else if (ch === '(') depth++;
		else if (ch === ')') {
			depth--;
			if (depth === 0 && i < part.length - 1) return false;
		}
	}
	return depth === 0;
}

export interface ParseOptions {
	/** When the property palette offers custom labels, `device.labels.x` folds
	 *  back into the label sentinel + key so the chip renders as a label chip. */
	hasCustomLabels?: boolean;
}

function condFromParts(prop: string, op: string, value: string, opts: ParseOptions): Cond {
	if (opts.hasCustomLabels) {
		const label = /^(?:device\.)?labels\.(.+)$/.exec(prop);
		if (label) return { property: LABEL_CUSTOM, labelKey: label[1], operator: op, value };
	}
	return { property: prop, labelKey: '', operator: op, value };
}

export function parseCondition(token: string, opts: ParseOptions = {}): Cond | null {
	const t = token.trim();
	if (!t) return null;

	const unary = /^(\S+)\s+(exists|notExists)$/i.exec(t);
	if (unary) {
		const op = UNARY_OPERATORS.find((o) => o.toLowerCase() === unary[2].toLowerCase())!;
		return condFromParts(unary[1], op, '', opts);
	}

	// `in`/`notIn` lists keep their parentheses so compile re-emits them verbatim.
	const list = /^(\S+)\s+(in|notIn)\s+(\(.*\))$/i.exec(t);
	if (list) {
		const op = LIST_OPERATORS.find((o) => o.toLowerCase() === list[2].toLowerCase())!;
		return condFromParts(list[1], op, list[3], opts);
	}

	const quoted = /^(\S+)\s+(\S+)\s+(?:"([^"]*)"|'([^']*)')$/.exec(t);
	if (quoted) {
		return condFromParts(quoted[1], quoted[2], quoted[3] ?? quoted[4] ?? '', opts);
	}

	return null;
}

/** Parse a stored query into chips. `null` means "the chips cannot draw this" —
 *  the caller must keep editing the raw text instead of rewriting the query. */
export function parseQuery(text: string, opts: ParseOptions = {}): QueryModel | null {
	const trimmed = text.trim();
	if (!trimmed) return emptyModel();

	const top = splitTopLevel(trimmed);
	if (!top) return null;

	const nodes: QueryNode[] = [];
	for (const part of top.parts) {
		if (isWrappedGroup(part)) {
			const inner = splitTopLevel(part.slice(1, -1).trim());
			if (!inner) return null;
			// One conjunction per group keeps the chiplet inside the dashed
			// parentheses unambiguous; a mixed group is not drawable.
			const uniform = inner.joins.every((j) => j === inner.joins[0]);
			if (!uniform) return null;
			const conds: Cond[] = [];
			for (const sub of inner.parts) {
				const c = parseCondition(sub, opts);
				if (!c) return null;
				conds.push(c);
			}
			if (conds.length === 0) return null;
			if (conds.length === 1) nodes.push({ kind: 'cond', cond: conds[0] });
			else nodes.push({ kind: 'group', join: inner.joins[0] ?? 'AND', conds });
			continue;
		}
		const c = parseCondition(part, opts);
		if (!c) return null;
		nodes.push({ kind: 'cond', cond: c });
	}

	if (nodes.length === 0) return null;
	return { nodes, joins: top.joins };
}
