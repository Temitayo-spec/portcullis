import * as Application from 'expo-application';
import { Platform } from 'react-native';

import type { Install } from './decide';

/**
 * This install, from native reads only (R7). Never Expo config: an
 * over-the-air update changes that, and comparing against it can ask people
 * to update with nothing newer in their store.
 *
 * The store needs this package's native module (milestone 4). Without it,
 * as in Expo Go, the store is null: such installs are never asked to update,
 * and switches there can only hide (R8, R14, R20).
 */
export function readInstall(): Install {
  return {
    store: null,
    version: Application.nativeApplicationVersion ?? null,
    build: Application.nativeBuildVersion ?? null,
  };
}

/** This app's own Play listing, from its installed package id. Owned by the app, never the server. */
export function playListing(): string[] {
  const id = Platform.OS === 'android' ? Application.applicationId : null;
  return id
    ? [`market://details?id=${id}`, `https://play.google.com/store/apps/details?id=${id}`]
    : [];
}
