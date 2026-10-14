package ctl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/gate"
	"github.com/Temitayo-spec/portcullis/server/internal/gatefile"
)

var now = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

type harness struct {
	t              *testing.T
	env            Env
	stdout, stderr *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	dir := t.TempDir()
	h := &harness{t: t, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	h.env = Env{
		GateFile: filepath.Join(dir, "data", "gate.json"),
		StateDir: filepath.Join(dir, "state"),
		User:     "temmy",
		Now:      func() time.Time { return now },
		Stdout:   h.stdout, Stderr: h.stderr,
	}
	return h
}

// run runs gatectl and fails the test unless it exits with want.
func (h *harness) run(want int, args ...string) string {
	h.t.Helper()
	h.stdout.Reset()
	h.stderr.Reset()
	if got := Run(args, h.env); got != want {
		h.t.Fatalf("gatectl %s exited %d, want %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), got, want, h.stdout, h.stderr)
	}
	return h.stdout.String() + h.stderr.String()
}

// file returns the gate file, checking it is one the server would serve.
func (h *harness) file() gate.Gate {
	h.t.Helper()
	raw, err := os.ReadFile(h.env.GateFile)
	if err != nil {
		h.t.Fatal(err)
	}
	r := gate.Validate(raw, now)
	if !r.OK() {
		h.t.Fatalf("gatectl wrote a gate the server would refuse: %v", r.Problems)
	}
	return r.Gate
}

func (h *harness) writeFile(content string) {
	h.t.Helper()
	os.MkdirAll(filepath.Dir(h.env.GateFile), 0o755)
	if err := os.WriteFile(h.env.GateFile, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func TestPauseAndResume(t *testing.T) {
	h := newHarness(t)
	h.run(0, "pause", "--reason", "upgrade", "--until", "2h")
	m := h.file().Maintenance
	if m == nil || m.Reason != "upgrade" || m.Until != "2026-10-14T14:00:00Z" {
		t.Fatalf("maintenance %+v", m)
	}
	h.run(0, "resume")
	if g := h.file(); g.Maintenance != nil {
		t.Errorf("still paused: %+v", g)
	}
}

func TestPauseWithEqualsAndZonedTime(t *testing.T) {
	h := newHarness(t)
	h.run(0, "pause", "--reason=outage", "--until=2026-10-14T19:00:00+01:00")
	if got := h.file().Maintenance.Until; got != "2026-10-14T18:00:00Z" {
		t.Errorf("until %q", got)
	}
}

func TestMinimumNeedsYes(t *testing.T) {
	h := newHarness(t)
	out := h.run(1, "min-version", "play", "2.5.0")
	if !strings.Contains(out, "Add --yes") {
		t.Errorf("no hint to add --yes: %s", out)
	}
	if _, err := os.Stat(h.env.GateFile); err == nil {
		t.Error("the file was written without --yes")
	}
	h.run(0, "min-version", "play", "2.5.0", "--yes")
	h.run(0, "min-build", "play", "130", "--yes")
	if got := h.file().Stores["play"]; got != (gate.Minimums{MinVersion: "2.5.0", MinBuild: 130}) {
		t.Errorf("play minimums %+v", got)
	}
	h.run(0, "min-version", "play", "clear")
	h.run(0, "min-build", "play", "clear")
	if got := h.file().Stores; got != nil {
		t.Errorf("stores %+v, want none left", got)
	}
}

func TestMinimumShowsInstallCounts(t *testing.T) {
	h := newHarness(t)
	os.MkdirAll(h.env.StateDir, 0o755)
	os.WriteFile(filepath.Join(h.env.StateDir, "reads.json"), []byte(`{
	  "since": "2026-10-14T11:50:00Z", "until": "2026-10-14T12:00:00Z",
	  "rows": [
	    {"store": "play", "version": "2.4.0", "build": "120", "reads": 30},
	    {"store": "play", "version": "2.5.0", "build": "130", "reads": 70},
	    {"store": "appstore", "version": "2.3.0", "build": "50", "reads": 9}
	  ]}`), 0o644)
	out := h.run(1, "min-version", "play", "2.5.0")
	if !strings.Contains(out, "30 of 100 reads from Google Play") {
		t.Errorf("counts missing: %s", out)
	}
}

func TestSwitches(t *testing.T) {
	h := newHarness(t)
	h.run(0, "hide", "rewards")
	out := h.run(0, "show", "referrals")
	if !strings.Contains(out, "Play installs") {
		t.Errorf("no Play warning on show: %s", out)
	}
	if got := h.file().Switches; !(got["rewards"] == false && got["referrals"] == true && len(got) == 2) {
		t.Errorf("switches %v", got)
	}
	h.run(0, "unset", "rewards")
	h.run(0, "unset", "referrals")
	if got := h.file().Switches; got != nil {
		t.Errorf("switches %v, want none", got)
	}
}

func TestResetNeedsYes(t *testing.T) {
	h := newHarness(t)
	h.run(0, "pause")
	h.run(1, "reset")
	if h.file().Maintenance == nil {
		t.Fatal("reset without --yes lifted the pause")
	}
	h.run(0, "reset", "--yes")
	if got := string(gate.Encode(h.file())); got != "{}\n" {
		t.Errorf("after reset: %s", got)
	}
}

// A broken file is fixed from the last good gate, so the pause stays.
func TestBrokenFileStartsFromLastGood(t *testing.T) {
	h := newHarness(t)
	h.run(0, "pause", "--reason", "outage")
	gatefile.Open(h.env.GateFile, filepath.Join(h.env.StateDir, "last-good.json"), gatefile.Options{}) // the server saves the pause
	h.writeFile(`{`)
	h.run(1, "status")
	h.run(0, "hide", "rewards")
	g := h.file()
	shown, set := g.Switches["rewards"]
	if g.Maintenance == nil || g.Maintenance.Reason != "outage" || !set || shown {
		t.Errorf("fixing the file lost something: %s", gate.Encode(g))
	}
}

func TestBrokenFileWithNoLastGood(t *testing.T) {
	h := newHarness(t)
	h.writeFile(`{`)
	h.run(1, "resume")
	h.run(0, "pause")
	if h.file().Maintenance == nil {
		t.Error("pause should still work")
	}
}

// gatectl never saves a gate the server would refuse: each refused case in
// the spec, found in the file, is reported and left for a command to replace.
func TestRefusedCasesAreNeverServed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "spec", "cases", "validate", "refused.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string          `json:"name"`
		Gate json.RawMessage `json:"gate"`
	}
	json.Unmarshal(raw, &cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			h := newHarness(t)
			h.writeFile(string(c.Gate))
			if out := h.run(1, "status"); !strings.Contains(out, "can't be used") {
				t.Errorf("status didn't say the file can't be used: %s", out)
			}
			h.run(0, "pause") // any command replaces it with a valid gate
			h.file()
		})
	}
}

