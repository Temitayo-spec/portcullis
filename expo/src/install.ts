import * as Application from 'expo-application';
import { Platform } from 'react-native';

import { PortcullisNative, type InstallSource } from './PortcullisModule';
import type { Install } from './decide';
import { storeKeyFor } from './read';

/** How long to wait for the native module to say which store this is. */
export const STORE_DEADLINE_MS = 1_500;

/**
 * This install's version and build, from native reads only (R7). Never Expo
 * config: an over-the-air update changes that, and comparing against it can
 * ask people to update with nothing newer in their store.
 */
export function readInstall(): Install {
  return {
    store: null,
    version: Application.nativeApplicationVersion ?? null,
    build: Application.nativeBuildVersion ?? null,
  };
}

let loaded: Promise<Install> | null = null;

/**
 * This install, store included. The store comes from the native module (R8);
 * without it, as in Expo Go, or if it takes too long, the store is null: such
 * installs are never asked to update, and switches there can only hide.
 *
 * A store that was read is kept for the rest of the launch. One that wasn't
 * (too slow, or an error) is tried again at the next read.
 */
export function loadInstall(): Promise<Install> {
  loaded ??= (async () => {
    const source = await readInstallSource();
    if (source === null && PortcullisNative) loaded = null; // retry next time
    return { ...readInstall(), store: source ? storeKeyFor(source) : null };
  })();
  return loaded;
}

/** What the native module reports, or null if it isn't there, fails or is slow. */
export async function readInstallSource(): Promise<InstallSource | null> {
  if (!PortcullisNative) return null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      PortcullisNative.getInstallSourceAsync(),
      new Promise<null>((resolve) => {
        timer = setTimeout(() => resolve(null), STORE_DEADLINE_MS);
      }),
    ]);
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}

/** Forget the loaded install. For tests only. */
export function resetInstallForTests(): void {
  loaded = null;
}

/** This app's own Play listing, from its installed package id. Owned by the app, never the server. */
export function playListing(): string[] {
  const id = Platform.OS === 'android' ? Application.applicationId : null;
  return id
    ? [`market://details?id=${id}`, `https://play.google.com/store/apps/details?id=${id}`]
    : [];
}
