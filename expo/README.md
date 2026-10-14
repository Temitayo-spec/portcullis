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
| `storeLinks` | Where each store's Update button goes, such as `{ appstore: 'https://apps.apple.com/app/id123' }`. Play defaults to your app's own listing |
| `renderPause`, `renderUpdate` | Replace the default screens |
| `onDecision` | Called whenever the decision changes, for analytics |

## Stores

Minimum versions are set per store. Telling which store installed the app needs this package's native module, which is still being built. Until then every install has no store, so it is never asked to update, and switches can only hide. Pausing works everywhere, including Expo Go.

## Try the example

Run the server from `server/`, then from `example/` run `npx expo run:ios`, and use `gatectl` to pause and resume. The example's screen lists the commands.
