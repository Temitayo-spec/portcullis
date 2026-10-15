/**
 * The decision: what a gate means for this install (spec/gate-v1.md §4–5).
 *
 * Pure, and the only code that can close the gate. Everything that isn't a
 * clear, well-typed instruction means "run" (R3), so a typo on the server or
 * a bug here can never lock people out by accident.
 */

/** Why the app is paused. Each picks a sentence the app ships (R11). */
export type Reason = 'upgrade' | 'outage' | 'issue';

export const REASONS: readonly Reason[] = ['upgrade', 'outage', 'issue'];

/** What this install is (R7, R8). Every field is null when unknown. */
export interface Install {
  /** `play`, `appstore`, another Android store's package name, or null. */
  store: string | null;
  /** The installed binary's version: Android versionName, iOS short version. */
  version: string | null;
  /** The installed binary's build: Android versionCode, iOS build number. */
  build: string | null;
}

export type Decision =
  | { kind: 'run' }
  | {
      kind: 'maintenance';
      reason: Reason | null;
      /** When the operator expects to be back, in ms since 1970, or null. */
      until: number | null;
    }
  | {
      kind: 'update';
      store: string;
      below: 'version';
      /** The digits the app parsed, never the raw string (R16). */
      minimum: string;
      current: string;
    }
  | { kind: 'update'; store: string; below: 'build'; minimum: number; current: number };

/** The full state the app acts on: a decision, and the switches the gate set. */
export interface GateState {
  decision: Decision;
  /** Only switches the app ships that the gate set, after the Play rule (R19, R20). */
  switches: Readonly<Record<string, boolean>>;
}

/** One read of the gate: an answer (R21), or the honest admission that there wasn't one. */
export type Reading = { kind: 'answer'; body: unknown } | { kind: 'unknown' };

export const RUN: Decision = Object.freeze({ kind: 'run' });

/** A fresh launch: run, with every switch at the app's default (R23). */
export const LAUNCH_STATE: GateState = Object.freeze({
  decision: RUN,
  switches: Object.freeze({}),
});

/** How far ahead an expected return time is still shown (R12). */
export const UNTIL_WINDOW_MS = 7 * 24 * 60 * 60 * 1000;

/** Play's versionCode ceiling (R17). */
export const MAX_BUILD = 2_100_000_000;

/**
 * What a gate means for this install.
 *
 * `switchIds` are the switches the app ships; the gate can set only those.
 */
export function decide(
  body: unknown,
  install: Install,
  switchIds: readonly string[],
  now: number
): GateState {
  const gate = asRecord(body);
  return {
    decision: gate ? decideGate(gate, install, now) : RUN,
    switches: gate ? decideSwitches(gate, install, switchIds) : {},
  };
}

/**
 * Fold a reading into the state the app already holds (R22, R23).
 *
 * No answer keeps what the app is doing: run at launch, but a pause on
 * screen stays, so flaky Wi-Fi is never a way past it. Any answer replaces
 * everything, so a published `{}` lifts a pause at once.
 */
export function applyReading(
  current: GateState,
  reading: Reading,
  install: Install,
  switchIds: readonly string[],
  now: number
): GateState {
  return reading.kind === 'unknown' ? current : decide(reading.body, install, switchIds, now);
}

function decideGate(gate: Record<string, unknown>, install: Install, now: number): Decision {
  // A pause outranks an update request (R13): there's no point updating into
  // a service that is down.
  const maintenance = asRecord(gate.maintenance);
  if (maintenance?.active === true) {
    const reason = REASONS.find((r) => r === maintenance.reason) ?? null;
    return { kind: 'maintenance', reason, until: readUntil(maintenance.until, now) };
  }

  // Only this store's entry, and only a key really written in the document
  // (R14): "__proto__" or "constructor" is not a store.
  if (!install.store) return RUN;
  const stores = asRecord(gate.stores);
  const entry =
    stores && Object.prototype.hasOwnProperty.call(stores, install.store)
      ? asRecord(stores[install.store])
      : null;
  if (!entry) return RUN;

  // A version or build that can't be read on either side asks nobody (R18).
  const minimum = parseVersion(entry.minVersion);
  const running = parseVersion(install.version);
  if (minimum && running && compareVersions(running, minimum) < 0) {
    return {
      kind: 'update',
      store: install.store,
      below: 'version',
      minimum: minimum.join('.'),
      current: running.join('.'),
    };
  }
  const minBuild = readBuild(entry.minBuild, false); // the gate's must be a JSON number
  const build = readBuild(install.build, true); // the device may send a string (R17)
  if (minBuild !== null && build !== null && build < minBuild) {
    return {
      kind: 'update',
      store: install.store,
      below: 'build',
      minimum: minBuild,
      current: build,
    };
  }
  return RUN;
}

function decideSwitches(
  gate: Record<string, unknown>,
  install: Install,
  switchIds: readonly string[]
): Record<string, boolean> {
  const switches = asRecord(gate.switches);
  if (!switches) return {};
  // On Play, and on an install whose store is unknown, a switch can only hide
  // (R20): Play bans reviewed apps that turn features on remotely.
  const mayShow = install.store !== null && install.store !== 'play';
  const out: Record<string, boolean> = {};
  for (const id of switchIds) {
    if (!Object.prototype.hasOwnProperty.call(switches, id)) continue;
    const value = switches[id];
    if (value === false || (value === true && mayShow)) out[id] = value;
  }
  return out;
}

/** "1.2.3" → [1, 2, 3]; null for anything that can't be read (R15). */
export function parseVersion(value: unknown): number[] | null {
  // No trimming, and only a lowercase "v": a space or "V" can't be read (R15).
  if (typeof value !== 'string' || value.length === 0 || value.length > 32) return null;
  const core = value.replace(/^v/, '').split(/[-+]/, 1)[0] ?? '';
  const parts = core.split('.');
  if (parts.length > 4) return null;
  const numbers: number[] = [];
  for (const part of parts) {
    if (!/^\d{1,6}$/.test(part)) return null;
    numbers.push(Number(part));
  }
  return numbers;
}

/** Number by number; a missing number counts as 0, so 1.2 equals 1.2.0 (R16). */
export function compareVersions(a: readonly number[], b: readonly number[]): number {
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const diff = (a[i] ?? 0) - (b[i] ?? 0);
    if (diff !== 0) return diff < 0 ? -1 : 1;
  }
  return 0;
}

/**
 * A whole number from 1 to 2,100,000,000 (R17). `fromDevice` also accepts a
 * string of digits, as the device sends it; a gate's minBuild must be a number.
 */
export function readBuild(value: unknown, fromDevice: boolean): number | null {
  const n =
    fromDevice && typeof value === 'string' && /^\d{1,10}$/.test(value) ? Number(value) : value;
  return typeof n === 'number' && Number.isInteger(n) && n >= 1 && n <= MAX_BUILD ? n : null;
}

// A zone is required: without one, the same text is a different moment on every phone (R12).
const ISO_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d{1,9})?)?(Z|[+-]\d{2}:\d{2})$/;

/** An expected return time within the next 7 days, or null (R12). */
export function readUntil(value: unknown, now: number): number | null {
  // ISO 8601 only: Date.parse alone would also accept "October 20, 2026".
  if (typeof value !== 'string' || value.length > 40 || !ISO_TIME.test(value)) return null;
  const at = Date.parse(value);
  if (!Number.isFinite(at) || at <= now || at - now > UNTIL_WINDOW_MS) return null;
  return at;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}
