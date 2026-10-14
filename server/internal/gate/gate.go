// Package gate reads, checks and describes a gate document (spec/gate-v1.md).
//
// This is the strict side of the contract: the app forgives a bad gate and
// runs, but the server and gatectl refuse it, whole, before it can reach a
// phone. Validate applies rules R33–R40 and returns either a clean Gate or the
// list of problems, one plain sentence each.
package gate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reasons are the pause reasons the app has a sentence for (R11, R36).
var Reasons = []string{"upgrade", "outage", "issue"}

// MaxBuild is Play's versionCode ceiling (R17).
const MaxBuild = 2_100_000_000

// UntilWindow is how far ahead the app still shows an expected return time (R12).
const UntilWindow = 7 * 24 * time.Hour

var (
	// R4: play, appstore, or another Android store's installer package name.
	storeKey = regexp.MustCompile(`^(play|appstore|[a-z][a-z0-9_]*(\.[a-z0-9_]+)+)$`)
	// R5: a lowercase letter, then up to 31 lowercase letters, digits, - or _.
	switchID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	// R39: one to four groups of up to six digits. No "v", no tail.
	plainVersion = regexp.MustCompile(`^\d{1,6}(\.\d{1,6}){0,3}$`)
	// R37: an RFC 3339 time with seconds and an explicit zone.
	zonedTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$`)
)

// Gate is a gate that passed Validate. Encoding it gives the JSON the server
// serves: empty parts are left out, so the zero Gate encodes as {}.
type Gate struct {
	Maintenance *Maintenance        `json:"maintenance,omitempty"`
	Stores      map[string]Minimums `json:"stores,omitempty"`
	Switches    map[string]bool     `json:"switches,omitempty"`
}

// Maintenance is a pause. Active is always true: "not paused" is no Maintenance.
type Maintenance struct {
	Active bool   `json:"active"`
	Reason string `json:"reason,omitempty"`
	Until  string `json:"until,omitempty"`
}

// Minimums are one store's lowest allowed version and build. Zero means unset.
type Minimums struct {
	MinVersion string `json:"minVersion,omitempty"`
	MinBuild   int64  `json:"minBuild,omitempty"`
}

// Result is what Validate found. Gate is only meaningful when Problems is empty.
type Result struct {
	Gate     Gate
	Problems []string // each one refuses the whole gate
	Warnings []string // allowed, but worth reading before saving
}

// OK reports whether the gate may be served or saved.
func (r Result) OK() bool { return len(r.Problems) == 0 }

// IsStoreKey reports whether key is a store key the contract allows (R4).
func IsStoreKey(key string) bool { return storeKey.MatchString(key) }

// IsSwitchID reports whether id is a switch id the contract allows (R5).
func IsSwitchID(id string) bool { return switchID.MatchString(id) }

// IsPlainVersion reports whether v is a version a gate may carry (R39).
func IsPlainVersion(v string) bool { return plainVersion.MatchString(v) }

// Validate checks raw JSON against rules R33–R40. now is used only for the
// warnings about an expected return time in the past or far ahead.
func Validate(raw []byte, now time.Time) Result {
	var r Result
	if len(bytes.TrimSpace(raw)) == 0 {
		r.problem("The gate is empty. Write {} for a gate that lets the app run.")
		return r
	}

	// UseNumber keeps numbers as written, so 120 and 1.5 can be told apart.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		r.problem(fmt.Sprintf("The gate is not valid JSON: %v.", err))
		return r
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		r.problem("The gate has more text after its closing brace.")
		return r
	}

	top, ok := doc.(map[string]any)
	if !ok {
		r.problem("The gate must be a JSON object, like {}.") // R33
		return r
	}

	for _, key := range sortedKeys(top) {
		switch key {
		case "maintenance":
			r.Gate.Maintenance = r.readMaintenance(top[key], now)
		case "stores":
			r.Gate.Stores = r.readStores(top[key])
		case "switches":
			r.Gate.Switches = r.readSwitches(top[key])
		case "minVersion":
			r.problem(`A single top-level "minVersion" is not allowed: set one per store under "stores".`) // R34
		case "storeUrl":
			r.problem(`"storeUrl" is not allowed: the app owns its store links.`) // R34, R2
		case "message":
			r.problem(`"message" is not allowed: the app owns every sentence it shows.`) // R34, R2
		default:
			r.problem(fmt.Sprintf("%q is not a gate field. Use maintenance, stores or switches.", key)) // R34
		}
	}

	if r.Gate.Maintenance != nil && len(r.Gate.Stores) > 0 {
		r.warn("The app is paused, so it shows the pause and not the update screen.")
	}
	return r
}

func (r *Result) readMaintenance(value any, now time.Time) *Maintenance {
	m, ok := value.(map[string]any)
	if !ok {
		r.problem(`maintenance must be an object like { "active": true }.`)
		return nil
	}
	out := &Maintenance{Active: true}
	for _, key := range sortedKeys(m) {
		switch key {
		case "active":
			switch m[key] {
			case true:
			case false:
				r.problem(`maintenance.active is false: to lift a pause, leave "maintenance" out.`) // R35
			default:
				r.problem("maintenance.active must be true, with no quotes.") // R35
			}
		case "reason":
			reason, _ := m[key].(string)
			if !contains(Reasons, reason) {
				r.problem(fmt.Sprintf("maintenance.reason must be one of %s.", strings.Join(Reasons, ", "))) // R36
			}
			out.Reason = reason
		case "until":
			out.Until = r.readUntil(m[key], now)
		case "message":
			r.problem(`maintenance.message is not allowed: pick a "reason" instead.`) // R34, R2
		default:
			r.problem(fmt.Sprintf("%q is not a maintenance field. Use active, reason or until.", "maintenance."+key)) // R34
		}
	}
	if _, has := m["active"]; !has {
		r.problem(`maintenance needs "active": true. To lift a pause, leave "maintenance" out.`) // R35
	}
	return out
}

func (r *Result) readUntil(value any, now time.Time) string {
	text, _ := value.(string)
	if len(text) > 40 || !zonedTime.MatchString(text) {
		r.problem(`maintenance.until must be a time with its zone, like "2026-10-14T18:00:00Z".`) // R37
		return ""
	}
	at, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		r.problem(fmt.Sprintf("maintenance.until is not a real time: %v.", err)) // R37
		return ""
	}
	switch {
	case !at.After(now):
		r.warn("maintenance.until is already past, so the app shows no time. The pause still stays until you resume.")
	case at.Sub(now) > UntilWindow:
		r.warn("maintenance.until is more than 7 days away. The app shows it once it is within 7 days.")
	}
	return at.UTC().Format(time.RFC3339)
}

func (r *Result) readStores(value any) map[string]Minimums {
	stores, ok := value.(map[string]any)
	if !ok {
		r.problem(`stores must be an object, like { "play": { "minVersion": "2.4.0" } }.`)
		return nil
	}
	out := map[string]Minimums{}
	for _, key := range sortedKeys(stores) {
		if !IsStoreKey(key) {
			r.problem(fmt.Sprintf("%q is not a store key. Use play, appstore, or another Android store's package name.%s", key, storeHint(key))) // R38
			continue
		}
		entry, ok := stores[key].(map[string]any)
		if !ok || len(entry) == 0 {
			r.problem(fmt.Sprintf(`stores.%s must set "minVersion", "minBuild" or both.`, key)) // R39
			continue
		}
		var min Minimums
		for _, field := range sortedKeys(entry) {
			switch field {
			case "minVersion":
				v, _ := entry[field].(string)
				if !IsPlainVersion(v) {
					r.problem(fmt.Sprintf(`stores.%s.minVersion must be plain digits and dots, like "2.4.0" (no "v", no tail).`, key)) // R39
					continue
				}
				min.MinVersion = v
				r.warn(fmt.Sprintf("Every %s install below version %s will be asked to update. First make sure that version is live for everyone in that store.", StoreName(key), v))
			case "minBuild":
				b, ok := readBuild(entry[field])
				if !ok {
					r.problem(fmt.Sprintf("stores.%s.minBuild must be a whole number from 1 to %d, with no quotes.", key, MaxBuild)) // R39
					continue
				}
				min.MinBuild = b
				r.warn(fmt.Sprintf("Every %s install below build %d will be asked to update. First make sure that build is live for everyone in that store.", StoreName(key), b))
			default:
				r.problem(fmt.Sprintf("%q is not a store field. Use minVersion or minBuild.", "stores."+key+"."+field)) // R39
			}
		}
		if min != (Minimums{}) {
			out[key] = min
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *Result) readSwitches(value any) map[string]bool {
	switches, ok := value.(map[string]any)
	if !ok {
		r.problem(`switches must be an object, like { "rewards": false }.`)
		return nil
	}
	out := map[string]bool{}
	for _, id := range sortedKeys(switches) {
		if !IsSwitchID(id) {
			r.problem(fmt.Sprintf("%q is not a switch id: a lowercase letter, then up to 31 lowercase letters, digits, - or _.", id)) // R40
			continue
		}
		shown, ok := switches[id].(bool)
		if !ok {
			r.problem(fmt.Sprintf("switches.%s must be true or false, with no quotes.", id)) // R40
			continue
		}
		out[id] = shown
		if shown {
			r.warn(fmt.Sprintf("switches.%s is true: Play installs and installs with no store ignore it and keep it hidden.", id)) // R20
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// readBuild accepts only a JSON number that is a whole number in range.
func readBuild(value any) (int64, bool) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	b, err := n.Int64()
	if err != nil || b < 1 || b > MaxBuild {
		return 0, false
	}
	return b, true
}

// CompareVersions compares two plain versions number by number, a missing
// number counting as 0 (R16). ok is false if either isn't a plain version.
func CompareVersions(a, b string) (cmp int, ok bool) {
	if !IsPlainVersion(a) || !IsPlainVersion(b) {
		return 0, false
	}
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		x, y := 0, 0
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

// StoreHint is a nudge toward the right store key for a common wrong one.
func StoreHint(key string) string { return storeHint(key) }

// StoreName is what a person would call a store.
func StoreName(key string) string {
	switch key {
	case "play":
		return "Google Play"
	case "appstore":
		return "App Store"
	default:
		return key
	}
}

func storeHint(key string) string {
	switch strings.ToLower(key) {
	case "ios", "apple", "app-store", "appstore":
		return ` For Apple's App Store, use "appstore".`
	case "android", "google", "googleplay", "google-play", "play":
		return ` For Google Play, use "play".`
	case "testflight":
		return " TestFlight installs are never asked to update."
	}
	return ""
}

