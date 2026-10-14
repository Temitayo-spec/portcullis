// Package ctl is gatectl: the one way to change the gate.
//
// Every change starts from the gate phones are being served, is checked by
// the same rules the server serves by (gate.Validate), and is written in one
// step. So gatectl can never write a gate the server would refuse, and fixing
// a broken file never lifts the pause in force. A minimum or a reset needs
// --yes, because a wrong one strands installs or lifts every stop.
package ctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/gate"
	"github.com/Temitayo-spec/portcullis/server/internal/gatefile"
)

// Usage lists every command.
const Usage = `Usage: gatectl <command>

  status                                     what phones are being served
  pause [--reason upgrade|outage|issue] [--until 2h|90m|3d|<time with zone>]
  resume                                     lift the pause
  min-version <store> <version|clear> [--yes]
  min-build <store> <number|clear> [--yes]
  hide <switch> | show <switch> | unset <switch>
  reset [--yes]                              lift everything: {}

  <store> is play, appstore, or another Android store's package name.
  Settings: GATE_FILE (default /data/gate.json), STATE_DIR (default /state).`

// Env is everything gatectl reads from outside, so tests can supply it.
type Env struct {
	GateFile, StateDir, User string
	Now                      func() time.Time
	Local                    *time.Location // for showing times; UTC is always shown too
	Stdout, Stderr           io.Writer
}

type command struct {
	verb    string
	reason  string
	until   string
	store   string
	version string
	build   int64
	clear   bool
	id      string
	yes     bool
}

// Run carries out one command. It returns the exit code: 0 done, 1 refused or
// failed, 2 the command itself was wrong.
func Run(args []string, env Env) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(env.Stdout, Usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	cmd, err := parse(args, env.Now())
	if err != nil {
		fmt.Fprintf(env.Stderr, "%v\n\n%s\n", err, Usage)
		return 2
	}

	file := readFile(env.GateFile, env.Now())
	lastGood, haveLastGood := gatefile.ReadLastGood(filepath.Join(env.StateDir, "last-good.json"), env.Now())

	if cmd.verb == "status" {
		return status(env, file, lastGood, haveLastGood)
	}

	// Start from what phones are being served. A broken file is replaced from
	// the last good gate, so fixing it never lifts the stop in force.
	current := file.gate
	if file.broken != "" && cmd.verb != "reset" {
		switch {
		case haveLastGood:
			fmt.Fprintf(env.Stdout, "The gate file can't be used (%s), so this starts from the last good gate.\n", file.broken)
			current = lastGood
		case cmd.verb == "pause":
			current = gate.Gate{}
		default:
			fmt.Fprintf(env.Stderr, "The gate file can't be used (%s), and there's no last good gate to start from.\n", file.broken)
			fmt.Fprintln(env.Stderr, "Run `gatectl reset --yes` to start from a gate that lets the app run, or `gatectl pause` to write a pause.")
			return 1
		}
	}

	next := apply(current, cmd)
	body, _ := json.MarshalIndent(next, "", "  ")
	body = append(body, '\n')
	result := gate.Validate(body, env.Now()) // the same check the server makes
	if !result.OK() {
		report(env, result.Gate, result.Problems, result.Warnings)
		fmt.Fprintln(env.Stderr, "Not saved.")
		return 1
	}

	if !cmd.yes {
		switch {
		case setsMinimum(cmd):
			report(env, result.Gate, nil, result.Warnings)
			showCounts(env, cmd)
			fmt.Fprintf(env.Stderr, "Not saved. Add --yes once that release is live for everyone in %s.\n", gate.StoreName(cmd.store))
			return 1
		case cmd.verb == "reset":
			fmt.Fprintln(env.Stderr, "Not saved. reset lifts every pause, minimum and switch. Add --yes to do it.")
			return 1
		}
	}

	err = os.MkdirAll(filepath.Dir(env.GateFile), 0o755)
	if err == nil {
		err = gatefile.WriteAtomic(env.GateFile, body)
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "Couldn't write %s (%v). Nothing changed.\n", env.GateFile, err)
		fmt.Fprintf(env.Stderr, "Its folder must be writable by user %d.\n", os.Getuid())
		return 1
	}
	if err := audit(env, args, current, next); err != nil {
		fmt.Fprintf(env.Stderr, "Saved, but the audit log couldn't be written: %v\n", err)
	}

	report(env, result.Gate, nil, result.Warnings)
	if cmd.verb == "pause" && cmd.until != "" {
		fmt.Fprintln(env.Stdout, "The time is only what the screen shows. The pause stays until you run resume.")
	}
	fmt.Fprintln(env.Stdout, "Saved. Phones pick it up at their next launch, or when they come back after 5 minutes away.")
	return 0
}

