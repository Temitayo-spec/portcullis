import * as Application from 'expo-application';
import {
  GateProvider,
  readInstallSource,
  useGate,
  type Install,
  type InstallSource,
} from 'expo-portcullis';
import { useEffect, useState, type ReactNode } from 'react';
import { Platform, Pressable, ScrollView, StyleSheet, Switch, Text, View } from 'react-native';

/**
 * Where the example reads its gate. Run the Go server on your Mac (see
 * server/), then: the iOS simulator reaches it as localhost, and the Android
 * emulator as 10.0.2.2. Set EXPO_PUBLIC_GATE_URL to point anywhere else.
 */
const GATE_URL =
  process.env.EXPO_PUBLIC_GATE_URL ??
  (Platform.OS === 'android' ? 'http://10.0.2.2:8080/v1/gate' : 'http://localhost:8080/v1/gate');

/** The switches this app ships, and whether each shows when the gate says nothing. */
const SHIPPED = { rewards: true, referrals: false } as const;
const SWITCH_IDS = Object.keys(SHIPPED);

export default function App() {
  // A simulator, emulator or adb install has no store, so it is never asked to
  // update. This lets the example pretend to be a Play install.
  const [pretendPlay, setPretendPlay] = useState(false);
  const install: Install | undefined = pretendPlay
    ? {
        store: 'play',
        version: Application.nativeApplicationVersion ?? null,
        build: Application.nativeBuildVersion ?? null,
      }
    : undefined;

  return (
    <GateProvider url={GATE_URL} switches={SWITCH_IDS} install={install}>
      <Home pretendPlay={pretendPlay} onPretendPlay={setPretendPlay} />
    </GateProvider>
  );
}

function Home({ pretendPlay, onPretendPlay }: { pretendPlay: boolean; onPretendPlay: (on: boolean) => void }) {
  const gate = useGate();
  // What the native module says about this install, before it becomes a store key.
  const [source, setSource] = useState<InstallSource | null | 'loading'>('loading');
  useEffect(() => {
    readInstallSource().then(setSource);
  }, []);
  const reported =
    source === 'loading'
      ? 'asking…'
      : source === null
        ? 'no native module'
        : source.platform === 'android'
          ? `installer ${source.installer ?? 'none'}`
          : `environment ${source.environment}`;
  const status = !gate.ready
    ? 'Reading the gate…'
    : gate.unreachable
      ? `Couldn't reach ${GATE_URL}, so the app keeps running.`
      : 'The gate says: run.';

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <Text accessibilityRole="header" style={styles.title}>
        Portcullis example
      </Text>
      <Text style={styles.body}>{status}</Text>

      <Section title="Switches">
        {SWITCH_IDS.map((id) => (
          <Text key={id} style={styles.body}>
            {id}: {gate.isShown(id, SHIPPED[id as keyof typeof SHIPPED]) ? 'shown' : 'hidden'}
            {id in gate.switches ? ' (set by the gate)' : ' (app default)'}
          </Text>
        ))}
      </Section>

      <Section title="This install">
        <Text style={styles.body}>
          Version {Application.nativeApplicationVersion ?? 'unknown'}, build {Application.nativeBuildVersion ?? 'unknown'}
        </Text>
        <Text style={styles.body}>Native module: {reported}</Text>
        <Text style={styles.body}>Store: {gate.install ? (gate.install.store ?? 'none') : 'not read yet'}</Text>
        <View style={styles.row}>
          <Text style={[styles.body, styles.grow]}>Pretend this is a Google Play install</Text>
          <Switch
            value={pretendPlay}
            onValueChange={(on) => {
              onPretendPlay(on);
              gate.checkAgain();
            }}
          />
        </View>
      </Section>

      <Pressable accessibilityRole="button" onPress={gate.checkAgain} disabled={gate.checking} style={styles.button}>
        <Text style={styles.buttonText}>{gate.checking ? 'Checking…' : 'Check again'}</Text>
      </Pressable>

      <Section title="Try it">
        {[
          'cd server && go run ./cmd/gatectl pause --reason upgrade --until 2h',
          'cd server && go run ./cmd/gatectl resume',
          'cd server && go run ./cmd/gatectl hide rewards',
          'cd server && go run ./cmd/gatectl min-version play 99.0.0 --yes',
        ].map((line) => (
          <Text key={line} selectable style={styles.code}>
            {line}
          </Text>
        ))}
        <Text style={styles.small}>
          Then tap Check again, or leave the app for 5 minutes. Run the server and gatectl with the same GATE_FILE and
          STATE_DIR.
        </Text>
      </Section>
    </ScrollView>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>{title}</Text>
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  page: { padding: 24, paddingTop: 72, gap: 16 },
  title: { fontSize: 26, fontWeight: '700' },
  section: { gap: 8, padding: 16, borderRadius: 12, backgroundColor: '#F1F2F4' },
  sectionTitle: { fontSize: 13, fontWeight: '600', textTransform: 'uppercase', color: '#5E646E' },
  body: { fontSize: 16, lineHeight: 22 },
  small: { fontSize: 13, lineHeight: 18, color: '#5E646E' },
  code: { fontFamily: Platform.select({ ios: 'Menlo', default: 'monospace' }), fontSize: 12 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  grow: { flex: 1 },
  button: { minHeight: 48, borderRadius: 12, backgroundColor: '#16181D', alignItems: 'center', justifyContent: 'center' },
  buttonText: { color: '#FFFFFF', fontSize: 16, fontWeight: '600' },
});
