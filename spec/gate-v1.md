# Gate contract v1

A gate is a small JSON document. A server publishes it, and an app reads it to
decide whether to run, pause, or ask for an update. It works for any app and
any mix of stores: nothing in it is tied to one app.

The words MUST, MUST NOT, SHOULD and MAY are used as in RFC 2119. Each rule
has a number (R1, R2, …) so the test cases in `cases/` can point to it.

## 1. What the gate is

**R1.** The gate is advisory. It reaches people only when they open the app or
come back to it. It MUST NOT be the only thing that stops money moving or data
changing: the services behind the app must refuse on their own.

**R2.** The server never sends words or links. Every sentence, button label and
store link a person sees MUST come from the app. The gate carries only codes,
version numbers, build numbers, times and true/false values.

**R3.** The app fails open. Anything it cannot read or does not recognise MUST
mean "run", except that a pause already on screen stays (R22).

## 2. The document

```json
{
  "maintenance": { "active": true, "reason": "upgrade", "until": "2026-10-14T18:00:00Z" },
  "stores": {
    "play":                 { "minVersion": "2.4.0", "minBuild": 120 },
    "appstore":             { "minVersion": "2.4.0" },
    "com.huawei.appmarket": { "minBuild": 118 }
  },
  "switches": { "rewards": false, "referrals": true }
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `maintenance` | object, optional | Present only while the app is paused |
| `maintenance.active` | `true` | Required inside `maintenance` |
| `maintenance.reason` | `"upgrade"`, `"outage"` or `"issue"`, optional | Picks which of the app's own sentences to show |
| `maintenance.until` | ISO 8601 time with a time zone, optional | When the operator expects to be back |
| `stores` | object, optional | Minimums, one entry per store key (R4) |
| `stores.<key>.minVersion` | string of 1–4 dotted numbers | Installs below this version are asked to update |
| `stores.<key>.minBuild` | whole number, 1 to 2,100,000,000 | Installs below this build are asked to update |
| `switches` | object, optional | Switch id (R5) to `true` (show) or `false` (hide) |

The machine-readable version is `gate-v1.schema.json`.

**R4.** A store key is `play` (Google Play), `appstore` (Apple's App Store), or
the package name of any other Android store's installer app, such as
`com.huawei.appmarket` (Huawei AppGallery), `com.sec.android.app.samsungapps`
(Galaxy Store) or `com.amazon.venezia` (Amazon Appstore).

**R5.** A switch id is 1 to 32 characters: a lowercase letter, then lowercase
letters, digits, `-` or `_`. Each app decides which ids it has; the gate can
only set ids the app already ships.

**R6.** `{}` is a complete gate. It means: run, ask nobody to update, and leave
every switch at the app's shipped default.

## 3. What the device sends

**R7.** Store, version and build MUST come from the installed native app:
Android's `versionName` and `versionCode`, iOS's `CFBundleShortVersionString`
and `CFBundleVersion`. Never from Expo config or an update manifest: an
over-the-air update changes those, and comparing against them can lock people
out with nothing newer to install.

**R8.** The device's store key comes from how the app was installed:

- Android: the installer package `com.android.vending` is `play`. Any other
  store's installer is its own package name (R4). No installer, or a system
  installer that means a sideload (`com.google.android.packageinstaller`,
  `com.android.packageinstaller`, `com.android.shell`), is no store.
- iOS: an App Store install is `appstore`. TestFlight, the simulator and
  development builds are no store.
- Any other platform (web), or a build without the native module, is no store.

`cases/store/` checks this mapping.

**R9.** Each read sends `Accept: application/json`, `Cache-Control: no-cache`,
`X-Portcullis-Contract: 1`, and, when known, `X-Portcullis-Store`,
`X-Portcullis-Version` (at most 32 characters) and `X-Portcullis-Build` (at
most 16). Nothing about the person, and no cookies.

## 4. How the app decides

The app turns a gate and its own facts into one decision:

```
{ "kind": "run" }
{ "kind": "maintenance", "reason": <code or null>, "until": <time or null> }
{ "kind": "update", "store": <key>, "below": "version" | "build",
  "minimum": <the gate's minimum>, "current": <the device's value> }
