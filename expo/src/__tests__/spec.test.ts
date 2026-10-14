/**
 * Runs every case in spec/cases/decide and spec/cases/store against this
 * library, so the app can't drift from the contract (spec/gate-v1.md §9).
 */
import { readdirSync, readFileSync } from 'fs';
import { join } from 'path';

import { applyReading, type Decision, type GateState, type Reading } from '../decide';
import { storeKeyFor } from '../read';

const CASES = join(__dirname, '..', '..', '..', 'spec', 'cases');

type Expect = Record<string, unknown>;
interface DecideCase {
  name: string;
  rule: string;
  now: string;
  current?: Expect;
  switchIds?: string[];
  gate?: unknown;
  reading?: 'unreachable' | 'invalid';
  device: { store: string | null; version: string | null; build: string | null };
  expect: Expect;
}

function load<T>(path: string): T[] {
  return JSON.parse(readFileSync(path, 'utf8')) as T[];
}

/** A case's `current` as the state the app holds. */
function toState(current: Expect = { kind: 'run' }): GateState {
  const { switches = {}, ...decision } = current as { switches?: Record<string, boolean> };
  const until = (decision as { until?: string | null }).until;
  return {
    decision: {
      ...decision,
      ...(typeof until === 'string' ? { until: Date.parse(until) } : {}),
    } as Decision,
    switches,
  };
}

/** The library's state in the case format, so `expect` can be compared field by field. */
function toCaseShape(state: GateState): Expect {
  const out: Expect = { ...state.decision, switches: state.switches };
  if (state.decision.kind === 'maintenance') out.until = state.decision.until;
  return out;
}

function matches(expect: Expect, got: Expect): boolean {
  return Object.entries(expect).every(([key, want]) => {
    if (key === 'until' && typeof want === 'string') return got.until === Date.parse(want);
    return JSON.stringify(want) === JSON.stringify(got[key]);
  });
}

describe('spec/cases/decide', () => {
  const files = readdirSync(join(CASES, 'decide'));
  it('found the case files', () => expect(files.length).toBeGreaterThanOrEqual(4));
  for (const file of files) {
    describe(file, () => {
      for (const c of load<DecideCase>(join(CASES, 'decide', file))) {
        it(`${c.rule} ${c.name}`, () => {
          const reading: Reading = c.reading
            ? { kind: 'unknown' }
            : { kind: 'answer', body: c.gate };
          const got = applyReading(
            toState(c.current),
            reading,
            c.device,
            c.switchIds ?? [],
            Date.parse(c.now)
          );
          const shape = toCaseShape(got);
          if (!matches(c.expect, shape)) {
            throw new Error(
              `expected ${JSON.stringify(c.expect)}\n     got ${JSON.stringify(shape)}`
            );
          }
        });
      }
    });
  }
});

describe('spec/cases/store', () => {
  const cases = load<{
    name: string;
    platform: string;
    installer?: string | null;
    environment?: string;
    expect: string | null;
  }>(join(CASES, 'store', 'installers.json'));
  for (const c of cases) {
    it(c.name, () => expect(storeKeyFor(c)).toBe(c.expect));
  }
});
