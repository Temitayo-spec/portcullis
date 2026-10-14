import { useEffect, useState, type ReactNode } from 'react';
import {
  Linking,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  useColorScheme,
  View,
} from 'react-native';

import type { Decision } from './decide';
import { playListing } from './install';
import type { Messages } from './messages';

export interface PauseScreenProps {
  decision: Extract<Decision, { kind: 'maintenance' }>;
  messages: Messages;
  checking: boolean;
  /** The last "Check again" reached nobody, so the pause stands. */
  unreachable: boolean;
  onCheckAgain: () => void;
  /** This install's version, for the small line at the bottom. */
  version: string | null;
}

export interface UpdateScreenProps {
  decision: Extract<Decision, { kind: 'update' }>;
  messages: Messages;
  storeLinks?: Readonly<Record<string, string>>;
  version: string | null;
}

/** The default pause screen. Replace it with GateProvider's `renderPause`. */
export function PauseScreen({
  decision,
  messages,
  checking,
  unreachable,
  onCheckAgain,
  version,
}: PauseScreenProps) {
  const colors = useColors();
  const now = useNow(30_000); // so a promised time that passes is taken down
  const backBy = decision.until ? messages.backBy(new Date(decision.until), new Date(now)) : null;
  return (
    <Frame colors={colors} version={version} messages={messages}>
      <Text accessibilityRole="header" style={[styles.title, { color: colors.text }]}>
        {messages.pauseTitle}
      </Text>
      <Text style={[styles.body, { color: colors.muted }]}>
        {messages.pauseReasons[decision.reason ?? 'general']}
      </Text>
      {backBy ? <Text style={[styles.small, { color: colors.muted }]}>{backBy}</Text> : null}
      <Button
        colors={colors}
        label={checking ? messages.checking : messages.checkAgain}
        onPress={onCheckAgain}
        disabled={checking}
      />
      {unreachable ? (
        <Text
          accessibilityRole="alert"
          accessibilityLiveRegion="polite"
          style={[styles.small, { color: colors.muted }]}>
          {messages.stillPaused}
        </Text>
      ) : null}
    </Frame>
  );
}

/** The default update screen. Replace it with GateProvider's `renderUpdate`. */
export function UpdateScreen({ decision, messages, storeLinks, version }: UpdateScreenProps) {
  const colors = useColors();
  const links = storeLinks?.[decision.store]
    ? [storeLinks[decision.store]!]
    : decision.store === 'play'
      ? playListing()
      : [];
  const storeName = messages.storeNames[decision.store] ?? 'your app store';
  return (
    <Frame colors={colors} version={version} messages={messages}>
      <Text accessibilityRole="header" style={[styles.title, { color: colors.text }]}>
        {messages.updateTitle}
      </Text>
      <Text style={[styles.body, { color: colors.muted }]}>
        {messages.updateBody(decision.below, String(decision.minimum), String(decision.current))}
      </Text>
      {links.length > 0 ? (
        <Button
          colors={colors}
          label={messages.updateButton}
          onPress={() => {
            openFirst(links);
          }}
        />
      ) : (
        <Text style={[styles.body, { color: colors.text }]}>
          {messages.updateInstructions(storeName)}
        </Text>
      )}
    </Frame>
  );
}

/** Tries each link in turn: the store app first, then its web page. */
async function openFirst(links: readonly string[]): Promise<void> {
  for (const link of links) {
    try {
      await Linking.openURL(link);
      return;
    } catch {
      // try the next one
    }
  }
}

type Colors = {
  background: string;
  text: string;
  muted: string;
  button: string;
  buttonText: string;
};

function useColors(): Colors {
  return useColorScheme() === 'dark'
    ? {
        background: '#111214',
        text: '#F4F5F7',
        muted: '#A6ACB6',
        button: '#F4F5F7',
        buttonText: '#111214',
      }
    : {
        background: '#FFFFFF',
        text: '#16181D',
        muted: '#5E646E',
        button: '#16181D',
        buttonText: '#FFFFFF',
      };
}

function useNow(everyMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), everyMs);
    return () => clearInterval(id);
  }, [everyMs]);
  return now;
}

function Frame({
  colors,
  version,
  messages,
  children,
}: {
  colors: Colors;
  version: string | null;
  messages: Messages;
  children: ReactNode;
}) {
  // Scrolls, so large text on a small screen never runs off the edge.
  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: colors.background }}
      contentContainerStyle={styles.frame}>
      <View style={styles.column}>{children}</View>
      {version ? (
        <Text style={[styles.version, { color: colors.muted }]}>
          {messages.versionLine(version)}
        </Text>
      ) : null}
    </ScrollView>
  );
}

function Button({
  colors,
  label,
  onPress,
  disabled = false,
}: {
  colors: Colors;
  label: string;
  onPress: () => void;
  disabled?: boolean;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled, busy: disabled }}
      disabled={disabled}
      onPress={onPress}
      style={({ pressed }) => [
        styles.button,
        { backgroundColor: colors.button, opacity: disabled ? 0.5 : pressed ? 0.8 : 1 },
      ]}>
      <Text style={[styles.buttonText, { color: colors.buttonText }]}>{label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  frame: {
    flexGrow: 1,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 32,
    paddingVertical: 64,
  },
  column: { width: '100%', maxWidth: 340, alignItems: 'center', gap: 14 },
  title: { fontSize: 26, fontWeight: '700', textAlign: 'center' },
  body: { fontSize: 16, lineHeight: 23, textAlign: 'center' },
  small: { fontSize: 14, lineHeight: 20, textAlign: 'center' },
  button: {
    marginTop: 18,
    minHeight: 48,
    alignSelf: 'stretch',
    borderRadius: 12,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 20,
  },
  buttonText: { fontSize: 16, fontWeight: '600' },
  version: { marginTop: 40, fontSize: 12, textAlign: 'center' },
});
