# Portcullis

A self-hosted remote gate for Expo apps: pause the app or require an update, per store, without shipping a build. The server can never put words or links on your users' screens. It sends only reason codes and version numbers; every sentence and store link lives in the app.

| Folder | What it is |
| --- | --- |
| [`spec/`](spec) | The contract: the rules in [`gate-v1.md`](spec/gate-v1.md), a JSON schema, and the shared test cases both sides run |
| [`server/`](server) | The Go server and `gatectl`, the command for changing the gate |
| [`expo/`](expo) | [`expo-portcullis`](https://www.npmjs.com/package/expo-portcullis), the library apps wrap themselves in, with an example app |

## Quickstart: a paused app in 15 minutes

You need Docker (Docker Desktop, or [Colima](https://github.com/abiosoft/colima) on a Mac), Node, an Expo app, and a simulator with Expo Go.

**1. Run the server.**

```bash
git clone https://github.com/Temitayo-spec/portcullis.git && cd portcullis/server
```

```bash
docker compose up -d
```

```bash
curl localhost:8080/health
```

It should print `{"gate":"current","lastGood":"saved","ok":true}`.

**2. Add the gate to your app.** In your Expo app's folder:

```bash
npx expo install expo-portcullis expo-application
```

Then wrap your app:

```tsx
import { GateProvider } from 'expo-portcullis';

export default function App() {
  return (
    <GateProvider url="http://localhost:8080/v1/gate">
      {/* your app */}
    </GateProvider>
  );
}
```

On the Android emulator, use `http://10.0.2.2:8080/v1/gate`: that's how the emulator reaches your computer.

**3. Start the app** with `npx expo start`, then press `i` for the iOS simulator or `a` for Android. It runs as normal.

**4. Pause it.** From `portcullis/server`:

```bash
docker compose run --rm gatectl pause --reason upgrade --until 1h
```

Press `r` in the Expo terminal to reload the app. The pause screen stands in for your app, with its "upgrading" sentence and the time it expects to be back.

**5. Bring it back.**

```bash
docker compose run --rm gatectl resume
```

Tap **Check again**, and your app returns.

### What Expo Go can't do

Pausing and switches work everywhere, Expo Go included. Asking for an update needs to know which store installed the app, which takes the library's native module, and Expo Go doesn't have it: there every install has no store, so it's never asked to update. Expo Go also reports its own version on the pause screen. A development build (`npx expo run:ios`) or a store build has both.

### Going live

- Put the server behind HTTPS, on a host your apps can reach. The `data` and `state` volumes keep the gate and its last good copy, so a broken file or a restart never lifts a pause.
- Run `gatectl` on the server with your name, so the audit log says who changed what: `docker compose run --rm -e USER=you gatectl status`.
- Set per-store minimums with `gatectl min-version play 2.4.0 --yes`. It refuses without `--yes` and shows how many recent installs would be asked to update.
- Read [`spec/gate-v1.md`](spec/gate-v1.md) for exactly how the app and server behave, and [`expo/README.md`](expo/README.md) for every `GateProvider` option.

MIT licensed.
