import { readGate, shouldRecheck, RECHECK_AFTER_MS } from '../read';

const install = { store: 'play', version: '2.4.0', build: '120' };

function respond(status: number, body: string): typeof fetch {
  return (async () => new Response(body, { status })) as typeof fetch;
}

describe('readGate', () => {
  it('reads a 200 JSON answer', async () => {
    await expect(
      readGate('https://g/v1/gate', install, {
        fetchImpl: respond(200, '{"maintenance":{"active":true}}'),
      })
    ).resolves.toEqual({
      kind: 'answer',
      body: { maintenance: { active: true } },
    });
  });

  it('treats a JSON array as an answer (R21)', async () => {
    await expect(readGate('u', install, { fetchImpl: respond(200, '[]') })).resolves.toEqual({
      kind: 'answer',
      body: [],
    });
  });

  it.each([
    ['a 503', respond(503, '{}')],
    ['a 204', respond(204, '')],
    ['HTML from a captive portal', respond(200, '<html>Sign in to Wi-Fi</html>')],
    [
      'a network error',
      (async () => {
        throw new TypeError('Network request failed');
      }) as typeof fetch,
    ],
  ])('gives no answer for %s (R21)', async (_name, fetchImpl) => {
    await expect(readGate('u', install, { fetchImpl })).resolves.toEqual({ kind: 'unknown' });
  });

  it('gives no answer after the deadline, even if the host never replies (R24)', async () => {
    const never = (() => new Promise<Response>(() => {})) as typeof fetch;
    await expect(readGate('u', install, { fetchImpl: never, timeoutMs: 20 })).resolves.toEqual({
      kind: 'unknown',
    });
  });

  it('sends the contract and the build, and no cookies (R9)', async () => {
    let seen: RequestInit | undefined;
    const spy = (async (_url: string, init?: RequestInit) => {
      seen = init;
      return new Response('{}', { status: 200 });
    }) as typeof fetch;
    await readGate('u', install, { fetchImpl: spy });
    expect(seen?.credentials).toBe('omit');
    expect(seen?.cache).toBe('no-store');
    expect(seen?.headers).toMatchObject({
      'X-Portcullis-Contract': '1',
      'X-Portcullis-Store': 'play',
      'X-Portcullis-Version': '2.4.0',
      'X-Portcullis-Build': '120',
    });
  });

  it('leaves out what it does not know', async () => {
    let headers: Record<string, string> = {};
    const spy = (async (_url: string, init?: RequestInit) => {
      headers = init?.headers as Record<string, string>;
      return new Response('{}', { status: 200 });
    }) as typeof fetch;
    await readGate('u', { store: null, version: null, build: null }, { fetchImpl: spy });
    expect(Object.keys(headers)).not.toContain('X-Portcullis-Store');
  });
});

describe('shouldRecheck (R25)', () => {
  it.each([
    [RECHECK_AFTER_MS, 0, true],
    [0, RECHECK_AFTER_MS, true],
    [-1, 0, true],
    [RECHECK_AFTER_MS - 1, RECHECK_AFTER_MS - 1, false],
  ])('wall %d, monotonic %d → %s', (wall, mono, want) => {
    expect(shouldRecheck(wall, mono)).toBe(want);
  });
});

it('gives up at once when its signal aborts', async () => {
  const never = (() => new Promise<Response>(() => {})) as typeof fetch;
  const stop = new AbortController();
  const reading = readGate('u', install, {
    fetchImpl: never,
    signal: stop.signal,
    timeoutMs: 60_000,
  });
  stop.abort();
  await expect(reading).resolves.toEqual({ kind: 'unknown' });
});