```

plus `switches`: the switch values the gate set, after R19 and R20. It checks
in this order: pause, then minimums, then run.

**R10.** The app MUST pause only when `maintenance.active` is the JSON value
`true`. Any other value, including `"true"` and `1`, MUST mean no pause.

**R11.** `reason` is kept when it is `upgrade`, `outage` or `issue`. Anything
else, or nothing, becomes `null`, and the app shows its general sentence.

**R12.** `until` is kept only when it is a string of at most 40 characters that
reads as an ISO 8601 time with a time zone (`Z` or an offset such as `+01:00`),
is after now, and is at most 7 days after now. A time with no zone can't be
read, since it would mean a different moment on every phone.
Otherwise it is `null` and the app shows no time. `until` never ends a pause;
only a new gate does.

**R13.** A pause MUST outrank an update request. When both apply, the app shows
the pause.

**R14.** For minimums, the app reads only the entry whose key is exactly its
own store key, and only a key actually written in the document (so
`__proto__` or `constructor` never match). With no store, or no entry for its
store, the app runs.

**R15.** A version is 1 to 4 dot-separated numbers of up to 6 digits each. The
app MUST ignore a leading lowercase `v` and anything from the first `-` or `+`.
Anything else cannot be read: a space anywhere (including around the version),
a capital `V`, any other letter, or a fifth number. The gate's `minVersion` is
read the same way.

**R16.** Versions compare number by number, left to right, as numbers, so
`1.0.10` is newer than `1.0.9`. A missing number counts as 0, so `1.2` equals
`1.2.0`. A device below `minVersion` gets `update` with `below: "version"`,
showing the digits the app parsed, never the raw string.

**R17.** A build is a whole number from 1 to 2,100,000,000. The device may
give it as a string of digits; the gate's `minBuild` must be a JSON number, and
a string there can't be read. A device below `minBuild` gets `update` with
`below: "build"`. Version is checked first, so a device below both shows the
version. A dotted iOS build such as `"1.0.3"` cannot be read, so `minBuild`
never fires for it: such apps should use `minVersion`.

**R18.** If either side of a check cannot be read, that check MUST NOT ask for
an update.

**R19.** A switch the app ships, set to `true` or `false`, takes that value. An
id the app doesn't ship, or a value that isn't `true` or `false`, is ignored.
A switch the gate doesn't mention stays at the app's shipped default.

**R20.** On `play`, and on an install with no store, the app MUST drop `true`:
there a remote switch can only hide. Google Play treats a reviewed app that
turns features on remotely as a policy violation, and an unknown store is
treated as the strictest one.

## 5. When there is no answer

**R21.** An answer is an HTTP 200 whose body reads as JSON. No network, a
timeout, any other status, or a body that isn't JSON is no answer. A JSON body
that isn't an object (such as `[]`) is an answer, and it means run.

**R22.** With no answer, the app MUST keep its current state: running stays
running, a pause on screen stays paused, and the last switches read stay.

**R23.** Any answer MUST replace the current state, even one that lifts a
pause. On a fresh launch, the current state is run with shipped switches.

## 6. When to check

**R24.** At launch. The app waits at most 3 seconds; after that it is no answer.

**R25.** When the app comes back after 5 minutes or more away. Time away is
measured on both the wall clock and a monotonic clock; either reaching 5
minutes, or the wall clock going backwards, starts a check.

**R26.** The pause screen offers "Check again", which reads the gate at once.
A failed read keeps the pause (R22).

## 7. What the server must do

**R27.** The server answers `GET /v1/gate` with 200 and `application/json`. It
MAY cache the gate in memory for up to 30 seconds. One server serves one app's
gate.

**R28.** Every gate response carries `X-Portcullis-State`: `current` when it is
the gate file as written, `stale` when it is the last good copy.

**R29.** If the gate file cannot be read, isn't JSON, or is refused by section
8, the server MUST serve the last good gate and mark it `stale`. It never
serves part of a gate.

**R30.** The last good gate is kept on disk, so it survives restarts. If there
has never been a good gate, the server serves `{}` marked `stale`.

**R31.** Deleting the gate file means `{}`, marked `current`: it lifts every
stop. A file counts as deleted only once it has been missing for 2 seconds, so
an editor saving it does not lift a pause.

**R32.** The server counts reads by store, version and build (R9) and logs the
counts every 10 minutes, busiest first, then starts again. It keeps nothing
about the person.

## 8. What the validator refuses

The server and `gatectl` MUST refuse the whole gate, and publish nothing from
it, if any rule below is broken. A field set to `null` counts as present and
wrong: to leave something out, leave the field out.

**R33.** It isn't a JSON object.

**R34.** It contains a field this contract doesn't define, including a
top-level `minVersion`, `storeUrl` or `message`. The server never sends words
or links.

**R35.** `maintenance` is present without `"active": true`. To show there is no
pause, leave `maintenance` out.

**R36.** `reason` isn't `upgrade`, `outage` or `issue`.

**R37.** `until` isn't an ISO 8601 time with a time zone, or is longer than 40
characters. A valid time in the past or more than 7 days ahead is accepted
with a warning: the app simply shows no time.

**R38.** A store key doesn't match R4. This catches `ios`, `android`,
`testflight` and typos. It also catches `com.android.vending` (use `play`) and
the sideload installers from R8, since no device ever reports those as its
store.

**R39.** A store entry is empty or has other fields, a `minVersion` has
anything but digits and dots (no `v`, no tail), or a `minBuild` isn't a whole
number from 1 to 2,100,000,000 (a string of digits is refused).

**R40.** A switch id doesn't match R5, or a switch value isn't `true` or
`false`.

`gatectl` SHOULD also warn before saving: how many installs a new minimum
would ask to update (from R32's counts), that a `true` switch won't show on
Play, and that a pause hides any update request.

## 9. Reading the test cases

Each file in `cases/` is a JSON array. Every case has a `name` and the `rule`
it checks.

`cases/decide/` cases give:

- `now`: the time the app decides at.
- `current`: the app's state before this read. Default `{ "kind": "run" }`.
- `switchIds`: the switch ids the app ships. Default `[]`.
- `gate` (the parsed answer), or `reading` when there is no answer:
  `"unreachable"` (no network, timeout or a non-200 status) or `"invalid"`
  (the body isn't JSON).
- `device`: `store` (a key, or `null`), `version` and `build` (strings, or
  `null` when unknown).
- `expect`: the decision.

A runner compares only the fields `expect` contains. `null` in `expect` means
the field must be `null`. Times are compared as instants, so
`2026-10-14T19:00:00+01:00` equals `2026-10-14T18:00:00Z`.

`cases/store/` cases give `platform` and either `installer` (Android) or
`environment` (iOS: `appstore`, `testflight`, `simulator` or `development`),
and `expect` is the store key, or `null`.

`cases/validate/` cases give a `gate` and `expect: "refuse"`. A gate is refused
when the schema rejects it or a rule in section 8 does; the schema alone can't
check everything (R37's time format, for one).