func parse(args []string, now time.Time) (command, error) {
	cmd := command{verb: args[0]}
	var positional []string
	flags := map[string]string{}
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		word := rest[i]
		if !strings.HasPrefix(word, "--") {
			positional = append(positional, word)
			continue
		}
		name, value, hasValue := strings.Cut(word, "=")
		switch name {
		case "--yes":
			if hasValue {
				return cmd, errors.New("--yes takes no value")
			}
			flags[name] = ""
		case "--reason", "--until":
			if !hasValue {
				if i+1 >= len(rest) || strings.HasPrefix(rest[i+1], "--") {
					return cmd, fmt.Errorf("%s needs a value", name)
				}
				i++
				value = rest[i]
			}
			flags[name] = value
		default:
			return cmd, fmt.Errorf("unknown option %s", name)
		}
	}
	_, cmd.yes = flags["--yes"]

	allow := func(allowed []string, positionals int) error {
		for name := range flags {
			if !contains(allowed, name) {
				return fmt.Errorf("%s doesn't take %s", cmd.verb, name)
			}
		}
		if len(positional) != positionals {
			return fmt.Errorf("%s takes %d word(s) after it, got %d", cmd.verb, positionals, len(positional))
		}
		return nil
	}

	switch cmd.verb {
	case "status", "resume":
		return cmd, allow(nil, 0)
	case "reset":
		return cmd, allow([]string{"--yes"}, 0)
	case "pause":
		if err := allow([]string{"--reason", "--until"}, 0); err != nil {
			return cmd, err
		}
		if reason, ok := flags["--reason"]; ok {
			if !contains(gate.Reasons, reason) {
				return cmd, fmt.Errorf("--reason must be one of %s", strings.Join(gate.Reasons, ", "))
			}
			cmd.reason = reason
		}
		if until, ok := flags["--until"]; ok {
			at, err := ParseUntil(until, now)
			if err != nil {
				return cmd, err
			}
			cmd.until = at
		}
		return cmd, nil
	case "hide", "show", "unset":
		if err := allow(nil, 1); err != nil {
			return cmd, err
		}
		cmd.id = positional[0]
		if !gate.IsSwitchID(cmd.id) {
			return cmd, fmt.Errorf("%q isn't a switch id: a lowercase letter, then up to 31 lowercase letters, digits, - or _", cmd.id)
		}
		return cmd, nil
	case "min-version", "min-build":
		if err := allow([]string{"--yes"}, 2); err != nil {
			return cmd, err
		}
		cmd.store, cmd.clear = positional[0], positional[1] == "clear"
		if !gate.IsStoreKey(cmd.store) {
			return cmd, fmt.Errorf("%q isn't a store key: use play, appstore, or another Android store's package name.%s", cmd.store, gate.StoreHint(cmd.store))
		}
		if cmd.clear {
			return cmd, nil
		}
		if cmd.verb == "min-version" {
			cmd.version = positional[1]
			if !gate.IsPlainVersion(cmd.version) {
				return cmd, errors.New(`min-version needs plain digits and dots, like 2.4.0 (no "v", no tail), or "clear"`)
			}
			return cmd, nil
		}
		b, err := strconv.ParseInt(positional[1], 10, 64)
		if err != nil || b < 1 || b > gate.MaxBuild {
			return cmd, fmt.Errorf(`min-build needs a whole number from 1 to %d, or "clear"`, gate.MaxBuild)
		}
		cmd.build = b
		return cmd, nil
	}
	return cmd, fmt.Errorf("unknown command %q", cmd.verb)
}

var relative = regexp.MustCompile(`^(\d{1,4})([mhd])$`)

// ParseUntil turns "90m", "2h", "3d" or a time with its zone into a UTC time.
func ParseUntil(value string, now time.Time) (string, error) {
	if m := relative.FindStringSubmatch(value); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		if n == 0 {
			return "", errors.New("--until must be in the future")
		}
		return now.Add(time.Duration(n) * unit).UTC().Format(time.RFC3339), nil
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", errors.New(`--until must be like 90m, 2h, 3d, or a time with its zone like 2026-10-14T18:00:00Z`)
	}
	return at.UTC().Format(time.RFC3339), nil
}

// apply returns the gate after one command. It never changes current.
func apply(current gate.Gate, cmd command) gate.Gate {
	next := gate.Gate{Maintenance: current.Maintenance, Stores: map[string]gate.Minimums{}, Switches: map[string]bool{}}
	for k, v := range current.Stores {
		next.Stores[k] = v
	}
	for k, v := range current.Switches {
		next.Switches[k] = v
	}
	switch cmd.verb {
	case "reset":
		return gate.Gate{}
	case "pause":
		next.Maintenance = &gate.Maintenance{Active: true, Reason: cmd.reason, Until: cmd.until}
	case "resume":
		next.Maintenance = nil
	case "hide":
		next.Switches[cmd.id] = false
	case "show":
		next.Switches[cmd.id] = true
	case "unset":
		delete(next.Switches, cmd.id)
	case "min-version", "min-build":
		min := next.Stores[cmd.store]
		switch {
		case cmd.verb == "min-version" && cmd.clear:
			min.MinVersion = ""
		case cmd.verb == "min-version":
			min.MinVersion = cmd.version
		case cmd.clear:
			min.MinBuild = 0
		default:
			min.MinBuild = cmd.build
		}
		if min == (gate.Minimums{}) {
			delete(next.Stores, cmd.store)
		} else {
			next.Stores[cmd.store] = min
		}
	}
	if len(next.Stores) == 0 {
		next.Stores = nil
	}
	if len(next.Switches) == 0 {
		next.Switches = nil
	}
	return next
}

