import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { AppState } from 'react-native';

import { applyReading, LAUNCH_STATE, type Decision, type GateState, type Install } from './decide';
import { loadInstall, readInstall } from './install';
import { DEFAULT_MESSAGES, type Messages } from './messages';
import { readGate, shouldRecheck } from './read';
import {
  PauseScreen,
  UpdateScreen,
  type PauseScreenProps,
  type UpdateScreenProps,
} from './screens';

export interface Gate {
  /** What the app should do: run, pause, or ask for an update. */
  decision: Decision;
  /** The switches the gate set. Use `isShown` rather than reading this directly. */
  switches: Readonly<Record<string, boolean>>;
  /** Whether a switch is on: the gate's value, or `shipped` when the gate doesn't set it. */
  isShown: (id: string, shipped: boolean) => boolean;
  /** Read the gate now. The pause screen's way out. */
  checkAgain: () => void;
  /** True while a read is in flight. */
  checking: boolean;
  /** True when the last finished read got no answer, so nothing changed. */
  unreachable: boolean;
  /** True once the first read has finished, answered or not. */
  ready: boolean;
  /** The install the last read used, store included. Null before the first read. */
  install: Install | null;
}

export interface GateProviderProps {
  /** Where the gate is served, such as https://gate.example.com/v1/gate. Empty turns the gate off. */
  url: string;
  /** The switch ids this app ships. The gate can set only these (R19). */
  switches?: readonly string[];
  /** The app's own words. Anything left out uses the English defaults. */
  messages?: Partial<Messages>;
  /**
   * Where each store's update button goes, owned by the app (R2). Play
   * defaults to this app's own listing; others show instructions instead.
   */
  storeLinks?: Readonly<Record<string, string>>;
  /**
   * This app's numeric App Store id, for the iOS update button. Optional: the
   * native module looks it up from the bundle id when it's left out.
   */
  appStoreId?: string;
  /** Replace the default pause screen. */
  renderPause?: (props: PauseScreenProps) => ReactNode;
  /** Replace the default update screen. */
  renderUpdate?: (props: UpdateScreenProps) => ReactNode;
  /** Called whenever the decision changes, for analytics. */
  onDecision?: (decision: Decision) => void;
  /** Overrides what the install reports. For tests and the example app only. */
  install?: Install;
  /** Swapped in tests. */
  fetchImpl?: typeof fetch;
  children: ReactNode;
}

const GateContext = createContext<Gate | null>(null);

/**
 * Reads the gate at launch and when the app comes back, and stands the pause
 * or update screen in for the app while the gate is closed.
 *
 * The app paints straight away: the gate starts at "run" and only moves if
 * the server says so. Nothing re-reads while someone is using the app, so a
 * pause can't pull the screen out from under them mid-task; coming back after
 * 5 minutes away is a fresh start, so that reads again (R25). A closed gate
 * reads on every return, since all a read can do then is let people back in.
 */
export function GateProvider({
  url,
  switches: switchIds = NO_IDS,
  messages,
  storeLinks,
  appStoreId,
  renderPause,
  renderUpdate,
  onDecision,
  install: installOverride,
  fetchImpl,
  children,
}: GateProviderProps) {
  // Version and build are known at once; the store waits on the native module.
  const version = useMemo(() => (installOverride ?? readInstall()).version, [installOverride]);
  const [round, setRound] = useState(0);
  const [answer, setAnswer] = useState<{
    round: number;
    state: GateState;
    unreachable: boolean;
    install: Install | null;
  }>({ round: -1, state: LAUNCH_STATE, unreachable: false, install: null });

  // Kept in a ref so a re-render with a new array or object doesn't trigger a
  // read. Declared before the read effect, so it is up to date when that runs.
  const latest = useRef({ installOverride, switchIds, fetchImpl });
  useEffect(() => {
    latest.current = { installOverride, switchIds, fetchImpl };
  });

  useEffect(() => {
    if (!url) return; // no gate: the app runs, and `ready` is true from the start
    const stop = new AbortController();
    const { installOverride, switchIds, fetchImpl } = latest.current;
    (async () => {
      const install = installOverride ?? (await loadInstall());
      if (stop.signal.aborted) return;
      const reading = await readGate(url, install, { fetchImpl, signal: stop.signal });
      if (stop.signal.aborted) return;
      setAnswer((prev) => ({
        round,
        state: applyReading(prev.state, reading, install, switchIds, Date.now()),
        unreachable: reading.kind === 'unknown',
        install,
      }));
    })();
    return () => stop.abort();
  }, [round, url]);

  const checkAgain = useCallback(() => setRound((n) => n + 1), []);

  const closed = answer.state.decision.kind !== 'run';
  const closedNow = useRef(closed);
  useEffect(() => {
    closedNow.current = closed;
  }, [closed]);

  useEffect(() => {
    let leftAt: { wall: number; monotonic: number } | null = null;
    const sub = AppState.addEventListener('change', (next) => {
      if (next === 'background') {
        leftAt ??= { wall: Date.now(), monotonic: performance.now() };
        return;
      }
      if (next !== 'active') return;
      const since = leftAt;
      leftAt = null;
      if (closedNow.current) checkAgain();
      else if (since && shouldRecheck(Date.now() - since.wall, performance.now() - since.monotonic))
        checkAgain();
    });
    return () => sub.remove();
  }, [checkAgain]);

  const decision = answer.state.decision;
  const onDecisionRef = useRef(onDecision);
  useEffect(() => {
    onDecisionRef.current = onDecision;
  });
  useEffect(() => {
    onDecisionRef.current?.(decision);
  }, [decision]);

  const checking = Boolean(url) && answer.round !== round;
  const value = useMemo<Gate>(() => {
    const switches = answer.state.switches;
    return {
      decision,
      switches,
      isShown: (id, shipped) => switches[id] ?? shipped,
      checkAgain,
      checking,
      // Only about a read that has finished; while one is in flight, `checking` says so.
      unreachable: answer.unreachable && !checking,
      ready: !url || answer.round >= 0,
      install: answer.install,
    };
  }, [answer, decision, checkAgain, checking]);

  const words = useMemo(() => ({ ...DEFAULT_MESSAGES, ...messages }), [messages]);

  let content: ReactNode = children;
  if (decision.kind === 'maintenance') {
    const props: PauseScreenProps = {
      decision,
      messages: words,
      checking: value.checking,
      unreachable: value.unreachable,
      onCheckAgain: checkAgain,
      version,
    };
    content = renderPause ? renderPause(props) : <PauseScreen {...props} />;
  } else if (decision.kind === 'update') {
    const props: UpdateScreenProps = {
      decision,
      messages: words,
      storeLinks,
      appStoreId,
      version,
    };
    content = renderUpdate ? renderUpdate(props) : <UpdateScreen {...props} />;
  }

  return <GateContext.Provider value={value}>{content}</GateContext.Provider>;
}

const NO_IDS: readonly string[] = [];

/** The gate, from anywhere inside GateProvider. */
export function useGate(): Gate {
  const gate = useContext(GateContext);
  if (!gate) throw new Error('useGate must be used inside <GateProvider>.');
  return gate;
}
