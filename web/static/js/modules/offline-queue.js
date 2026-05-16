/**
 * Offline Compose Queue — IndexedDB-backed queue for offline posts.
 *
 * When publish fails due to network error, the post is queued.
 * When connectivity returns, the queue is drained automatically.
 * CSRF token is read fresh from <meta> on each drain attempt.
 *
 * DB name is per-blog via blog-storage.js. Legacy 'markgo' DB (v3.8) is
 * drained into the new name on first access; see migrateLegacyDB().
 */

import { authenticatedFetch, getCSRFToken } from './auth-fetch.js';
import { dbName, key } from './blog-storage.js';

const DB_NAME = dbName;
const STORE_NAME = 'compose-queue';
const DB_VERSION = 1;
const LEGACY_DB_NAME = 'markgo';
const IDB_MIGRATED_FLAG = key('idb-migrated-v1');

let migrationPromise = null;

function openNamed(name) {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open(name, DB_VERSION);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains(STORE_NAME)) {
                db.createObjectStore(STORE_NAME, { keyPath: 'id', autoIncrement: true });
            }
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
    });
}

// migrateLegacyDB drains the v3.8 'markgo' DB into the per-blog DB once.
// Runs before the first openDB() call. Multi-tab edge: if tab A migrates
// while tab B holds the legacy DB open, deleteDatabase blocks; we resolve
// anyway and the flag is set, so the next session converges. Worst case is
// a queued post lingering until tab B reloads.
async function migrateLegacyDB() {
    if (DB_NAME === LEGACY_DB_NAME) return; // no-op when fallback NS in use
    try {
        if (localStorage.getItem(IDB_MIGRATED_FLAG)) return;
    } catch { return; }

    let legacy = null;
    try {
        legacy = await openNamed(LEGACY_DB_NAME);
        const items = legacy.objectStoreNames.contains(STORE_NAME)
            ? await new Promise((res, rej) => {
                const tx = legacy.transaction(STORE_NAME, 'readonly');
                const req = tx.objectStore(STORE_NAME).getAll();
                req.onsuccess = () => res(req.result);
                req.onerror = () => rej(req.error);
            })
            : [];
        legacy.close();
        legacy = null;

        if (items.length > 0) {
            const target = await openNamed(DB_NAME);
            await new Promise((res, rej) => {
                const tx = target.transaction(STORE_NAME, 'readwrite');
                const store = tx.objectStore(STORE_NAME);
                for (const it of items) {
                    const { id: _drop, ...rest } = it; // let target assign new ids
                    store.add(rest);
                }
                tx.oncomplete = () => { target.close(); res(); };
                tx.onerror = () => { target.close(); rej(tx.error); };
            });
        }

        await new Promise((res) => {
            const req = indexedDB.deleteDatabase(LEGACY_DB_NAME);
            req.onsuccess = req.onerror = req.onblocked = () => res();
        });
    } catch (err) {
        console.warn('IndexedDB legacy migration failed; will retry next session:', err?.message || err);
        if (legacy) try { legacy.close(); } catch { /* ignore */ }
        return; // do not set flag; retry next session
    }

    try { localStorage.setItem(IDB_MIGRATED_FLAG, '1'); } catch { /* ignore */ }
}

async function openDB() {
    if (!migrationPromise) migrationPromise = migrateLegacyDB();
    await migrationPromise;
    return openNamed(DB_NAME);
}

/**
 * Add a compose input to the offline queue.
 * @param {{ content: string, title?: string }} input
 */
export async function queuePost(input) {
    const db = await openDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, 'readwrite');
        tx.objectStore(STORE_NAME).add({ ...input, queuedAt: Date.now() });
        tx.oncomplete = () => { db.close(); resolve(); };
        tx.onerror = () => { db.close(); reject(tx.error); };
    });
}

/**
 * Get number of queued posts.
 */
export async function getQueueCount() {
    const db = await openDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, 'readonly');
        const req = tx.objectStore(STORE_NAME).count();
        req.onsuccess = () => { db.close(); resolve(req.result); };
        req.onerror = () => { db.close(); reject(req.error); };
    });
}

/**
 * Drain the queue: POST each item to /compose/quick.
 * Returns { published: number, failed: number }.
 * failed === -1 signals missing CSRF token (caller should warn user).
 */
export async function drainQueue() {
    const token = getCSRFToken();
    if (!token) {
        console.warn('drainQueue: no CSRF token available, cannot sync');
        return { published: 0, failed: -1 };
    }

    let db;
    try {
        db = await openDB();
    } catch (err) {
        console.warn('drainQueue: IndexedDB unavailable:', err?.message || err);
        return { published: 0, failed: 0 };
    }

    try {
        const items = await new Promise((resolve, reject) => {
            const tx = db.transaction(STORE_NAME, 'readonly');
            const req = tx.objectStore(STORE_NAME).getAll();
            req.onsuccess = () => resolve(req.result);
            req.onerror = () => reject(req.error);
        });

        if (items.length === 0) return { published: 0, failed: 0 };

        let published = 0;
        let failed = 0;

        for (const item of items) {
            const body = { content: item.content };
            if (item.title) body.title = item.title;

            try {
                const response = await authenticatedFetch('/compose/quick', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });

                if (response.ok) {
                    // Remove from queue on success
                    await new Promise((resolve, reject) => {
                        const tx = db.transaction(STORE_NAME, 'readwrite');
                        tx.objectStore(STORE_NAME).delete(item.id);
                        tx.oncomplete = () => resolve();
                        tx.onerror = () => reject(tx.error);
                    });
                    published++;
                } else if (response.status === 401 || response.status === 403) {
                    // Auth/CSRF expired — stop draining, keep items for retry after login
                    failed += items.length - published;
                    break;
                } else {
                    failed++;
                }
            } catch (err) {
                // Network error or unexpected failure — stop draining
                console.warn('Queue drain stopped:', err?.message || err);
                failed += items.length - published;
                break;
            }
        }

        return { published, failed };
    } finally {
        if (db) db.close();
    }
}
