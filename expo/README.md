# expo-portcullis

A remote pause and update gate for Expo apps. Pause the app or require an update, per store, without shipping a build. The server can never put words or links on your users' screens: it sends only reason codes and version numbers, and every sentence and store link lives in the app.

This is the app side. The server and `gatectl` live in [`server/`](../server), and the contract both follow is in [`spec/gate-v1.md`](../spec/gate-v1.md).

## Use it

```bash
npx expo install expo-portcullis expo-application
```

Wrap your app:

```tsx
import { GateProvider } from 'expo-portcullis';

export default function App() {
  return (
    <GateProvider url="https://gate.example.com/v1/gate" switches={['rewards']}>
      <YourApp />
    </GateProvider>
  );
}
```

- The app paints at once. If the gate says pause or update, that screen stands in for the app.
- The gate is read at launch, again when someone comes back after 5 minutes away, and on every return while paused.
- If the server can't be reached, the app keeps doing what it was doing: it runs at launch, and a pause on screen stays.

Read the gate anywhere inside:

```tsx
import { useGate } from 'expo-portcullis';

function Home() {
  const gate = useGate();
  return gate.isShown('rewards', true) ? <Rewards /> : null; // true = shown when the gate says nothing
}
```

## Props

| Prop | What it does |
| --- | --- |
| `url` | Where the gate is served. An empty string turns the gate off |
| `switches` | The switch ids your app ships. The gate can only set these |
| `messages` | Your own words for the screens; anything left out uses the English defaults |
| `storeLinks` | Where each store's Update button goes, such as `{ 'com.huawei.appmarket': 'appmarket://details?id=…' }`. Overrides the defaults below |
| `appStoreId` | Your numeric App Store id, for the iOS Update button. Optional: it's looked up from your bundle id when left out |
| `renderPause`, `renderUpdate` | Replace the default screens |
| `onDecision` | Called whenever the decision changes, for analytics |

## Stores

Minimum versions are set per store. This package's native module tells which store installed the app:

- **Android:** the package that installed it. Google Play becomes `play`; another store, such as Galaxy Store, is its own package name. Sideloads have no store.
- **iOS:** App Store installs are `appstore`. TestFlight, development builds and the simulator have no store.

An install with no store is never asked to update, and switches there can only hide. The module never shows an App Store sign-in prompt.

The Update button:

- **Play:** Play's own full-screen update flow. If Play can't run it, your Play listing opens instead.
- **App Store:** your App Store page, in a sheet inside the app.
- **Other stores:** instructions naming the store, unless you pass a link in `storeLinks`.

In Expo Go, or anywhere the native module isn't built in, every install has no store. Pausing and switches still work there.

Building the example for Android needs Java 17; newer versions fail while configuring React Native's native code.

## Try the example

Run the server from `server/`, then from `example/` run `npx expo run:ios`, and use `gatectl` to pause and resume. The example's screen lists the commands.
