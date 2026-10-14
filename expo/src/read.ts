/**
 * Reading the gate over the network (spec/gate-v1.md R9, R21, R24), and
 * when to read it again (R25).
 *
 * Every failure becomes `{ kind: 'unknown' }`: never a stop, and never
 * permission to run either. Only the caller knows what "no answer" is worth
 * (see applyReading).
 */
import type { Install, Reading } from './decide';

/** The contract this library speaks, sent with every read. */
export const CONTRACT = '1';

/** How long a read may take before it counts as no answer (R24). */
export const READ_DEADLINE_MS = 3_000;

/** How long the app must be away before coming back reads the gate again (R25). */
export const RECHECK_AFTER_MS = 5 * 60_000;

/** What a read tells the server: the contract and which build is asking. Nothing about the person (R9). */
export function gateHeaders(install: Install): Record<string, string> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    // A pause read from a stale cache is not a pause.
    'Cache-Control': 'no-cache',
    'X-Portcullis-Contract': CONTRACT,
  };
  if (install.store) headers['X-Portcullis-Store'] = install.store;
  if (install.version) headers['X-Portcullis-Version'] = install.version.slice(0, 32);
  if (install.build) headers['X-Portcullis-Build'] = install.build.slice(0, 16);
  return headers;
}

export interface ReadOptions {
  timeoutMs?: number;
  /** Swapped in tests. */
  fetchImpl?: typeof fetch;
  /** Abort to give up early, such as when the screen reading the gate goes away. */
  signal?: AbortSignal;
}

/** Reads the gate once. Never throws. */
export async function readGate(
  url: string,
  install: Install,
  options: ReadOptions = {}
): Promise<Reading> {
  const timeoutMs = options.timeoutMs ?? READ_DEADLINE_MS;
  const doFetch = options.fetchImpl ?? fetch;
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  // Raced as well as aborted: React Native's fetch doesn't always reject when
  // aborted against a host that never answers, and parsing is inside the race
  // too, so an endless body can't outlast the deadline.
  const deadline = new Promise<Reading>((resolve) => {
    const giveUp = () => {
      controller.abort();
      resolve({ kind: 'unknown' });
    };
    timer = setTimeout(giveUp, timeoutMs);
    if (options.signal?.aborted) giveUp();
    else options.signal?.addEventListener('abort', giveUp, { once: true });
  });
  const attempt = (async (): Promise<Reading> => {
    const res = await doFetch(url, {
      headers: gateHeaders(install),
      // No cookies either way: the gate host must never set one that rides
      // along on the app's other requests.
      credentials: 'omit',
      // Never an old answer from a cache: iOS honours the server's caching
      // headers, and a cached answer would hold a pause after resume.
      cache: 'no-store',
      signal: controller.signal,
    });
    if (res.status !== 200) return { kind: 'unknown' };
    const text = await res.text();
    return { kind: 'answer', body: JSON.parse(text) as unknown };
  })().catch((): Reading => ({ kind: 'unknown' }));
  try {
    return await Promise.race([attempt, deadline]);
  } finally {
    clearTimeout(timer);
  }
}

/**
 * Whether time away should read the gate again (R25). Two clocks, because
 * each can be fooled alone: the wall clock can be set back, and Android's
 * monotonic clock stops in deep sleep. Either one reaching the limit counts,
 * and a wall clock that went backwards counts too.
 */
export function shouldRecheck(awayWallMs: number, awayMonotonicMs: number): boolean {
  return awayWallMs < 0 || awayWallMs >= RECHECK_AFTER_MS || awayMonotonicMs >= RECHECK_AFTER_MS;
}

/** Sideloads, adb installs and installs with no recorded installer: no store (R8). */
const SIDELOAD_INSTALLERS = new Set([
  'com.google.android.packageinstaller',
  'com.android.packageinstaller',
  'com.android.shell',
]);

/**
 * The store key for an install (R8). The native module (milestone 4) reports
 * the installer; until then every install has no store.
 */
export function storeKeyFor(info: {
  platform: string;
  installer?: string | null;
  environment?: string | null;
}): string | null {
  if (info.platform === 'android') {
    if (!info.installer || SIDELOAD_INSTALLERS.has(info.installer)) return null;
    return info.installer === 'com.android.vending' ? 'play' : info.installer;
  }
  if (info.platform === 'ios') return info.environment === 'appstore' ? 'appstore' : null;
  return null;
}