func TestBadCommands(t *testing.T) {
	for _, args := range [][]string{
		{"launch"},
		{"pause", "now"},
		{"pause", "--reason", "party"},
		{"pause", "--until", "tomorrow"},
		{"pause", "--until", "0h"},
		{"pause", "--reason"},
		{"resume", "--yes"},
		{"min-version", "ios", "2.4.0"},
		{"min-version", "play", "v2.4.0"},
		{"min-version", "play"},
		{"min-build", "play", "0"},
		{"min-build", "play", "12.5"},
		{"hide", "Rewards"},
		{"hide"},
		{"reset", "--yes=1"},
		{"status", "--force"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(t)
			h.run(2, args...)
			if _, err := os.Stat(h.env.GateFile); err == nil {
				t.Error("a bad command wrote the file")
			}
		})
	}
}

func TestStoreHint(t *testing.T) {
	h := newHarness(t)
	if out := h.run(2, "min-version", "ios", "2.4.0"); !strings.Contains(out, `use "appstore"`) {
		t.Errorf("no hint: %s", out)
	}
}

func TestStatus(t *testing.T) {
	h := newHarness(t)
	if out := h.run(0, "status"); !strings.Contains(out, "runs normally") {
		t.Errorf("no file: %s", out)
	}
	h.run(0, "pause", "--until", "2026-10-14T18:00:00Z")
	h.env.Local = time.FixedZone("WAT", 3600)
	if out := h.run(0, "status"); !strings.Contains(out, "Wed 14 Oct 19:00 WAT (Wed 14 Oct 18:00 UTC)") {
		t.Errorf("time not shown in both zones: %s", out)
	}
}

func TestAuditLog(t *testing.T) {
	h := newHarness(t)
	h.run(0, "pause", "--reason", "issue")
	h.run(0, "resume")
	raw, err := os.ReadFile(filepath.Join(h.env.StateDir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d audit lines, want 2", len(lines))
	}
	var entry struct {
		User, Command string
		Before        gate.Gate
	}
	json.Unmarshal([]byte(lines[1]), &entry)
	if entry.User != "temmy" || entry.Command != "resume" || entry.Before.Maintenance == nil {
		t.Errorf("audit entry %+v", entry)
	}
}

func TestParseUntil(t *testing.T) {
	for in, want := range map[string]string{
		"90m":                       "2026-10-14T13:30:00Z",
		"2h":                        "2026-10-14T14:00:00Z",
		"3d":                        "2026-10-17T12:00:00Z",
		"2026-10-14T19:00:00+01:00": "2026-10-14T18:00:00Z",
	} {
		if got, err := ParseUntil(in, now); err != nil || got != want {
			t.Errorf("ParseUntil(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
