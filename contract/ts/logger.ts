

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

const LEVEL_ORDER: Record<LogLevel, number> = {
	debug: 10,
	info: 20,
	warn: 30,
	error: 40,
};

export interface LogEvent {

	level: LogLevel;

	name: string;

	message: string;

	context?: Record<string, unknown>;

	timestamp: string;
}

export type LogSink = (event: LogEvent) => void;

function consoleSink(e: LogEvent): void {
	const prefix = `[${e.timestamp}] ${e.level.toUpperCase()} ${e.name}:`;
	switch (e.level) {
		case 'debug':
		case 'info':

			console.log(prefix, e.message, e.context ?? '');
			return;
		case 'warn':

			console.warn(prefix, e.message, e.context ?? '');
			return;
		case 'error':

			console.error(prefix, e.message, e.context ?? '');
			return;
	}
}

let currentSink: LogSink = consoleSink;
let currentMinLevel: LogLevel = 'info';

export function setLogSink(sink: LogSink): LogSink {
	const previous = currentSink;
	currentSink = sink;
	return previous;
}

export function resetLogSink(): void {
	currentSink = consoleSink;
	currentMinLevel = 'info';
}

export function setLogLevel(level: LogLevel): void {
	currentMinLevel = level;
}

function emit(name: string, level: LogLevel, message: string, context?: Record<string, unknown>): void {
	if (LEVEL_ORDER[level] < LEVEL_ORDER[currentMinLevel]) return;
	currentSink({
		level,
		name,
		message,
		context,
		timestamp: new Date().toISOString(),
	});
}

export interface Logger {
	name: string;
	debug(message: string, context?: Record<string, unknown>): void;
	info(message: string, context?: Record<string, unknown>): void;
	warn(message: string, context?: Record<string, unknown>): void;
	error(message: string, context?: Record<string, unknown>): void;

	named(child: string): Logger;
}

function makeLogger(name: string): Logger {
	return {
		name,
		debug: (m, c) => emit(name, 'debug', m, c),
		info: (m, c) => emit(name, 'info', m, c),
		warn: (m, c) => emit(name, 'warn', m, c),
		error: (m, c) => emit(name, 'error', m, c),
		named: (child) => makeLogger(`${name}.${child}`),
	};
}

export const logger: Logger = makeLogger('cadestro-contract');

export function describeError(err: unknown): Record<string, unknown> {
	if (err instanceof Error) {
		return { name: err.name, message: err.message, stack: err.stack };
	}
	if (typeof err === 'object' && err !== null) {
		return { value: err as Record<string, unknown> };
	}
	return { value: String(err) };
}
