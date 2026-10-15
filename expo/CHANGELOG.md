# Changelog

## Unpublished

## 0.1.0 — 2026-10-15

First release.

- `GateProvider` and `useGate()`: pause the app or ask for an update, per store, from a self-hosted gate. The server sends only codes and numbers; every sentence and link ships in the app.
- Default pause and update screens, replaceable, with every word replaceable through `messages`.
- Switches that can hide (and, outside Play, show) features the app already has.
- Native module: which store installed the app (Android installer, iOS App Store environment), Play's in-app update flow, and the App Store page in a sheet.
- Fails open: a server that can't be reached never locks anyone out, and never lifts a pause on screen.
