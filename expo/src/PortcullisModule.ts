import { requireOptionalNativeModule } from 'expo';

/** What the native side reports about how this copy was installed (R8). Raw facts; storeKeyFor maps them. */
export type InstallSource =
  | { platform: 'android'; installer: string | null }
  | {
      platform: 'ios';
      environment: 'appstore' | 'testflight' | 'simulator' | 'development' | 'unknown';
    };

/** How Play's update screen ended. Anything but accepted or cancelled means: open the listing instead. */
export type PlayUpdateResult =
  'accepted' | 'cancelled' | 'not-available' | 'not-allowed' | 'failed' | 'no-activity';

interface PortcullisNativeModule {
  getInstallSourceAsync(): Promise<InstallSource>;
  /** Android only. */
  startPlayUpdateAsync?: () => Promise<PlayUpdateResult>;
  /** iOS only. Looks the id up from the bundle id when null. */
  openAppStorePageAsync?: (appStoreId: string | null) => Promise<boolean>;
}

/**
 * The native module, or null where it isn't built in: Expo Go, the web, and
 * tests. Everything that uses it has a fallback, so the gate still works
 * without it; installs just have no store (R8).
 */
export const PortcullisNative = requireOptionalNativeModule<PortcullisNativeModule>('Portcullis');
