

import { openDB, type IDBPDatabase } from 'idb';
import { logger, describeError } from './logger.js';

const log = logger.named('offline');

const DB_NAME = 'cadestro-offline';
const DB_VERSION = 1;

interface OfflineDB {
	drafts: {
		key: string;
		value: {
			id: string;
			type: string;
			data: unknown;
			updatedAt: Date;
		};
	};
}

let dbPromise: Promise<IDBPDatabase<OfflineDB>> | null = null;

function getDB(): Promise<IDBPDatabase<OfflineDB>> {
	if (typeof indexedDB === 'undefined') {
		return Promise.reject(new Error('IndexedDB not available'));
	}

	if (!dbPromise) {
		dbPromise = openDB<OfflineDB>(DB_NAME, DB_VERSION, {
			upgrade(db) {
				if (!db.objectStoreNames.contains('drafts')) {
					db.createObjectStore('drafts', { keyPath: 'id' });
				}
			}
		});
	}
	return dbPromise;
}

export type DraftType =
	| 'create-token'
	| 'create-definition';

export class OfflineStore {
	private drafts: Map<string, unknown> = new Map();
	private loaded = false;
	private listeners = new Set<() => void>();

	constructor() {
		if (typeof window !== 'undefined') {
			this.loadDrafts();
		}
	}

	private notify() {
		for (const fn of this.listeners) fn();
	}

	onChange(listener: () => void): () => void {
		this.listeners.add(listener);
		return () => this.listeners.delete(listener);
	}

	private async loadDrafts() {
		try {
			const db = await getDB();
			const allDrafts = await db.getAll('drafts');
			const map = new Map<string, unknown>();
			for (const draft of allDrafts) {
				map.set(draft.id, draft.data);
			}
			this.drafts = map;
			this.loaded = true;
			this.notify();
		} catch (error) {
			log.error('failed to load drafts', describeError(error));
			this.loaded = true;
			this.notify();
		}
	}

	get isLoaded() {
		return this.loaded;
	}

	getDraft(type: DraftType, id: string = 'default'): unknown | undefined {
		const key = `${type}:${id}`;
		return this.drafts.get(key);
	}

	hasDraft(type: DraftType, id: string = 'default'): boolean {
		const key = `${type}:${id}`;
		return this.drafts.has(key);
	}

	async saveDraft(
		type: DraftType,
		data: unknown,
		id: string = 'default'
	): Promise<void> {
		const key = `${type}:${id}`;

		try {
			const db = await getDB();
			await db.put('drafts', {
				id: key,
				type,
				data,
				updatedAt: new Date()
			});
		} catch (error) {
			log.error('failed to save draft', describeError(error));
			throw error;
		}

		const newMap = new Map(this.drafts);
		newMap.set(key, data);
		this.drafts = newMap;
		this.notify();
	}

	async clearDraft(type: DraftType, id: string = 'default'): Promise<void> {
		const key = `${type}:${id}`;

		try {
			const db = await getDB();
			await db.delete('drafts', key);
		} catch (error) {
			log.error('failed to clear draft', describeError(error));
			throw error;
		}

		const newMap = new Map(this.drafts);
		newMap.delete(key);
		this.drafts = newMap;
		this.notify();
	}

	async clearAllDrafts(): Promise<void> {
		try {
			const db = await getDB();
			await db.clear('drafts');
		} catch (error) {
			log.error('failed to clear all drafts', describeError(error));
			throw error;
		}

		this.drafts = new Map();
		this.notify();
	}

	getDraftsOfType(type: DraftType): Map<string, unknown> {
		const result = new Map<string, unknown>();
		for (const [key, value] of this.drafts.entries()) {
			if (key.startsWith(`${type}:`)) {
				const id = key.slice(type.length + 1);
				result.set(id, value);
			}
		}
		return result;
	}
}
