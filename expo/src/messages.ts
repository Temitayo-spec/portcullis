/**
 * Every word the gate screens show. They ship inside the app: the server can
 * only pick among them with a reason code, never send its own (R2). Pass
 * `messages` to GateProvider to use your app's name, voice or language.
 */
import type { Reason } from './decide';

export interface Messages {
  pauseTitle: string;
  /** The sentence under the title, by reason. `general` is for no reason or an unknown one. */
  pauseReasons: Readonly<Record<Reason | 'general', string>>;
  /** "Expected back by 7:05 PM". Return null to show no time. */
  backBy: (until: Date, now: Date) => string | null;
  checkAgain: string;
  checking: string;
  /** Shown when "Check again" couldn't reach the server, so the pause stands. */
  stillPaused: string;
  updateTitle: string;
  /** The sentence under the title. `minimum` and `current` are the parsed numbers, never server text. */
  updateBody: (below: 'version' | 'build', minimum: string, current: string) => string;
  updateButton: string;
  /** Shown instead of a button when the app has no link for that store. */
  updateInstructions: (storeName: string) => string;
  /** What a person calls a store. Unknown Android stores fall back to "your app store". */
  storeNames: Readonly<Record<string, string>>;
  /** The small line at the bottom, such as "Version 2.4.0". */
  versionLine: (version: string) => string;
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** "7:05 PM", built by hand: Hermes' Intl isn't something to lean on during an outage. */
function clock(at: Date): string {
  const hours = at.getHours();
  const minutes = String(at.getMinutes()).padStart(2, '0');
  // A non-breaking space, so a narrow screen never strands "PM" on its own line.
  return `${hours % 12 === 0 ? 12 : hours % 12}:${minutes}\u00a0${hours < 12 ? 'AM' : 'PM'}`;
}

export const DEFAULT_MESSAGES: Messages = {
  pauseTitle: 'Paused for maintenance',
  pauseReasons: {
    upgrade: "We're upgrading the app. It will be back shortly.",
    outage: "A service the app relies on is down. We're working to bring it back.",
    issue: "We've paused the app while we fix a problem.",
    general: "We're making some changes. The app will be back shortly.",
  },
  backBy: (until, now) => {
    if (until.getTime() <= now.getTime()) return null; // never promise a time already gone
    const sameDay = until.toDateString() === now.toDateString();
    return sameDay
      ? `Expected back by ${clock(until)}.`
      : `Expected back on ${until.getDate()} ${MONTHS[until.getMonth()]} at ${clock(until)}.`;
  },
  checkAgain: 'Check again',
  checking: 'Checking',
  stillPaused: "Still paused. We couldn't reach the server just now.",
  updateTitle: 'Time to update',
  updateBody: (below, minimum, current) =>
    below === 'version'
      ? `This version (${current}) is no longer supported. Update to version ${minimum} or newer to keep going.`
      : `This build (${current}) is no longer supported. Update to build ${minimum} or newer to keep going.`,
  updateButton: 'Update',
  updateInstructions: (storeName) => `Open ${storeName} and update the app to the latest version.`,
  storeNames: {
    play: 'Google Play',
    appstore: 'the App Store',
    'com.huawei.appmarket': 'AppGallery',
    'com.sec.android.app.samsungapps': 'Galaxy Store',
    'com.amazon.venezia': 'the Amazon Appstore',
  },
  versionLine: (version) => `Version ${version}`,
};
