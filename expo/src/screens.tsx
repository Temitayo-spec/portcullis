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

import { PortcullisNative } from './PortcullisModule';
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
  /** This app's numeric App Store id; looked up from the bundle id when left out. */
  appStoreId?: string;
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
export function UpdateScreen({
  decision,
  messages,
  storeLinks,
  appStoreId,
  version,
}: UpdateScreenProps) {
  const colors = useColors();
  const [failed, setFailed] = useState(false);
  const action = updateAction(decision.store, storeLinks, appStoreId);
  const storeName = messages.storeNames[decision.store] ?? 'your app store';
  return (
    <Frame colors={colors} version={version} messages={messages}>
      <Text accessibilityRole="header" style={[styles.title, { color: colors.text }]}>
        {messages.updateTitle}
      </Text>
      <Text style={[styles.body, { color: colors.muted }]}>
        {messages.updateBody(decision.below, String(decision.minimum), String(decision.current))}
      </Text>
      {action ? (
        <Button
          colors={colors}
          label={messages.updateButton}
          onPress={() => {
            action().then((opened) => setFailed(!opened));
          }}
        />
      ) : null}
      {!action || failed ? (
        <Text style={[styles.body, { color: colors.text }]}>
          {messages.updateInstructions(storeName)}
        </Text>
      ) : null}
    </Frame>
  );
}

/**
 * What the Update button does for this store, or null for no button. Every
 * destination is owned by the app, never sent by the server (R2):
 *
 * - a link the app passed in `storeLinks` wins;
 * - Play: Play's own update screen (native module), else the app's listing;
 * - App Store: the app's page in an in-app sheet (native module).
 *
 * Resolves false when nothing could be opened, so the screen shows instructions.
 */
function updateAction(
  store: string,
  storeLinks: Readonly<Record<string, string>> | undefined,
  appStoreId: string | undefined
): (() => Promise<boolean>) | null {
  const link = storeLinks?.[store];
  if (link) return () => openFirst([link]);
  if (store === 'play') {
    const startPlayUpdate = PortcullisNative?.startPlayUpdateAsync;
    const listing = playListing();
    if (startPlayUpdate) {
      return async () => {
        const result = await startPlayUpdate().catch(() => 'failed' as const);
        // Accepted or cancelled: Play showed its screen, and the person chose.
        return result === 'accepted' || result === 'cancelled' || openFirst(listing);
      };
    }
    return listing.length > 0 ? () => openFirst(listing) : null;
  }
  const openAppStorePage = PortcullisNative?.openAppStorePageAsync;
  if (store === 'appstore' && openAppStorePage) {
    return () => openAppStorePage(appStoreId ?? null).catch(() => false);
  }
  return null;
}

/** Tries each link in turn: the store app first, then its web page. False if none opened. */
async function openFirst(links: readonly string[]): Promise<boolean> {
  for (const link of links) {
    try {
      await Linking.openURL(link);
      return true;
    } catch {
      // try the next one
    }
  }
  return false;
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
