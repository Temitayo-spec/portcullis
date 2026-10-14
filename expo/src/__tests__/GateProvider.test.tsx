import { act, fireEvent, render, screen } from '@testing-library/react-native';
import { AppState, Linking, Text, type AppStateStatus } from 'react-native';

import { GateProvider, useGate } from '../GateProvider';

const PLAY = { store: 'play', version: '2.4.0', build: '120' };

/** A fake server: each read gets the next answer. `null` means no answer. */
function server(...answers: (string | null)[]) {
  const reads: RequestInit[] = [];
  const fetchImpl = (async (_url: string, init?: RequestInit) => {
    reads.push(init ?? {});
    const body = answers.length > 1 ? answers.shift()! : answers[0]!;
    if (body === null) throw new TypeError('Network request failed');
    return new Response(body, { status: 200 });
  }) as typeof fetch;
  return { fetchImpl, reads };
}

function Home() {
  const gate = useGate();
  return (
    <>
      <Text>Home</Text>
      <Text>rewards {gate.isShown('rewards', true) ? 'shown' : 'hidden'}</Text>
      <Text>{gate.ready ? 'ready' : 'reading'}</Text>
    </>
  );
}

async function renderGate(
  fetchImpl: typeof fetch,
  props: Partial<Parameters<typeof GateProvider>[0]> = {}
) {
  await render(
    <GateProvider
      url="https://gate.test/v1/gate"
      install={PLAY}
      switches={['rewards']}
      fetchImpl={fetchImpl}
      {...props}>
      <Home />
    </GateProvider>
  );
  await act(async () => {}); // let the first read finish
}

it('shows the app when the gate says run', async () => {
  await renderGate(server('{}').fetchImpl);
  expect(screen.getByText('Home')).toBeTruthy();
  expect(screen.getByText('ready')).toBeTruthy();
});

it('shows the app at once, before the first read answers', async () => {
  const never = (() => new Promise<Response>(() => {})) as typeof fetch;
  await render(
    <GateProvider url="u" install={PLAY} fetchImpl={never}>
      <Home />
    </GateProvider>
  );
  expect(screen.getByText('Home')).toBeTruthy();
  expect(screen.getByText('reading')).toBeTruthy();
});

it('stands the pause screen in for the app, with the reason the app ships', async () => {
  await renderGate(server('{"maintenance":{"active":true,"reason":"outage"}}').fetchImpl);
  expect(screen.queryByText('Home')).toBeNull();
  expect(screen.getByText('Paused for maintenance')).toBeTruthy();
  expect(screen.getByText(/A service the app relies on is down/)).toBeTruthy();
});

it('lifts the pause when Check again gets {}', async () => {
  await renderGate(server('{"maintenance":{"active":true}}', '{}').fetchImpl);
  await fireEvent.press(screen.getByText('Check again'));
  await act(async () => {});
  expect(screen.getByText('Home')).toBeTruthy();
});

it('keeps the pause when Check again reaches nobody (R22)', async () => {
  await renderGate(server('{"maintenance":{"active":true}}', null).fetchImpl);
  await fireEvent.press(screen.getByText('Check again'));
  await act(async () => {});
  expect(screen.getByText('Paused for maintenance')).toBeTruthy();
  expect(screen.getByText(/couldn't reach the server/)).toBeTruthy();
});

it('runs when the first read reaches nobody', async () => {
  await renderGate(server(null).fetchImpl);
  expect(screen.getByText('Home')).toBeTruthy();
});

it('asks for an update, and the button opens the link the app ships', async () => {
  const open = jest.spyOn(Linking, 'openURL').mockResolvedValue(true);
  await renderGate(server('{"stores":{"play":{"minVersion":"2.5.0"}}}').fetchImpl, {
    storeLinks: { play: 'https://play.google.com/store/apps/details?id=com.example' },
  });
  expect(screen.getByText('Time to update')).toBeTruthy();
  expect(
    screen.getByText(/This version \(2\.4\.0\) is no longer supported\. Update to version 2\.5\.0/)
  ).toBeTruthy();
  await fireEvent.press(screen.getByText('Update'));
  await act(async () => {});
  expect(open).toHaveBeenCalledWith('https://play.google.com/store/apps/details?id=com.example');
});

it('shows instructions instead of a button when the app has no link for that store', async () => {
  await renderGate(server('{"stores":{"com.huawei.appmarket":{"minBuild":200}}}').fetchImpl, {
    install: { store: 'com.huawei.appmarket', version: '2.4.0', build: '120' },
  });
  expect(
    screen.getByText('Open AppGallery and update the app to the latest version.')
  ).toBeTruthy();
  expect(screen.queryByText('Update')).toBeNull();
});

it('applies switches, and Play can only hide (R20)', async () => {
  await renderGate(server('{"switches":{"rewards":false}}').fetchImpl);
  expect(screen.getByText('rewards hidden')).toBeTruthy();
});

it('uses the app’s own words and screens', async () => {
  await renderGate(server('{"maintenance":{"active":true,"reason":"upgrade"}}').fetchImpl, {
    messages: { pauseTitle: 'Back soon' },
  });
  expect(screen.getByText('Back soon')).toBeTruthy();

  await renderGate(server('{"maintenance":{"active":true}}').fetchImpl, {
    renderPause: ({ decision }) => <Text>custom pause {decision.reason ?? 'none'}</Text>,
  });
  expect(screen.getByText('custom pause none')).toBeTruthy();
});

it('sends the R9 headers', async () => {
  const s = server('{}');
  await renderGate(s.fetchImpl);
  expect(s.reads[0]?.headers).toMatchObject({
    'X-Portcullis-Store': 'play',
    'X-Portcullis-Version': '2.4.0',
  });
});

it('reads nothing with an empty url', async () => {
  const s = server('{"maintenance":{"active":true}}');
  await renderGate(s.fetchImpl, { url: '' });
  expect(s.reads).toHaveLength(0);
  expect(screen.getByText('ready')).toBeTruthy();
});

describe('coming back to the app (R25)', () => {
  // Every listener, since React Native's own components may listen too.
  let listeners: ((state: AppStateStatus) => void)[] = [];
  const onChange = (state: AppStateStatus) => listeners.forEach((listener) => listener(state));
  beforeEach(() => {
    listeners = [];
    jest.spyOn(AppState, 'addEventListener').mockImplementation((_type, handler) => {
      listeners.push(handler as (state: AppStateStatus) => void);
      return { remove: () => {} } as ReturnType<typeof AppState.addEventListener>;
    });
  });
  afterEach(() => jest.restoreAllMocks());

  async function away(ms: number) {
    const start = Date.now();
    const wall = jest.spyOn(Date, 'now');
    wall.mockReturnValue(start);
    await act(async () => onChange('background'));
    wall.mockReturnValue(start + ms);
    await act(async () => onChange('active'));
    wall.mockRestore();
  }

  it('reads again after 5 minutes away', async () => {
    const s = server('{}', '{"maintenance":{"active":true}}');
    await renderGate(s.fetchImpl);
    await away(5 * 60_000);
    expect(s.reads).toHaveLength(2);
    expect(screen.getByText('Paused for maintenance')).toBeTruthy();
  });

  it('does not read again after a short trip away', async () => {
    const s = server('{}');
    await renderGate(s.fetchImpl);
    await away(60_000);
    expect(s.reads).toHaveLength(1);
  });

  it('reads on every return while paused, since all it can do is let people in', async () => {
    const s = server('{"maintenance":{"active":true}}', '{}');
    await renderGate(s.fetchImpl);
    await away(1_000);
    expect(screen.getByText('Home')).toBeTruthy();
  });
});
