# Portcullis

A self-hosted remote gate for Expo apps: pause the app or require an update, per store, without shipping a build. The server can never put words or links on your users' screens. It sends only reason codes and version numbers; every sentence and store link lives in the app.

| Folder | What it is |
| --- | --- |
| [`spec/`](spec) | The contract: the rules in [`gate-v1.md`](spec/gate-v1.md), a JSON schema, and the shared test cases both sides run |
| [`server/`](server) | The Go server and `gatectl`, the command for changing the gate |
| [`expo/`](expo) | `expo-portcullis`, the library apps wrap themselves in, with an example app |

Status: v1 in progress. The spec, server, `gatectl`, library and native module are built and tested on the iOS simulator and an Android emulator. Next: using it in real apps, then publishing.

MIT licensed.
