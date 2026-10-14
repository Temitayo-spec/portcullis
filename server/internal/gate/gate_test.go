package gate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// Every gate in spec/cases/validate/refused.json must be refused (R33–R40).
func TestRefusedCases(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "spec", "cases", "validate", "refused.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string          `json:"name"`
		Rule string          `json:"rule"`
		Gate json.RawMessage `json:"gate"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases found")
	}
	for _, c := range cases {
		t.Run(c.Rule+" "+c.Name, func(t *testing.T) {
			if r := Validate(c.Gate, now); r.OK() {
				t.Errorf("accepted %s; want it refused", c.Gate)
			}
		})
	}
}

func TestAccepted(t *testing.T) {
	for name, raw := range map[string]string{
		"empty gate":           `{}`,
		"pause with all":       `{"maintenance":{"active":true,"reason":"outage","until":"2026-10-14T19:00:00+01:00"}}`,
		"minimums per store":   `{"stores":{"play":{"minVersion":"2.4.0","minBuild":120},"appstore":{"minVersion":"2.4.0"},"com.huawei.appmarket":{"minBuild":118}}}`,
		"switches":             `{"switches":{"rewards":false,"referrals":true}}`,
		"four-part version":    `{"stores":{"play":{"minVersion":"2.4.0.1"}}}`,
		"build at the ceiling": `{"stores":{"play":{"minBuild":2100000000}}}`,
		"until in the past":    `{"maintenance":{"active":true,"until":"2026-10-01T00:00:00Z"}}`,
		"surrounding spaces":   " \n {} \n ",
	} {
		t.Run(name, func(t *testing.T) {
			if r := Validate([]byte(raw), now); !r.OK() {
				t.Errorf("refused: %v", r.Problems)
			}
		})
	}
}

func TestRefusedBeyondTheSpec(t *testing.T) {
	for name, raw := range map[string]string{
		"empty file":             ``,
		"not JSON":               `{`,
		"two documents":          `{} {}`,
		"build over the ceiling": `{"stores":{"play":{"minBuild":2100000001}}}`,
		"build written as 1e2":   `{"stores":{"play":{"minBuild":1e2}}}`,
		"five-part version":      `{"stores":{"play":{"minVersion":"1.2.3.4.5"}}}`,
		"until without seconds":  `{"maintenance":{"active":true,"until":"2026-10-14T18:00Z"}}`,
		"until on 30 February":   `{"maintenance":{"active":true,"until":"2026-02-30T18:00:00Z"}}`,
		"stores not an object":   `{"stores":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if r := Validate([]byte(raw), now); r.OK() {
				t.Errorf("accepted %s", raw)
			}
		})
	}
}

func TestCleanGate(t *testing.T) {
	r := Validate([]byte(`{"maintenance":{"active":true,"until":"2026-10-14T19:00:00+01:00"},"switches":{"rewards":false}}`), now)
	if !r.OK() {
		t.Fatal(r.Problems)
	}
	if got := r.Gate.Maintenance.Until; got != "2026-10-14T18:00:00Z" {
		t.Errorf("until = %q, want it in UTC", got)
	}
	if got, want := string(Encode(r.Gate)), `{"maintenance":{"active":true,"until":"2026-10-14T18:00:00Z"},"switches":{"rewards":false}}`+"\n"; got != want {
		t.Errorf("encoded %s, want %s", got, want)
	}
	if got := string(Encode(Gate{})); got != "{}\n" {
		t.Errorf("empty gate encoded %q", got)
	}
}

func TestWarnings(t *testing.T) {
	for name, tc := range map[string]struct{ raw, want string }{
		"minimum":            {`{"stores":{"play":{"minVersion":"2.4.0"}}}`, "below version 2.4.0"},
		"switch on Play":     {`{"switches":{"rewards":true}}`, "Play installs"},
		"pause hides update": {`{"maintenance":{"active":true},"stores":{"play":{"minBuild":5}}}`, "not the update screen"},
		"until far away":     {`{"maintenance":{"active":true,"until":"2026-10-25T12:00:00Z"}}`, "more than 7 days"},
	} {
		t.Run(name, func(t *testing.T) {
			r := Validate([]byte(tc.raw), now)
			if !r.OK() {
				t.Fatal(r.Problems)
			}
			if !strings.Contains(strings.Join(r.Warnings, "\n"), tc.want) {
				t.Errorf("warnings %q do not mention %q", r.Warnings, tc.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
		ok   bool
	}{
		{"1.0.9", "1.0.10", -1, true},
		{"1.2", "1.2.0", 0, true},
		{"2.5.0", "2.4.9.9", 1, true},
		{"v1.0", "1.0", 0, false},
		{"other", "1.0", 0, false},
	} {
		got, ok := CompareVersions(tc.a, tc.b)
		if got != tc.want || ok != tc.ok {
			t.Errorf("CompareVersions(%q, %q) = %d, %v; want %d, %v", tc.a, tc.b, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDescribe(t *testing.T) {
	r := Validate([]byte(`{"maintenance":{"active":true,"reason":"upgrade"},"stores":{"play":{"minVersion":"2.4.0","minBuild":120}},"switches":{"rewards":false}}`), now)
	want := "paused (upgrade), until you run resume; Google Play installs below version 2.4.0 or build 120 are asked to update; rewards is hidden everywhere"
	if got := Describe(r.Gate); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if got := Describe(Gate{}); got != "the app runs normally" {
		t.Errorf("empty gate: %q", got)
	}
}