// Describe says in one line what the app will do with a gate.
func Describe(g Gate) string {
	var parts []string
	if m := g.Maintenance; m != nil {
		s := "paused"
		if m.Reason != "" {
			s += " (" + m.Reason + ")"
		}
		if m.Until != "" {
			s += `, showing "expected back" ` + m.Until
		}
		parts = append(parts, s+", until you run resume")
	}
	for _, key := range sortedKeys(g.Stores) {
		min := g.Stores[key]
		var below []string
		if min.MinVersion != "" {
			below = append(below, "version "+min.MinVersion)
		}
		if min.MinBuild != 0 {
			below = append(below, fmt.Sprintf("build %d", min.MinBuild))
		}
		parts = append(parts, fmt.Sprintf("%s installs below %s are asked to update", StoreName(key), strings.Join(below, " or ")))
	}
	for _, id := range sortedKeys(g.Switches) {
		if g.Switches[id] {
			parts = append(parts, id+" is shown, except on Play and installs with no store")
		} else {
			parts = append(parts, id+" is hidden everywhere")
		}
	}
	if len(parts) == 0 {
		return "the app runs normally"
	}
	return strings.Join(parts, "; ")
}

// Encode returns the gate as compact JSON with a trailing newline.
func Encode(g Gate) []byte {
	b, _ := json.Marshal(g) // a Gate holds only strings, numbers and bools
	return append(b, '\n')
}

func (r *Result) problem(s string) { r.Problems = append(r.Problems, s) }
func (r *Result) warn(s string)    { r.Warnings = append(r.Warnings, s) }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
