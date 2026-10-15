# Changelog

## Unpublished

## 0.1.1 — 2026-10-15

- iOS: works on iOS 15.1 and later, Expo SDK 55's minimum (0.1.0 required 16.4, which broke `pod install` in apps that support older iPhones). On iOS 15 the install reports no store, so it's never asked to update; pausing works as before.
- Never crashes an app that ships over-the-air updates to binaries built without `expo-application`. 0.1.0 imported that package, whose JavaScript requires its native module on load; the library now reads the version and build from the native module only if it's there. Without it, version and build are unknown, so minimums don't apply, and pausing still works. `expo-application` is now an optional peer dependency.

## 0.1.0 — 2026-10-15

First release.

- `GateProvider` and `useGate()`: pause the app or ask for an update, per store, from a self-hosted gate. The server sends only codes and numbers; every sentence and link ships in the app.
- Default pause and update screens, replaceable, with every word replaceable through `messages`.
- Switches that can hide (and, outside Play, show) features the app already has.
- Native module: which store installed the app (Android installer, iOS App Store environment), Play's in-app update flow, and the App Store page in a sheet.
- Fails open: a server that can't be reached never locks anyone out, and never lifts a pause on screen.
