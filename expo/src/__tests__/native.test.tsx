import { fireEvent, render, screen } from '@testing-library/react-native';
import { Linking } from 'react-native';

import { loadInstall, resetInstallForTests, STORE_DEADLINE_MS } from '../install';
import { DEFAULT_MESSAGES } from '../messages';
import { UpdateScreen } from '../screens';

// A stand-in for the native module, changed per test. Jest moves jest.mock
// above the imports, so install.ts and screens.tsx see it.
const mockNative: {
  getInstallSourceAsync: jest.Mock;
  startPlayUpdateAsync?: jest.Mock;
  openAppStorePageAsync?: jest.Mock;
} = { getInstallSourceAsync: jest.fn() };

jest.mock('../PortcullisModule', () => ({
  get PortcullisNative() {
    return mockNative;
  },
}));

beforeEach(() => {
  resetInstallForTests();
  mockNative.getInstallSourceAsync = jest.fn();
  delete mockNative.startPlayUpdateAsync;
  delete mockNative.openAppStorePageAsync;
});

describe('loadInstall', () => {
  it.each([
    [{ platform: 'android', installer: 'com.android.vending' }, 'play'],
    [
      { platform: 'android', installer: 'com.sec.android.app.samsungapps' },
      'com.sec.android.app.samsungapps',
    ],
    [{ platform: 'android', installer: 'com.google.android.packageinstaller' }, null],
    [{ platform: 'android', installer: null }, null],
    [{ platform: 'ios', environment: 'appstore' }, 'appstore'],
    [{ platform: 'ios', environment: 'testflight' }, null],
    [{ platform: 'ios', environment: 'simulator' }, null],
  ])('turns %j into store %p (R8)', async (source, store) => {
    mockNative.getInstallSourceAsync.mockResolvedValue(source);
    await expect(loadInstall()).resolves.toMatchObject({ store });
  });

  it('reads the store once per launch', async () => {
    mockNative.getInstallSourceAsync.mockResolvedValue({
      platform: 'ios',
      environment: 'appstore',
    });
    await loadInstall();
    await loadInstall();
    expect(mockNative.getInstallSourceAsync).toHaveBeenCalledTimes(1);
  });

  it('gives no store when the native module fails, and tries again next time', async () => {
    mockNative.getInstallSourceAsync.mockRejectedValueOnce(new Error('boom'));
    await expect(loadInstall()).resolves.toMatchObject({ store: null });
    mockNative.getInstallSourceAsync.mockResolvedValueOnce({
      platform: 'android',
      installer: 'com.android.vending',
    });
    await expect(loadInstall()).resolves.toMatchObject({ store: 'play' });
  });

  it('gives no store when the native module is too slow', async () => {
    jest.useFakeTimers();
    mockNative.getInstallSourceAsync.mockReturnValue(new Promise(() => {}));
    const install = loadInstall();
    jest.advanceTimersByTime(STORE_DEADLINE_MS);
    await expect(install).resolves.toMatchObject({ store: null });
    jest.useRealTimers();
  });
});

describe('the update button', () => {
  const playUpdate = {
    kind: 'update',
    store: 'play',
    below: 'version',
    minimum: '2.5.0',
    current: '2.4.0',
  } as const;
  const appStoreUpdate = { ...playUpdate, store: 'appstore' } as const;

  it("uses Play's own update screen when the native module has it", async () => {
    mockNative.startPlayUpdateAsync = jest.fn().mockResolvedValue('accepted');
    const open = jest.spyOn(Linking, 'openURL');
    await render(
      <UpdateScreen decision={playUpdate} messages={DEFAULT_MESSAGES} version="2.4.0" />
    );
    await fireEvent.press(screen.getByText('Update'));
    expect(mockNative.startPlayUpdateAsync).toHaveBeenCalled();
    expect(open).not.toHaveBeenCalled();
  });

  it('shows instructions when Play can’t help and there is no listing to open', async () => {
    mockNative.startPlayUpdateAsync = jest.fn().mockResolvedValue('failed');
    await render(
      <UpdateScreen decision={playUpdate} messages={DEFAULT_MESSAGES} version="2.4.0" />
    );
    await fireEvent.press(screen.getByText('Update'));
    expect(
      screen.getByText('Open Google Play and update the app to the latest version.')
    ).toBeTruthy();
  });

  it('opens the App Store page with the app’s own id', async () => {
    mockNative.openAppStorePageAsync = jest.fn().mockResolvedValue(true);
    await render(
      <UpdateScreen
        decision={appStoreUpdate}
        messages={DEFAULT_MESSAGES}
        appStoreId="123456789"
        version="2.4.0"
      />
    );
    await fireEvent.press(screen.getByText('Update'));
    expect(mockNative.openAppStorePageAsync).toHaveBeenCalledWith('123456789');
  });

  it('prefers a link the app passed in', async () => {
    mockNative.openAppStorePageAsync = jest.fn();
    const open = jest.spyOn(Linking, 'openURL').mockResolvedValue(true);
    await render(
      <UpdateScreen
        decision={appStoreUpdate}
        messages={DEFAULT_MESSAGES}
        storeLinks={{ appstore: 'https://apps.apple.com/app/id1' }}
        version="2.4.0"
      />
    );
    await fireEvent.press(screen.getByText('Update'));
    expect(open).toHaveBeenCalledWith('https://apps.apple.com/app/id1');
    expect(mockNative.openAppStorePageAsync).not.toHaveBeenCalled();
  });

  it('shows no button for a store it has no way to open', async () => {
    await render(
      <UpdateScreen
        decision={{ ...playUpdate, store: 'com.huawei.appmarket' }}
        messages={DEFAULT_MESSAGES}
        version="2.4.0"
      />
    );
    expect(screen.queryByText('Update')).toBeNull();
    expect(
      screen.getByText('Open AppGallery and update the app to the latest version.')
    ).toBeTruthy();
  });
});