func setsMinimum(cmd command) bool {
	return (cmd.verb == "min-version" || cmd.verb == "min-build") && !cmd.clear
}

type fileReading struct {
	gate    gate.Gate
	missing bool
	broken  string // why the file can't be used, or ""
}

func readFile(path string, now time.Time) fileReading {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileReading{missing: true}
	}
	if err != nil {
		return fileReading{broken: "it can't be read: " + err.Error()}
	}
	r := gate.Validate(raw, now)
	if !r.OK() {
		return fileReading{broken: strings.Join(r.Problems, " ")}
	}
	return fileReading{gate: r.Gate}
}

func status(env Env, file fileReading, lastGood gate.Gate, haveLastGood bool) int {
	switch {
	case file.missing:
		fmt.Fprintln(env.Stdout, "There's no gate file, so the app runs normally.")
		return 0
	case file.broken != "":
		fmt.Fprintf(env.Stdout, "The gate file can't be used: %s\n", file.broken)
		if haveLastGood {
			fmt.Fprintln(env.Stdout, "Phones are being served the last good gate:")
			report(env, lastGood, nil, nil)
		}
		fmt.Fprintln(env.Stdout, "Any command except reset fixes the file, starting from that gate.")
		return 1
	}
	report(env, file.gate, nil, nil)
	return 0
}

func report(env Env, g gate.Gate, problems, warnings []string) {
	fmt.Fprintf(env.Stdout, "The app will: %s.\n", gate.Describe(g))
	if g.Maintenance != nil && g.Maintenance.Until != "" {
		fmt.Fprintf(env.Stdout, "  Shown as expected back: %s.\n", describeTime(g.Maintenance.Until, env.Local))
	}
	fmt.Fprintf(env.Stdout, "  Gate: %s", gate.Encode(g))
	for _, p := range problems {
		fmt.Fprintf(env.Stdout, "  problem: %s\n", p)
	}
	for _, w := range warnings {
		fmt.Fprintf(env.Stdout, "  note: %s\n", w)
	}
}

func describeTime(value string, local *time.Location) string {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	utc := at.UTC().Format("Mon 2 Jan 15:04") + " UTC"
	if local == nil || local == time.UTC {
		return utc
	}
	return fmt.Sprintf("%s (%s)", at.In(local).Format("Mon 2 Jan 15:04 MST"), utc)
}

// showCounts says how many recent reads came from installs a new minimum
// would ask to update, from the counts the server saves (R32).
func showCounts(env Env, cmd command) {
	raw, err := os.ReadFile(filepath.Join(env.StateDir, "reads.json"))
	if err != nil {
		fmt.Fprintln(env.Stdout, "  No install counts yet. The server saves them every 10 minutes.")
		return
	}
	var snap struct {
		Since, Until time.Time
		Rows         []struct {
			Store, Version, Build string
			Reads                 int
		}
	}
	if json.Unmarshal(raw, &snap) != nil {
		return
	}
	total, below := 0, 0
	for _, row := range snap.Rows {
		if row.Store != cmd.store {
			continue
		}
		total += row.Reads
		if cmd.verb == "min-version" {
			if c, ok := gate.CompareVersions(row.Version, cmd.version); ok && c < 0 {
				below += row.Reads
			}
		} else if b, err := strconv.ParseInt(row.Build, 10, 64); err == nil && b < cmd.build {
			below += row.Reads
		}
	}
	window := snap.Since.UTC().Format("15:04") + "–" + snap.Until.UTC().Format("15:04 UTC")
	if total == 0 {
		fmt.Fprintf(env.Stdout, "  No reads from %s were counted in the last window (%s).\n", gate.StoreName(cmd.store), window)
		return
	}
	fmt.Fprintf(env.Stdout, "  In the last window (%s), %d of %d reads from %s came from installs this would ask to update.\n",
		window, below, total, gate.StoreName(cmd.store))
}

func audit(env Env, args []string, before, after gate.Gate) error {
	if err := os.MkdirAll(env.StateDir, 0o755); err != nil {
		return err
	}
	line, _ := json.Marshal(map[string]any{
		"at": env.Now().UTC().Format(time.RFC3339), "user": env.User,
		"command": strings.Join(args, " "), "before": before, "after": after,
	})
	f, err := os.OpenFile(filepath.Join(env.StateDir, "audit.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
