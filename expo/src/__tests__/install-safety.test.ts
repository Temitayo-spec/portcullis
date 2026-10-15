import { readFileSync, readdirSync } from 'fs';
import { join } from 'path';

// An app that ships over-the-air updates can run this library on a binary
// built without expo-application's native module. Loading the library there
// must not crash, so no file may import the expo-application package itself.
it('never imports expo-application', () => {
  const src = join(__dirname, '..');
  for (const file of readdirSync(src).filter((f) => /\.tsx?$/.test(f))) {
    const code = readFileSync(join(src, file), 'utf8');
    expect({
      file,
      imports: /from ['"]expo-application['"]|require\(['"]expo-application['"]\)/.test(code),
    }).toEqual({
      file,
      imports: false,
    });
  }
});

describe('without expo-application in the binary', () => {
  beforeEach(() => jest.resetModules());
  afterEach(() => jest.dontMock('expo'));

  it('loads, and reports version and build as unknown', () => {
    jest.doMock('expo', () => ({ requireOptionalNativeModule: () => null }));

    const { readInstall, playListing } = require('../install') as typeof import('../install');
    expect(readInstall()).toEqual({ store: null, version: null, build: null });
    expect(playListing()).toEqual([]);
  });

  it('reads them when the native module is there', () => {
    jest.doMock('expo', () => ({
      requireOptionalNativeModule: (name: string) =>
        name === 'ExpoApplication'
          ? { nativeApplicationVersion: '2.4.0', nativeBuildVersion: '120' }
          : null,
    }));

    const { readInstall } = require('../install') as typeof import('../install');
    expect(readInstall()).toEqual({ store: null, version: '2.4.0', build: '120' });
  });
});
